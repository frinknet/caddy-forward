package caddyforward

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/you/caddy-forward/pkg/subreq"
)

// Helper: shared requester with generous limits.
func testRequester() *subreq.Requester {
	return subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024)
}

// Helper: inspect incoming request with a dict body for form/multipart analysis.
type inspected struct {
	method     string
	url        string
	headers    http.Header
	body       string
	contentType string
}

// Helper: make a server that records the incoming request for inspection.
func recorderServer() (*httptest.Server, *inspected) {
	info := &inspected{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info.method = r.Method
		info.url = r.URL.String()
		info.headers = r.Header
		b, _ := io.ReadAll(r.Body)
		info.body = string(b)
		ct := r.Header.Get("Content-Type")
		if ct != "" {
			info.contentType = ct
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	return server, info
}

// ── Query-group method tests ──────────────────────────────────────────────

func TestSendGetNilData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "GET", server.URL, nil, nil)
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.method != "GET" {
		t.Errorf("expected GET, got %s", info.method)
	}
	if strings.Contains(info.url, "?") {
		t.Errorf("nil data should not add query, got %s", info.url)
	}
}

func TestSendGetStringData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "GET", server.URL, nil, "custom=param")
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if !strings.Contains(info.url, "custom=param") {
		t.Errorf("string data should appear in URL, got %s", info.url)
	}
}

func TestSendGetDictData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "GET", server.URL, nil, map[string]any{"q": "caddy"})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if !strings.Contains(info.url, "q=caddy") {
		t.Errorf("dict data should become query params, got %s", info.url)
	}
}

func TestSendGetWithExistingQuery(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	url := server.URL + "?existing=1"
	resp := r.Send(context.Background(), "GET", url, nil, map[string]any{"new": "2"})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if !strings.Contains(info.url, "existing=1") || !strings.Contains(info.url, "new=2") {
		t.Errorf("should merge both existing and new query params, got %s", info.url)
	}
}

func TestSendHeadReturnsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(200)
	}))
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "HEAD", server.URL, nil, nil)
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if resp.Body != "" {
		t.Errorf("HEAD response body should be empty, got %q", resp.Body)
	}
	if resp.Headers.Get("Content-Length") != "42" {
		t.Errorf("headers should be preserved, got %q", resp.Headers.Get("Content-Length"))
	}
}

func TestSendHeadWithQueryData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "HEAD", server.URL, nil, map[string]any{"a": "1"})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if !strings.Contains(info.url, "a=1") {
		t.Errorf("HEAD should accept query data, got %s", info.url)
	}
}

func TestSendDeleteWithQueryData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "DELETE", server.URL, nil, map[string]any{"id": "42"})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if !strings.Contains(info.url, "id=42") {
		t.Errorf("DELETE should accept query data, got %s", info.url)
	}
}

func TestSendOptionsWithNilData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "OPTIONS", server.URL, nil, nil)
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.method != "OPTIONS" {
		t.Errorf("expected OPTIONS, got %s", info.method)
	}
}

// ── Body-group method tests ───────────────────────────────────────────────

func TestSendPostStringBody(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "POST", server.URL, nil, "hello")
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.body != "hello" {
		t.Errorf("body should be 'hello', got %q", info.body)
	}
	if info.contentType != "text/plain" {
		t.Errorf("default Content-Type should be text/plain, got %q", info.contentType)
	}
}

func TestSendPutStringBody(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "PUT", server.URL, nil, "data")
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.body != "data" {
		t.Errorf("body should be 'data', got %q", info.body)
	}
}

func TestSendPatchStringBody(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "PATCH", server.URL, nil, "update")
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.body != "update" {
		t.Errorf("body should be 'update', got %q", info.body)
	}
}

func TestSendPostDictFormEncoded(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "POST", server.URL, nil, map[string]any{"a": "1", "b": "2"})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.body == "" {
		t.Errorf("body should not be empty")
	}
	if !strings.HasPrefix(info.contentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type should be form-encoded, got %q", info.contentType)
	}
	if !strings.Contains(info.body, "a=1") || !strings.Contains(info.body, "b=2") {
		t.Errorf("body should contain form fields, got %q", info.body)
	}
}

func TestSendPostDictMultipartForBinary(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "POST", server.URL, nil, map[string]any{"file": []byte{0x01, 0x02}})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.body == "" {
		t.Errorf("body should not be empty")
	}
	if !strings.HasPrefix(info.contentType, "multipart/form-data") {
		t.Errorf("binary dict should force multipart, got %q", info.contentType)
	}
}

func TestSendPostDictWithExplicitContentType(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	resp := r.Send(context.Background(), "POST", server.URL, headers, map[string]any{"a": "1"})
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if !strings.HasPrefix(info.contentType, "application/x-www-form-urlencoded") {
		t.Errorf("should respect explicit Content-Type, got %q", info.contentType)
	}
}

func TestSendPostDictAmbiguousContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	r := testRequester()
	headers := map[string]string{"Content-Type": "application/json"}
	resp := r.Send(context.Background(), "POST", server.URL, headers, map[string]any{"a": "1"})
	if resp.Err == nil {
		t.Errorf("ambiguous Content-Type should set Err")
	}
}

