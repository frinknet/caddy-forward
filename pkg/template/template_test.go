package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"text/template"
	"testing"
	"time"

	"github.com/frinknet/caddy-forward/pkg/subreq"
)

// ── Template rendering with mock functions ───────────────────────────────

// Helper to render a template with a given FuncMap.
func render(t *testing.T, src string, funcs template.FuncMap) string {
	tmpl := template.New("test").Funcs(funcs)
	tmpl, parseErr := tmpl.Parse(src)
	if parseErr != nil {
		t.Errorf("parse error: %v", parseErr)
		return ""
	}
	buf := &bytes.Buffer{}
	execErr := tmpl.Execute(buf, nil)
	if execErr != nil {
		t.Errorf("execute error: %v", execErr)
		return ""
	}
	return string(buf.Bytes())
}

func TestTemplateGetRendersBody(t *testing.T) {
	out := render(t, `{{ GET "http://example.com" nil nil }}`, template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "hello from test", Status: 200}
		},
	})
	if out != "hello from test" {
		t.Errorf("should render the body, got %q", out)
	}
}

func TestTemplateAllMethods(t *testing.T) {
	methods := []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	for _, m := range methods {
		funcs := template.FuncMap{}
		funcs[m] = func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: m, Status: 200}
		}
		out := render(t, fmt.Sprintf(`{{ %s "http://x" nil nil }}`, m), funcs)
		if out != m {
			t.Errorf("%s should render its name, got %q", m, out)
		}
	}
}

func TestTemplateAccessStatus(t *testing.T) {
	out := render(t, `{{ (GET "http://example.com" nil nil).Status }}`, template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "body", Status: 201}
		},
	})
	if out != "201" {
		t.Errorf("should render the status code, got %q", out)
	}
}

func TestTemplateAccessHeaders(t *testing.T) {
	hdrs := http.Header{}
	hdrs.Set("Content-Type", "application/json")
	out := render(t, `{{ (GET "http://x" nil nil).Headers.Get "Content-Type" }}`, template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "", Status: 200, Headers: hdrs}
		},
	})
	if out != "application/json" {
		t.Errorf("should render header value, got %q", out)
	}
}

func TestTemplateContinuesOnError(t *testing.T) {
	out := render(t, `before{{ GET "http://example.com" nil nil }}after`, template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "", Status: 0, Err: fmt.Errorf("network error")}
		},
	})
	if out != "beforeafter" {
		t.Errorf("should concatenate around error, got %q", out)
	}
}

func TestTemplatePipedData(t *testing.T) {
	out := render(t, `{{ "data" | POST "http://x" nil }}`, template.FuncMap{
		"POST": func(url string, headers, data any) subreq.Response {
			switch d := data.(type) {
			case string:
				return subreq.Response{Body: d, Status: 200}
			}
			return subreq.Response{Body: "no-data", Status: 500}
		},
	})
	if out != "data" {
		t.Errorf("piped data should be passed as data param, got %q", out)
	}
}

func TestTemplateBodyAndStatus(t *testing.T) {
	out := render(t, `{{ $r := GET "http://x" nil nil }}{{ $r.Status }} {{ $r }}`, template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "hello", Status: 200}
		},
	})
	if out != "200 hello" {
		t.Errorf("should render status and body, got %q", out)
	}
}

func TestTemplateResponseString(t *testing.T) {
	// String() returns Body, which is what templates render
	r := subreq.Response{Body: "hello", Status: 200}
	s := fmt.Sprintf("%v", r)
	if s != "hello" {
		t.Errorf("String() should return body, got %q", s)
	}
}

func TestTemplateResponseStringEmptyOnError(t *testing.T) {
	r := subreq.Response{Body: "", Status: 0, Err: fmt.Errorf("fail")}
	s := fmt.Sprintf("%v", r)
	if s != "" {
		t.Errorf("String() should be empty on error, got %q", s)
	}
}

// ── Integration test with actual server ───────────────────────────────────

func TestTemplateIntegrationGet(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("integration ok"))
	}))
	defer backend.Close()

	// Use a real requester, call Send directly (as CustomTemplateFunctions does)
	r := subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024)

	funcs := template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return r.Send(context.Background(), "GET", url, headers, data)
		},
	}

	out := render(t, fmt.Sprintf(`{{ GET %q nil nil }}`, backend.URL), funcs)
	if out != "integration ok" {
		t.Errorf("should render backend response body, got %q", out)
	}
}

func TestTemplateIntegrationPostWithBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Echo", string(b))
		w.WriteHeader(200)
		w.Write(b)
	}))
	defer backend.Close()

	r := subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024)
	funcs := template.FuncMap{
		"POST": func(url string, headers, data any) subreq.Response {
			return r.Send(context.Background(), "POST", url, headers, data)
		},
	}

	out := render(t, fmt.Sprintf(`{{ POST %q nil "body-data" }}`, backend.URL), funcs)
	if out != "body-data" {
		t.Errorf("should echo body, got %q", out)
	}
}

func TestTemplateIntegrationNetworkError(t *testing.T) {
	r := subreq.NewRequester(1 * time.Second, 10 * 1024 * 1024)
	funcs := template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return r.Send(context.Background(), "GET", url, headers, data)
		},
	}

	out := render(t, `before{{ GET "http://127.0.0.1:1/x" nil nil }}after`, funcs)
	if out != "beforeafter" {
		t.Errorf("should continue past network error, got %q", out)
	}
}