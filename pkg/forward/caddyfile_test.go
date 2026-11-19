package forward

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/frinknet/caddy-forward/pkg/config"
	"github.com/frinknet/caddy-forward/pkg/subreq"
)

// ── Helpers ───────────────────────────────────────────────────────────────

func parseForward(input string) (*ForwardHandler, error) {
	d := caddyfile.NewTestDispenser(input)
	var h ForwardHandler
	if err := h.UnmarshalCaddyfile(d); err != nil {
		return nil, err
	}
	return &h, nil
}

func parseForwardBlock(input string) (*ForwardHandler, error) {
	full := "forward " + input
	return parseForward(full)
}

func parseConfig(input string) (*config.App, error) {
	d := caddyfile.NewTestDispenser(input)
	var a config.App
	if err := a.UnmarshalCaddyfile(d); err != nil {
		return nil, err
	}
	return &a, nil
}

// ── Config parsing ────────────────────────────────────────────────────────

func TestConfigParseTimeoutAndMaxSize(t *testing.T) {
	a, err := parseConfig(`forward { timeout 30s max_response_size 2000000 }`)
	if err != nil {
		t.Errorf("should parse: %v", err)
		return
	}
	if a.Timeout != caddy.Duration(30*time.Second) {
		t.Errorf("timeout should be 30s, got %v", a.Timeout)
	}
	if a.MaxResponseSize != 2000000 {
		t.Errorf("max_response_size should be 2000000, got %d", a.MaxResponseSize)
	}
}

func TestConfigParseTimeoutOnly(t *testing.T) {
	a, err := parseConfig(`forward { timeout 5s }`)
	if err != nil {
		t.Errorf("should parse: %v", err)
		return
	}
	if a.Timeout != caddy.Duration(5*time.Second) {
		t.Errorf("timeout should be 5s, got %v", a.Timeout)
	}
}

func TestConfigParseMaxSizeOnly(t *testing.T) {
	a, err := parseConfig(`forward { max_response_size 5000000 }`)
	if err != nil {
		t.Errorf("should parse: %v", err)
		return
	}
	if a.MaxResponseSize != 5000000 {
		t.Errorf("max_response_size should be 5000000, got %d", a.MaxResponseSize)
	}
}

func TestConfigParseUnknownOption(t *testing.T) {
	_, err := parseConfig(`forward { unknown_option 10s }`)
	if err == nil {
		t.Errorf("unknown option should error")
	}
}

func TestConfigParseInvalidTimeout(t *testing.T) {
	_, err := parseConfig(`forward { timeout not_a_duration }`)
	if err == nil {
		t.Errorf("invalid timeout should error")
	}
}

func TestConfigProvisionDefaults(t *testing.T) {
	a := &config.App{}
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	if err := a.Provision(ctx); err != nil {
		t.Errorf("Provision should succeed: %v", err)
	}
	if a.Timeout != caddy.Duration(config.DefaultTimeout) {
		t.Errorf("Timeout should default to %v, got %v", config.DefaultTimeout, a.Timeout)
	}
	if a.MaxResponseSize != config.DefaultMaxResponseSize {
		t.Errorf("MaxResponseSize should default to %d, got %d", config.DefaultMaxResponseSize, a.MaxResponseSize)
	}
}

func TestConfigProvisionKeepsCustomValues(t *testing.T) {
	a := &config.App{
		Timeout:         caddy.Duration(30 * time.Second),
		MaxResponseSize: 5000000,
	}
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	if err := a.Provision(ctx); err != nil {
		t.Errorf("Provision should succeed: %v", err)
	}
	if a.Timeout != caddy.Duration(30*time.Second) {
		t.Errorf("Timeout should keep 30s, got %v", a.Timeout)
	}
	if a.MaxResponseSize != 5000000 {
		t.Errorf("MaxResponseSize should keep 5000000, got %d", a.MaxResponseSize)
	}
}

func TestConfigValidateNegativeTimeout(t *testing.T) {
	a := &config.App{Timeout: caddy.Duration(-1 * time.Second)}
	err := a.Validate()
	if err == nil {
		t.Errorf("negative timeout should error")
	}
}

