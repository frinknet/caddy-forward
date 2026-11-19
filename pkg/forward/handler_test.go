package forward

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/frinknet/caddy-forward/pkg/subreq"
)

// ── Validate() tests ──────────────────────────────────────────────────────

func TestForwardHandlerBodyOnGetFailsValidation(t *testing.T) {
	h := ForwardHandler{
		Method: "GET",
		URL:    "http://example.com",
		Body:   "should not be allowed",
	}

	err := h.Validate()
	if err == nil {
		t.Errorf("Validate should reject body on GET")
	}
}

func TestForwardHandlerMissingUrlFailsValidation(t *testing.T) {
	h := ForwardHandler{
		Method: "GET",
		URL:    "",
	}

	err := h.Validate()
	if err == nil {
		t.Errorf("Validate should reject empty URL")
	}
}

func TestForwardHandlerUnsupportedMethodFailsValidation(t *testing.T) {
	h := ForwardHandler{
		Method: "CONNECT",
		URL:    "http://example.com",
	}

	err := h.Validate()
	if err == nil {
		t.Errorf("Validate should reject CONNECT")
	}
}

func TestForwardHandlerValidPostPassesValidation(t *testing.T) {
	h := ForwardHandler{
		Method: "POST",
		URL:    "http://example.com",
		Body:   "valid body",
	}

	err := h.Validate()
	if err != nil {
		t.Errorf("POST with body should validate: %v", err)
	}
}

func TestForwardHandlerValidGetPassesValidation(t *testing.T) {
	h := ForwardHandler{
		Method: "GET",
		URL:    "http://example.com",
	}

	err := h.Validate()
	if err != nil {
		t.Errorf("GET should validate: %v", err)
	}
}

func TestForwardHandlerTraceFailsValidation(t *testing.T) {
	h := ForwardHandler{
		Method: "TRACE",
		URL:    "http://example.com",
	}

	err := h.Validate()
	if err == nil {
		t.Errorf("TRACE should be rejected")
	}
}

// ── ServeHTTP tests ──────────────────────────────────────────────────────

func TestForwardHandlerServeGetOK(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("X-Backend", "test")
		w.WriteHeader(200)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP should not return error: %v", serveErr)
	}

	resp := recorder.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status should be 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from backend" {
		t.Errorf("body should be 'hello from backend', got %q", string(body))
	}
	if resp.Header.Get("X-Backend") != "test" {
		t.Errorf("should forward X-Backend header, got %q", resp.Header.Get("X-Backend"))
	}
}

func TestForwardHandlerServePostWithBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "request-body" {
			t.Errorf("body should be 'request-body', got %q", string(b))
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(201)
		w.Write(b)
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "POST",
		URL:    backend.URL,
		Body:   "request-body",
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP should succeed: %v", serveErr)
	}

	resp := recorder.Result()
	if resp.StatusCode != 201 {
		t.Errorf("status should be 201, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "request-body" {
		t.Errorf("body should echo 'request-body', got %q", string(body))
	}
}

func TestForwardHandlerServeWithParams(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=caddy") {
			t.Errorf("should have q=caddy in query, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Params: []Param{Param{Name: "q", Value: "caddy"}},
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP should succeed: %v", serveErr)
	}

	resp := recorder.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status should be 200, got %d", resp.StatusCode)
	}
}

func TestForwardHandlerServeWithHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("should have Authorization header, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Headers: []Header{
			Header{Name: "Authorization", Value: "Bearer secret"},
		},
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP should succeed: %v", serveErr)
	}
}

func TestForwardHandlerServeUpstream5xx(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("upstream 5xx is not a handler error: %v", serveErr)
	}

	resp := recorder.Result()
	if resp.StatusCode != 500 {
		t.Errorf("status should be 500, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "server error") {
		t.Errorf("body should contain upstream message, got %q", string(body))
	}
}

func TestForwardHandlerServeNetworkErrorReturns502(t *testing.T) {
	h := ForwardHandler{
		Method: "GET",
		URL:    "http://127.0.0.1:1/",
	}
	h.SetRequester(subreq.NewRequester(1*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr == nil {
		t.Errorf("network error should return caddyhttp.Error")
		return
	}
	// Verify the error is a 502
	if httpErr, ok := serveErr.(caddyhttp.HandlerError); ok {
		if httpErr.StatusCode != 502 {
			t.Errorf("should return 502, got %d", httpErr.StatusCode)
		}
	} else {
		t.Errorf("ServeHTTP error should be caddyhttp.Error, got %T", serveErr)
	}
}

func TestForwardHandlerServeWithPlaceholderURL(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(r.URL.String()))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL + "/{http.request.uri.path}",
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	repl := caddy.NewReplacer()
	repl.Set("http.request.uri.path", "/placeholder")
	req := httptest.NewRequest("GET", "http://test/", nil)
	ctx := context.WithValue(req.Context(), caddy.ReplacerCtxKey, repl)
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP with placeholder should succeed: %v", serveErr)
	}

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "/placeholder") {
		t.Errorf("placeholder should be replaced in URL, got %q", string(body))
	}
}

