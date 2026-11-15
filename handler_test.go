package caddyforward

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/you/caddy-forward/pkg/forward"
	"github.com/you/caddy-forward/pkg/subreq"
)

// ── Validate() tests ──────────────────────────────────────────────────────

func TestForwardHandlerBodyOnGetFailsValidation(t *testing.T) {
	h := forward.ForwardHandler{
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
	h := forward.ForwardHandler{
		Method: "GET",
		URL:    "",
	}

	err := h.Validate()
	if err == nil {
		t.Errorf("Validate should reject empty URL")
	}
}

func TestForwardHandlerUnsupportedMethodFailsValidation(t *testing.T) {
	h := forward.ForwardHandler{
		Method: "CONNECT",
		URL:    "http://example.com",
	}

	err := h.Validate()
	if err == nil {
		t.Errorf("Validate should reject CONNECT")
	}
}

func TestForwardHandlerValidPostPassesValidation(t *testing.T) {
	h := forward.ForwardHandler{
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
	h := forward.ForwardHandler{
		Method: "GET",
		URL:    "http://example.com",
	}

	err := h.Validate()
	if err != nil {
		t.Errorf("GET should validate: %v", err)
	}
}

func TestForwardHandlerTraceFailsValidation(t *testing.T) {
	h := forward.ForwardHandler{
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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "POST",
		URL:    backend.URL,
		Body:   "request-body",
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Params: []forward.Param{forward.Param{Name: "q", Value: "caddy"}},
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Headers: []forward.Header{
			forward.Header{Name: "Authorization", Value: "Bearer secret"},
		},
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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
	h := forward.ForwardHandler{
		Method: "GET",
		URL:    "http://127.0.0.1:1/",
	}
	h.SetRequester(subreq.NewRequester(1 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL + "/{http.request.uri.path}",
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Params: []forward.Param{{Name: "q", Value: "{http.request.uri.query.q}"}},
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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

	h := forward.ForwardHandler{
		Method: "GET",
		URL:    backend.URL,
		Headers: []forward.Header{
			{Name: "Authorization", Value: "Bearer {http.request.header.Token}"},
		},
	}
	h.SetRequester(subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024))

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