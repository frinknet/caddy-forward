package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	caddytpl "github.com/caddyserver/caddy/v2/modules/caddyhttp/templates"
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

// ── USE template function ────────────────────────────────────────────────

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func testContext(t *testing.T, dir, target string) *caddytpl.TemplateContext {
	t.Helper()
	funcs := MethodFuncs(subreq.NewRequester(5*time.Second, 1024))
	req := httptest.NewRequest("GET", target, nil)
	repl := caddy.NewReplacer()
	repl.Set("http.request.uri.path", target)
	req = req.WithContext(context.WithValue(req.Context(), caddy.ReplacerCtxKey, repl))
	return &caddytpl.TemplateContext{
		Root:        http.Dir(dir),
		Req:         req,
		CustomFuncs: []template.FuncMap{funcs},
	}
}

func TestMethodFuncsIncludesUse(t *testing.T) {
	funcs := MethodFuncs(subreq.NewRequester(5*time.Second, 1024))
	if _, ok := funcs["USE"]; !ok {
		t.Errorf("MethodFuncs should expose USE")
	}
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if _, ok := funcs[m]; !ok {
			t.Errorf("MethodFuncs should still expose %s", m)
		}
	}
}

func TestUseDefaultsToContextDot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "child.html", `path={{ .Req.URL.Path }} {{ placeholder "http.request.uri.path" }}`)

	ctx := testContext(t, dir, "/from-context")
	out, err := Use("child.html", ctx)
	if err != nil {
		t.Fatalf("USE should render: %v", err)
	}
	if out != "path=/from-context /from-context" {
		t.Errorf("child should receive the parent context, got %q", out)
	}
}

func TestUsePassesCustomDot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "child.html", `hi {{ .Name }}`)

	ctx := testContext(t, dir, "/")
	out, err := Use("child.html", ctx, map[string]any{"Name": "Bob"})
	if err != nil {
		t.Fatalf("USE should render: %v", err)
	}
	if out != "hi Bob" {
		t.Errorf("child should see the passed data as its dot, got %q", out)
	}
}

func TestUseNests(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "outer.html", `outer({{ USE "inner.html" . }})`)
	writeFile(t, dir, "inner.html", `inner:{{ .Req.URL.Path }}`)

	ctx := testContext(t, dir, "/nested")
	out, err := Use("outer.html", ctx)
	if err != nil {
		t.Fatalf("nested USE should render: %v", err)
	}
	if out != "outer(inner:/nested)" {
		t.Errorf("unexpected nested render: %q", out)
	}
}

func TestUseRendersInsideParentTemplate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "child.html", `[{{ .Req.URL.Query.Get "q" }}]`)

	ctx := testContext(t, dir, "/?q=42")
	tpl, err := ctx.NewTemplate("parent").Parse(`before {{ USE "child.html" . }} after`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sb.String() != "before [42] after" {
		t.Errorf("unexpected render: %q", sb.String())
	}
}

func TestUseWithoutContextIsAnError(t *testing.T) {
	if _, err := Use("child.html", map[string]any{"Name": "Bob"}); err == nil {
		t.Errorf("USE without a template context should error")
	}
}

func TestUseMissingFileIsAnError(t *testing.T) {
	ctx := testContext(t, t.TempDir(), "/")
	if _, err := Use("nope.html", ctx); err == nil {
		t.Errorf("USE of a missing file should error")
	}
}