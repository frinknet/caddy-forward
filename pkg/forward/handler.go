package forward

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/frinknet/caddy-forward/pkg/config"
	"github.com/frinknet/caddy-forward/pkg/subreq"
)

func init() {
	caddy.RegisterModule(ForwardHandler{})
}

// ForwardHandler implements the `forward` Caddyfile directive.
type ForwardHandler struct {
	Method  string
	URL     string
	Params  []Param
	Headers []Header
	Body    string

	requester *subreq.Requester
}

// Param appends a URL query parameter.
type Param struct {
	Name  string
	Value string
}

// Header sets a request header.
type Header struct {
	Name  string
	Value string
}

func (ForwardHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.forward",
		New: func() caddy.Module { return new(ForwardHandler) },
	}
}

func (h *ForwardHandler) Provision(ctx caddy.Context) error {
	timeout := config.DefaultTimeout
	maxSize := int64(config.DefaultMaxResponseSize)

	if appIface, err := ctx.App("forward"); err == nil {
		if app, ok := appIface.(*config.App); ok {
			timeout = time.Duration(app.Timeout)
			maxSize = app.MaxResponseSize
		}
	}

	h.requester = subreq.NewRequester(timeout, maxSize)
	return nil
}

// SetRequester is a test helper to set the requester without provisioning.
func (h *ForwardHandler) SetRequester(r *subreq.Requester) {
	h.requester = r
}

func (h *ForwardHandler) Validate() error {
	if h.URL == "" {
		return fmt.Errorf("forward: url is required")
	}
	if !subreq.IsBodyMethod(h.Method) && !subreq.IsQueryMethod(h.Method) {
		return fmt.Errorf("forward: unsupported method %q", h.Method)
	}
	if h.Body != "" && !subreq.IsBodyMethod(h.Method) {
		return fmt.Errorf("forward: body subdirective is only allowed on POST, PUT, PATCH")
	}
	return nil
}

func (h *ForwardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	repl, _ := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	rawURL := h.URL
	if repl != nil {
		rawURL = repl.ReplaceAll(rawURL, "")
	}

	if len(h.Params) > 0 {
		vals := url.Values{}
		for _, p := range h.Params {
			name, value := p.Name, p.Value
			if repl != nil {
				name = repl.ReplaceAll(name, "")
				value = repl.ReplaceAll(value, "")
			}
			vals.Add(name, value)
		}
		rawURL = subreq.MergeQueryIntoURL(rawURL, vals.Encode())
	}

	headers := make(map[string]string, len(h.Headers))
	for _, hdr := range h.Headers {
		v := hdr.Value
		if repl != nil {
			v = repl.ReplaceAll(v, "")
		}
		headers[hdr.Name] = v
	}

	var data any
	if subreq.IsBodyMethod(h.Method) && h.Body != "" {
		data = h.Body
		if repl != nil {
			data = repl.ReplaceAll(h.Body, "")
		}
	}

	resp := h.requester.Send(r.Context(), h.Method, rawURL, headers, data)

	if resp.Err != nil {
		return caddyhttp.Error(http.StatusBadGateway, resp.Err)
	}

	for k, vals := range resp.Headers {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.Status)
	if _, err := w.Write([]byte(resp.Body)); err != nil {
		return err
	}
	return nil
}

var (
	_ caddy.Module                = (*ForwardHandler)(nil)
	_ caddy.Provisioner           = (*ForwardHandler)(nil)
	_ caddy.Validator             = (*ForwardHandler)(nil)
	_ caddyhttp.MiddlewareHandler = (*ForwardHandler)(nil)
)