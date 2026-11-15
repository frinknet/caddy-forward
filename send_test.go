package caddyforward

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/caddy-forward/pkg/subreq"
)

func TestSendGetWithQueryDict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method should be GET, got %s", r.Method)
		}
		if r.URL.RawQuery == "" {
			t.Errorf("should have query string")
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	r := subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024)
	resp := r.Send(context.Background(), "GET", server.URL, nil, map[string]any{"q": "caddy"})
	if resp.Err != nil {
		t.Errorf("Send should succeed: %v", resp.Err)
	}
	if resp.Status != 200 {
		t.Errorf("status should be 200, got %d", resp.Status)
	}

	server.Close()
}

func TestSendPostWithStringBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method should be POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "hello" {
			t.Errorf("body should be 'hello', got %q", string(body))
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	r := subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024)
	resp := r.Send(context.Background(), "POST", server.URL, nil, "hello")
	if resp.Err != nil {
		t.Errorf("Send should succeed: %v", resp.Err)
	}
	if resp.Status != 200 {
		t.Errorf("status should be 200, got %d", resp.Status)
	}

	server.Close()
}

func TestSendHeadReturnsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("method should be HEAD, got %s", r.Method)
		}
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(200)
	}))

	r := subreq.NewRequester(5 * time.Second, 10 * 1024 * 1024)
	resp := r.Send(context.Background(), "HEAD", server.URL, nil, nil)
	if resp.Err != nil {
		t.Errorf("Send should succeed: %v", resp.Err)
	}
	if resp.Body != "" {
		t.Errorf("HEAD response body should be empty, got %q", resp.Body)
	}
	if resp.Headers.Get("Content-Length") != "42" {
		t.Errorf("headers should be preserved, got %q", resp.Headers.Get("Content-Length"))
	}

	server.Close()
}

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

	r := subreq.NewRequester(5 * time.Second, 5) // max 5 bytes
	resp := r.Send(context.Background(), "GET", server.URL, nil, nil)
	if resp.Err == nil {
		t.Errorf("oversize response should set Err")
	}

	server.Close()
}