func TestConfigValidateNegativeMaxSize(t *testing.T) {
	a := &config.App{MaxResponseSize: -100}
	err := a.Validate()
	if err == nil {
		t.Errorf("negative max_response_size should error")
	}
}

func TestConfigValidateZeroValuesOK(t *testing.T) {
	a := &config.App{}
	err := a.Validate()
	if err != nil {
		t.Errorf("zero values should be valid: %v", err)
	}
}

// ── Forward directive parsing: top-level syntax ──────────────────────────

func TestCaddyfileParseGetSimple(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com"`)
	if err != nil {
		t.Errorf("should parse: %v", err)
		return
	}
	if h.Method != "GET" {
		t.Errorf("method should be GET, got %q", h.Method)
	}
	if h.URL != "http://example.com" {
		t.Errorf("URL should be http://example.com, got %q", h.URL)
	}
}

func TestCaddyfileParsePost(t *testing.T) {
	h, err := parseForwardBlock(`POST "http://example.com/api"`)
	if err != nil {
		t.Errorf("should parse POST: %v", err)
		return
	}
	if h.Method != "POST" {
		t.Errorf("method should be POST, got %q", h.Method)
	}
}

func TestCaddyfileParseAllMethods(t *testing.T) {
	methods := []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	for _, m := range methods {
		h, err := parseForwardBlock(m + ` "http://x"`)
		if err != nil {
			t.Errorf("%s should parse: %v", m, err)
			continue
		}
		if h.Method != m {
			t.Errorf("method should be %s, got %q", m, h.Method)
		}
	}
}

// ── Forward directive parsing: method case ───────────────────────────────

func TestCaddyfileParseMethodLowercase(t *testing.T) {
	h, err := parseForwardBlock(`get "http://example.com"`)
	if err != nil {
		t.Errorf("lowercase method should parse: %v", err)
		return
	}
	if h.Method != "GET" {
		t.Errorf("should uppercase to GET, got %q", h.Method)
	}
}

// ── Forward directive parsing: error cases ───────────────────────────────

func TestCaddyfileParseMissingMethod(t *testing.T) {
	_, err := parseForward(``)
	if err == nil {
		t.Errorf("missing method should error")
	}
}

func TestCaddyfileParseMissingURL(t *testing.T) {
	_, err := parseForwardBlock(`GET`)
	if err == nil {
		t.Errorf("missing URL should error")
	}
}

func TestCaddyfileParseInvalidMethod(t *testing.T) {
	_, err := parseForwardBlock(`INVALID "http://example.com"`)
	if err == nil {
		t.Errorf("invalid method should error")
	}
}

func TestCaddyfileParseConnectRejected(t *testing.T) {
	_, err := parseForwardBlock(`CONNECT "http://example.com"`)
	if err == nil {
		t.Errorf("CONNECT should be rejected")
	}
}

func TestCaddyfileParseTraceRejected(t *testing.T) {
	_, err := parseForwardBlock(`TRACE "http://example.com"`)
	if err == nil {
		t.Errorf("TRACE should be rejected")
	}
}

// ── Forward directive parsing: param subdirective ────────────────────────

func TestCaddyfileParseOneParam(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com" {
		param q value
	}`)
	if err != nil {
		t.Errorf("should parse param: %v", err)
		return
	}
	if len(h.Params) != 1 {
		t.Errorf("should have 1 param, got %d", len(h.Params))
	}
	if h.Params[0].Name != "q" {
		t.Errorf("param name should be 'q', got %q", h.Params[0].Name)
	}
	if h.Params[0].Value != "value" {
		t.Errorf("param value should be 'value', got %q", h.Params[0].Value)
	}
}

func TestCaddyfileParseMultipleParams(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com" {
		param a 1
		param b 2
	}`)
	if err != nil {
		t.Errorf("should parse multiple params: %v", err)
		return
	}
	if len(h.Params) != 2 {
		t.Errorf("should have 2 params, got %d", len(h.Params))
	}
}

// ── Forward directive parsing: header subdirective ───────────────────────

