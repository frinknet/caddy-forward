package subreq

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/frinknet/caddy-forward/pkg/encoding"
)

// IsBodyMethod reports whether the method sends data in the request body.
func IsBodyMethod(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH":
		return true
	}
	return false
}

// IsQueryMethod reports whether the method sends data in the query string.
func IsQueryMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "DELETE", "OPTIONS":
		return true
	}
	return false
}

// Send performs the sub-request. Errors are returned inside Response.Err —
// never as a Go error — so template rendering continues on failure.
func (r *Requester) Send(ctx context.Context, method, rawURL string, headers, data any) Response {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Response{Err: fmt.Errorf("invalid URL: %w", err)}
	}

	hdrs, err := normalizeHeaders(headers)
	if err != nil {
		return Response{Err: err}
	}

	data, err = normalizeData(data)
	if err != nil {
		return Response{Err: err}
	}

	var body io.Reader
	var contentType string

	if IsQueryMethod(method) {
		switch d := data.(type) {
		case nil:
			// nothing
		case string:
			u.RawQuery = encoding.MergeQuery(u.RawQuery, d)
		case map[string]any:
			merged, err := encoding.MergeQueryDict(u.RawQuery, d)
			if err != nil {
				return Response{Err: err}
			}
			u.RawQuery = merged
		}
	} else {
		switch d := data.(type) {
		case nil:
			// empty body
		case string:
			body = strings.NewReader(d)
			if lookupCI(hdrs, "Content-Type") == "" {
				contentType = "text/plain"
			}
		case map[string]any:
			ct := lookupCI(hdrs, "Content-Type")
			lower := strings.ToLower(ct)
			switch {
			case encoding.HasBinary(d) || strings.HasPrefix(lower, "multipart/form-data"):
				b, mpCT, err := encoding.EncodeMultipart(d)
				if err != nil {
					return Response{Err: err}
				}
				body = bytes.NewReader(b)
				contentType = mpCT
			case ct == "" || strings.HasPrefix(lower, "application/x-www-form-urlencoded"):
				f, err := encoding.EncodeForm(d)
				if err != nil {
					return Response{Err: err}
				}
				body = strings.NewReader(f)
				contentType = "application/x-www-form-urlencoded"
			default:
				return Response{Err: fmt.Errorf("dict body needs a form Content-Type, got %q", ct)}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return Response{Err: err}
	}

	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := r.Client.Do(req)
	if err != nil {
		return Response{Err: err}
	}
	defer resp.Body.Close()

	if method == "HEAD" {
		return Response{Body: "", Status: resp.StatusCode, Headers: resp.Header}
	}

	limited := io.LimitReader(resp.Body, r.MaxResponseSize+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return Response{Err: err}
	}
	if int64(len(bodyBytes)) > r.MaxResponseSize {
		return Response{Err: fmt.Errorf("response body exceeds max size (%d bytes)", r.MaxResponseSize)}
	}

	return Response{
		Body:    string(bodyBytes),
		Status:  resp.StatusCode,
		Headers: resp.Header,
	}
}

// MergeQueryIntoURL appends qs to rawURL's query string and returns the
// re-encoded URL. If rawURL is unparseable, it is returned unchanged.
func MergeQueryIntoURL(rawURL, qs string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = encoding.MergeQuery(u.RawQuery, qs)
	return u.String()
}

func normalizeHeaders(v any) (map[string]string, error) {
	switch h := v.(type) {
	case nil:
		return nil, nil
	case map[string]string:
		return h, nil
	case map[string]any:
		out := make(map[string]string, len(h))
		for k, val := range h {
			out[k] = fmt.Sprintf("%v", val)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported headers type: %T", v)
	}
}

func normalizeData(v any) (any, error) {
	switch d := v.(type) {
	case nil:
		return nil, nil
	case string:
		return d, nil
	case map[string]any:
		return d, nil
	case map[string]string:
		out := make(map[string]any, len(d))
		for k, val := range d {
			out[k] = val
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported data type: %T", v)
	}
}

func lookupCI(h map[string]string, key string) string {
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}