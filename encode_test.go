package caddyforward

import (
	"testing"

	"github.com/you/caddy-forward/pkg/encoding"
)

func TestEncodeForm(t *testing.T) {
	d := map[string]any{"a": "1", "b": "2"}
	result, err := encoding.EncodeForm(d)
	if err != nil {
		t.Errorf("EncodeForm should succeed: %v", err)
	}
	if result == "" {
		t.Errorf("EncodeForm should produce output")
	}
}

func TestHasBinary(t *testing.T) {
	d := map[string]any{"a": []byte{1, 2, 3}}
	if !encoding.HasBinary(d) {
		t.Errorf("HasBinary should detect []byte values")
	}

	d2 := map[string]any{"a": "string"}
	if encoding.HasBinary(d2) {
		t.Errorf("HasBinary should return false for string values")
	}
}

func TestEncodeMultipart(t *testing.T) {
	d := map[string]any{"field": "value"}
	body, ct, err := encoding.EncodeMultipart(d)
	if err != nil {
		t.Errorf("EncodeMultipart should succeed: %v", err)
	}
	if len(body) <= 0 {
		t.Errorf("body should not be empty")
	}
	if ct == "" {
		t.Errorf("Content-Type should be set")
	}
}

func TestMergeQuery(t *testing.T) {
	result := encoding.MergeQuery("a=1", "b=2")
	if result != "a=1&b=2" {
		t.Errorf("should append with &: got %q", result)
	}

	result = encoding.MergeQuery("", "b=2")
	if result != "b=2" {
		t.Errorf("should return incoming when existing is empty: got %q", result)
	}

	result = encoding.MergeQuery("a=1", "")
	if result != "a=1" {
		t.Errorf("should return existing when incoming is empty: got %q", result)
	}
}

func TestMergeQueryDict(t *testing.T) {
	result, err := encoding.MergeQueryDict("a=1", map[string]any{"b": "2"})
	if err != nil {
		t.Errorf("MergeQueryDict should succeed: %v", err)
	}
	if result == "" {
		t.Errorf("should produce output")
	}
}