func TestCaddyfileParseOneHeader(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com" {
		header Authorization "Bearer x"
	}`)
	if err != nil {
		t.Errorf("should parse header: %v", err)
		return
	}
	if len(h.Headers) != 1 {
		t.Errorf("should have 1 header, got %d", len(h.Headers))
	}
	if h.Headers[0].Name != "Authorization" {
		t.Errorf("header name should be 'Authorization', got %q", h.Headers[0].Name)
	}
}

func TestCaddyfileParseMultipleHeaders(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com" {
		header X-Custom val1
		header X-Other val2
	}`)
	if err != nil {
		t.Errorf("should parse multiple headers: %v", err)
		return
	}
	if len(h.Headers) != 2 {
		t.Errorf("should have 2 headers, got %d", len(h.Headers))
	}
}

func TestCaddyfileParseHeaderMissingValue(t *testing.T) {
	_, err := parseForwardBlock(`GET "http://example.com" {
		header Authorization
	}`)
	if err == nil {
		t.Errorf("missing header value should error")
	}
}

// ── Forward directive parsing: body subdirective ─────────────────────────

func TestCaddyfileParseBodyOnPost(t *testing.T) {
	h, err := parseForwardBlock(`POST "http://example.com" {
		body "hello world"
	}`)
	if err != nil {
		t.Errorf("body on POST should parse: %v", err)
		return
	}
	if h.Body != "hello world" {
		t.Errorf("body should be 'hello world', got %q", h.Body)
	}
}

func TestCaddyfileParseBodyOnPut(t *testing.T) {
	h, err := parseForwardBlock(`PUT "http://example.com" {
		body "data"
	}`)
	if err != nil {
		t.Errorf("body on PUT should parse: %v", err)
		return
	}
	if h.Body != "data" {
		t.Errorf("body should be 'data', got %q", h.Body)
	}
}

func TestCaddyfileParseBodyOnGetRejected(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com" {
		body "data"
	}`)
	if err != nil {
		t.Errorf("parsing should succeed, got %v", err)
		return
	}
	if err := h.Validate(); err == nil {
		t.Errorf("Validate should reject body on GET")
	}
}

func TestCaddyfileParseBodyOnHeadRejected(t *testing.T) {
	h, err := parseForwardBlock(`HEAD "http://example.com" {
		body "data"
	}`)
	if err != nil {
		t.Errorf("parsing should succeed, got %v", err)
		return
	}
	if err := h.Validate(); err == nil {
		t.Errorf("Validate should reject body on HEAD")
	}
}

// ── Forward directive parsing: mixed subdirectives ───────────────────────

func TestCaddyfileParseParamAndHeader(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://example.com" {
		param q caddy
		header Authorization "Bearer x"
	}`)
	if err != nil {
		t.Errorf("should parse param+header: %v", err)
		return
	}
	if len(h.Params) != 1 || len(h.Headers) != 1 {
		t.Errorf("should have 1 param and 1 header")
	}
}

func TestCaddyfileParseBodyAndHeader(t *testing.T) {
	h, err := parseForwardBlock(`POST "http://example.com" {
		header Content-Type "text/plain"
		body "data"
	}`)
	if err != nil {
		t.Errorf("should parse header+body: %v", err)
		return
	}
	if h.Body != "data" || len(h.Headers) != 1 {
		t.Errorf("should have body and header")
	}
}

func TestCaddyfileParseUnknownSubdirective(t *testing.T) {
	_, err := parseForwardBlock(`GET "http://example.com" {
		unknown_directive value
	}`)
	if err == nil {
		t.Errorf("unknown subdirective should error")
	}
}

// ── Full e2e: parse → serve → verify response ───────────────────────────

func TestCaddyfileParseAndServe(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "test")
		w.WriteHeader(200)
		w.Write([]byte("e2e ok"))
	}))
	defer backend.Close()

	h, err := parseForwardBlock(`GET "` + backend.URL + `"`)
	if err != nil {
		t.Errorf("parse error: %v", err)
		return
	}

	// Set up requester (bypass Provision for test)
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	// Serve
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP error: %v", serveErr)
		return
	}

	resp := recorder.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status should be 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "e2e ok" {
		t.Errorf("body should be 'e2e ok', got %q", string(body))
	}
}

func TestCaddyfileParseAndServeWithParam(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=caddy") {
			t.Errorf("query should contain q=caddy, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(200)
		w.Write([]byte("param ok"))
	}))
	defer backend.Close()

	h, err := parseForwardBlock(`GET "` + backend.URL + `" {
		param q caddy
	}`)
	if err != nil {
		t.Errorf("parse error: %v", err)
		return
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP error: %v", serveErr)
		return
	}

	resp := recorder.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status should be 200, got %d", resp.StatusCode)
	}
}

func TestCaddyfileParseAndServeWithHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Errorf("should have X-Api-Key header, got %q", r.Header.Get("X-Api-Key"))
		}
		w.WriteHeader(200)
		w.Write([]byte("header ok"))
	}))
	defer backend.Close()

	h, err := parseForwardBlock(`GET "` + backend.URL + `" {
		header X-Api-Key secret
	}`)
	if err != nil {
		t.Errorf("parse error: %v", err)
		return
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP error: %v", serveErr)
		return
	}

	resp := recorder.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status should be 200, got %d", resp.StatusCode)
	}
}

func TestCaddyfileParseAndServePostWithBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "request-data" {
			t.Errorf("body should be 'request-data', got %q", string(b))
		}
		w.WriteHeader(201)
		w.Write(b)
	}))
	defer backend.Close()

	h, err := parseForwardBlock(`POST "` + backend.URL + `" {
		body "request-data"
	}`)
	if err != nil {
		t.Errorf("parse error: %v", err)
		return
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("ServeHTTP error: %v", serveErr)
		return
	}

	resp := recorder.Result()
	if resp.StatusCode != 201 {
		t.Errorf("status should be 201, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "request-data" {
		t.Errorf("body should echo 'request-data', got %q", string(body))
	}
}

func TestCaddyfileParseAndServeUpstream5xx(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte("bad upstream"))
	}))
	defer backend.Close()

	h, err := parseForwardBlock(`GET "` + backend.URL + `"`)
	if err != nil {
		t.Errorf("parse error: %v", err)
		return
	}
	h.SetRequester(subreq.NewRequester(5*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr != nil {
		t.Errorf("upstream error should not be a handler error: %v", serveErr)
		return
	}

	resp := recorder.Result()
	if resp.StatusCode != 502 {
		t.Errorf("status should be 502, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "bad upstream") {
		t.Errorf("body should contain upstream message, got %q", string(body))
	}
}

func TestCaddyfileParseAndServeNetworkError(t *testing.T) {
	h, err := parseForwardBlock(`GET "http://127.0.0.1:1/"`)
	if err != nil {
		t.Errorf("parse error: %v", err)
		return
	}
	h.SetRequester(subreq.NewRequester(1*time.Second, 10*1024*1024))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/", nil)
	serveErr := h.ServeHTTP(recorder, req, nil)
	if serveErr == nil {
		t.Errorf("network error should return caddyhttp.Error")
		return
	}
	if handlerErr, ok := serveErr.(caddyhttp.HandlerError); ok {
		if handlerErr.StatusCode != 502 {
			t.Errorf("should return 502, got %d", handlerErr.StatusCode)
		}
	} else {
		t.Errorf("error should be HandlerError, got %T", serveErr)
	}
}

// ── USE (forward USE path) ────────────────────────────────────────────────

func TestForwardUseParses(t *testing.T) {
	h, err := parseForwardBlock(`USE page.html`)
	if err != nil {
		t.Fatalf("forward USE should parse: %v", err)
	}
	if h.Method != MethodUse {
		t.Errorf("method should be USE, got %q", h.Method)
	}
	if h.URL != "page.html" {
		t.Errorf("url should be the template path, got %q", h.URL)
	}
	if err := h.Validate(); err != nil {
		t.Errorf("USE with just a path should validate: %v", err)
	}
}

func TestForwardUseParsesQueryAndBody(t *testing.T) {
	h, err := parseForwardBlock("USE page.html {\n\t\tparam q caddy\n\t\tbody hello\n\t}")
	if err != nil {
		t.Fatalf("forward USE with query and body should parse: %v", err)
	}
	if len(h.Params) != 1 || h.Params[0].Name != "q" || h.Params[0].Value != "caddy" {
		t.Errorf("param should parse, got %+v", h.Params)
	}
	if h.Body != "hello" {
		t.Errorf("body should parse, got %q", h.Body)
	}
	if err := h.Validate(); err != nil {
		t.Errorf("USE with query and body should validate: %v", err)
	}
}