func TestForwardHandlerServeWithPlaceholderParam(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(r.URL.RawQuery))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Params: []Param{{Name: "q", Value: "{http.request.uri.query.q}"}},
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	repl := caddy.NewReplacer()
	repl.Set("http.request.uri.query.q", "caddy")
	req := httptest.NewRequest("GET", "http://test/?q=caddy", nil)
	ctx := context.WithValue(req.Context(), caddy.ReplacerCtxKey, repl)
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP with placeholder param should succeed: %v", serveErr)
	}

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "q=caddy") {
		t.Errorf("placeholder should be replaced in param value, got %q", string(body))
	}
}

func TestForwardHandlerServeWithPlaceholderHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo", r.Header.Get("Authorization"))
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	h := ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Headers: []Header{
			{Name: "Authorization", Value: "Bearer {http.request.header.Token}"},
		},
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	repl := caddy.NewReplacer()
	repl.Set("http.request.header.Token", "secret123")
	req := httptest.NewRequest("GET", "http://test/", nil)
	ctx := context.WithValue(req.Context(), caddy.ReplacerCtxKey, repl)
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP with placeholder header should succeed: %v", serveErr)
	}

	resp := recorder.Result()
	if resp.Header.Get("X-Echo") != "Bearer secret123" {
		t.Errorf("placeholder should be replaced in header value, got %q", resp.Header.Get("X-Echo"))
	}
}

// ── USE (forward USE path) ────────────────────────────────────────────────

func TestForwardUseRequiresPath(t *testing.T) {
	h := ForwardHandler{Method: MethodUse}
	if err := h.Validate(); err == nil {
		t.Errorf("USE without a path should fail validation")
	}
}

func TestForwardUseRejectsHeader(t *testing.T) {
	h := ForwardHandler{Method: MethodUse, URL: "page.html", Headers: []Header{{Name: "X", Value: "y"}}}
	if err := h.Validate(); err == nil {
		t.Errorf("USE should reject header")
	}
}

func useHandler(t *testing.T, dir, path, contents string) *ForwardHandler {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	h := &ForwardHandler{Method: MethodUse, URL: path}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))
	return h
}

func useRequest(dir, target string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	req = req.WithContext(context.WithValue(req.Context(), caddyhttp.VarsCtxKey, map[string]any{}))
	caddyhttp.NewTestReplacer(req)
	caddyhttp.SetVar(req.Context(), "root", dir)
	return req
}

func TestForwardUseServesTemplate(t *testing.T) {
	dir := t.TempDir()
	h := useHandler(t, dir, "page.html", `hello {{ .Req.URL.Path }} {{ placeholder "http.request.uri.path" }}`)

	recorder := httptest.NewRecorder()
	if err := h.ServeHTTP(recorder, useRequest(dir, "/page"), nil); err != nil {
		t.Fatalf("ServeHTTP should succeed: %v", err)
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("status should be 200, got %d", recorder.Code)
	}
	if got := recorder.Body.String(); got != "hello /page /page" {
		t.Errorf("unexpected body: %q", got)
	}
	if ct := recorder.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type should be html, got %q", ct)
	}
}

func TestForwardUseCancelsTemplateFetch(t *testing.T) {
	started := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer backend.Close()

	dir := t.TempDir()
	h := useHandler(t, dir, "page.html", `{{ GET "`+backend.URL+`" nil nil }}`)

	req := useRequest(dir, "/page")
	reqCtx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(reqCtx)

	done := make(chan error, 1)
	go func() {
		done <- h.ServeHTTP(httptest.NewRecorder(), req, nil)
	}()

	<-started
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("ServeHTTP should succeed after cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("template fetch did not cancel with the request")
	}
}

func TestForwardUsePassesQueryAndBody(t *testing.T) {
	dir := t.TempDir()
	h := useHandler(t, dir, "page.html", `q={{ placeholder "http.request.uri.query.q" }} b={{ placeholder "http.request.body" }}`)
	h.Params = []Param{{Name: "q", Value: "caddy"}}
	h.Body = "hello body"

	recorder := httptest.NewRecorder()
	if err := h.ServeHTTP(recorder, useRequest(dir, "/page"), nil); err != nil {
		t.Fatalf("ServeHTTP should succeed: %v", err)
	}
	if got := recorder.Body.String(); got != "q=caddy b=hello body" {
		t.Errorf("query and body should reach the template, got %q", got)
	}
}

func TestForwardUseMissingFileIs500(t *testing.T) {
	dir := t.TempDir()
	h := &ForwardHandler{Method: MethodUse, URL: "nope.html"}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	err := h.ServeHTTP(recorder, useRequest(dir, "/missing"), nil)
	if err == nil {
		t.Fatalf("missing template should be an error")
	}
	httpErr, ok := err.(caddyhttp.HandlerError)
	if !ok {
		t.Fatalf("missing template should be a caddyhttp.HandlerError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("status should be 500, got %d", httpErr.StatusCode)
	}
}