func TestSendPostWithStringAndExplicitContentType(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	headers := map[string]string{"Content-Type": "application/json"}
	resp := r.Send(context.Background(), "POST", server.URL, headers, `{"hello":"world"}`)
	if resp.Err != nil {
		t.Errorf("string body with explicit Content-Type should succeed: %v", resp.Err)
	}
	if info.contentType != "application/json" {
		t.Errorf("should use explicit Content-Type, got %q", info.contentType)
	}
}

// ── Header passthrough tests ──────────────────────────────────────────────

func TestSendCustomHeaders(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	headers := map[string]string{
		"Authorization": "Bearer token123",
		"X-Custom":      "value",
	}
	resp := r.Send(context.Background(), "GET", server.URL, headers, nil)
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if info.headers.Get("Authorization") != "Bearer token123" {
		t.Errorf("should forward Authorization header, got %q", info.headers.Get("Authorization"))
	}
	if info.headers.Get("X-Custom") != "value" {
		t.Errorf("should forward X-Custom header, got %q", info.headers.Get("X-Custom"))
	}
}

// ── Error-path tests ──────────────────────────────────────────────────────

func TestSendNetworkFailure(t *testing.T) {
	r := subreq.NewRequester(1 * time.Second, 10 * 1024 * 1024)
	resp := r.Send(context.Background(), "GET", "http://127.0.0.1:1/nonexistent", nil, nil)
	if resp.Err == nil {
		t.Errorf("network failure should set Err")
	}
	if resp.Status != 0 {
		t.Errorf("status should be 0 on failure")
	}
	if resp.Body != "" {
		t.Errorf("body should be empty on failure")
	}
}

func TestSendResponseExceedsMaxSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("too large"))
	}))
	defer server.Close()

	r := subreq.NewRequester(5 * time.Second, 5) // max 5 bytes
	resp := r.Send(context.Background(), "GET", server.URL, nil, nil)
	if resp.Err == nil {
		t.Errorf("oversize response should set Err")
	}
}

func TestSendInvalidURL(t *testing.T) {
	r := testRequester()
	resp := r.Send(context.Background(), "GET", "://invalid", nil, nil)
	if resp.Err == nil {
		t.Errorf("invalid URL should set Err")
	}
	if resp.Status != 0 {
		t.Errorf("status should be 0 on invalid URL")
	}
}

func TestSendInvalidHeadersType(t *testing.T) {
	r := testRequester()
	resp := r.Send(context.Background(), "GET", "http://example.com", "not-a-header", nil)
	if resp.Err == nil {
		t.Errorf("invalid headers type should set Err")
	}
}

func TestSendInvalidDataType(t *testing.T) {
	r := testRequester()
	resp := r.Send(context.Background(), "POST", "http://example.com", nil, 42)
	if resp.Err == nil {
		t.Errorf("invalid data type should set Err")
	}
}

func TestSendPostNilBody(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "POST", server.URL, nil, nil)
	if resp.Err != nil {
		t.Errorf("nil body should succeed: %v", resp.Err)
	}
	if info.body != "" {
		t.Errorf("nil data should send empty body, got %q", info.body)
	}
}

// ── Response property tests ───────────────────────────────────────────────

func TestSendResponseStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte("created"))
	}))
	defer server.Close()

	r := testRequester()
	resp := r.Send(context.Background(), "POST", server.URL, nil, "data")
	if resp.Err != nil {
		t.Errorf("should succeed: %v", resp.Err)
	}
	if resp.Status != 201 {
		t.Errorf("status should be 201, got %d", resp.Status)
	}
	if resp.Body != "created" {
		t.Errorf("body should be 'created', got %q", resp.Body)
	}
}

func TestSendResponseString(t *testing.T) {
	r := testRequester()
	resp := r.Send(context.Background(), "GET", "http://127.0.0.1:1/x", nil, nil)
	// On failure, String() returns body (which is empty)
	s := fmt.Sprintf("%v", resp)
	if s != resp.Body {
		t.Errorf("String() should return body, got %q", s)
	}
}

func TestSendMapStringStringData(t *testing.T) {
	server, info := recorderServer()
	defer server.Close()

	r := testRequester()
	// map[string]string should also work as data
	resp := r.Send(context.Background(), "POST", server.URL, nil, map[string]string{"a": "1"})
	if resp.Err != nil {
		t.Errorf("map[string]string data should succeed: %v", resp.Err)
	}
	if info.body == "" {
		t.Errorf("body should not be empty with map[string]string data")
	}
}

func TestMergeQueryIntoURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		qs       string
		expected string
	}{
		{"append to empty query", "http://example.com/endpoint", "a=1", "http://example.com/endpoint?a=1"},
		{"append to existing query", "http://example.com/endpoint?existing=1", "new=2", "http://example.com/endpoint?existing=1&new=2"},
		{"empty qs returns original", "http://example.com/endpoint?q=v", "", "http://example.com/endpoint?q=v"},
		{"invalid url returns original", "://invalid", "a=1", "://invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := subreq.MergeQueryIntoURL(tt.rawURL, tt.qs)
			if result != tt.expected {
				t.Errorf("MergeQueryIntoURL(%q, %q) = %q, want %q", tt.rawURL, tt.qs, result, tt.expected)
			}
		})
	}
}