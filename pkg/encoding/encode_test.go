package encoding

import (
	"strings"
	"testing"
)

func TestEncodeFormPairs(t *testing.T) {
	d := map[string]any{"a": "1", "b": "2"}
	result, err := EncodeForm(d)
	if err != nil {
		t.Errorf("EncodeForm should succeed: %v", err)
	}
	if !strings.Contains(result, "a=1") || !strings.Contains(result, "b=2") {
		t.Errorf("output should contain both key=value pairs, got %q", result)
	}
	if !strings.Contains(result, "&") {
		t.Errorf("pairs should be &-separated, got %q", result)
	}
}

func TestEncodeFormSpecialChars(t *testing.T) {
	d := map[string]any{"key": "hello world"}
	result, err := EncodeForm(d)
	if err != nil {
		t.Errorf("EncodeForm should succeed: %v", err)
	}
	if result == "" {
		t.Errorf("EncodeForm should produce output for special chars")
	}
	if strings.Contains(result, " ") {
		t.Errorf("output should URL-encode spaces, got %q", result)
	}
}

func TestEncodeFormEmpty(t *testing.T) {
	result, err := EncodeForm(map[string]any{})
	if err != nil {
		t.Errorf("EncodeForm should succeed on empty dict: %v", err)
	}
	if result != "" {
		t.Errorf("EncodeForm should return empty string for empty dict, got %q", result)
	}
}

func TestHasBinaryTrue(t *testing.T) {
	d := map[string]any{"a": []byte{1, 2, 3}}
	if !HasBinary(d) {
		t.Errorf("HasBinary should detect []byte values")
	}
}

func TestHasBinaryFalse(t *testing.T) {
	d2 := map[string]any{"a": "string"}
	if HasBinary(d2) {
		t.Errorf("HasBinary should return false for string values")
	}
}

func TestHasBinaryEmpty(t *testing.T) {
	if HasBinary(map[string]any{}) {
		t.Errorf("HasBinary should return false for empty dict")
	}
}

func TestHasBinaryMixed(t *testing.T) {
	d := map[string]any{"a": "text", "b": []byte{0x01}}
	if !HasBinary(d) {
		t.Errorf("HasBinary should detect []byte even among strings")
	}
}

func TestEncodeMultipartField(t *testing.T) {
	d := map[string]any{"field": "value"}
	body, ct, err := EncodeMultipart(d)
	if err != nil {
		t.Errorf("EncodeMultipart should succeed: %v", err)
	}
	if len(body) <= 0 {
		t.Errorf("body should not be empty")
	}
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Errorf("Content-Type should start with multipart/form-data, got %q", ct)
	}
	if !strings.Contains(ct, "boundary=") {
		t.Errorf("Content-Type should include boundary=, got %q", ct)
	}
	if !strings.Contains(string(body), "field") {
		t.Errorf("body should contain field name")
	}
}

func TestEncodeMultipartBinary(t *testing.T) {
	d := map[string]any{"file": []byte{0x01, 0x02, 0x03}}
	body, ct, err := EncodeMultipart(d)
	if err != nil {
		t.Errorf("EncodeMultipart should succeed with binary: %v", err)
	}
	if len(body) <= 0 {
		t.Errorf("body should not be empty")
	}
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Errorf("Content-Type should start with multipart/form-data, got %q", ct)
	}
}

func TestEncodeMultipartEmpty(t *testing.T) {
	body, ct, err := EncodeMultipart(map[string]any{})
	if err != nil {
		t.Errorf("EncodeMultipart should succeed on empty dict: %v", err)
	}
	// Empty multipart body may still have boundary markers
	if len(body) <= 0 {
		t.Errorf("body should not be empty for multipart even with empty dict")
	}
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Errorf("Content-Type should start with multipart/form-data, got %q", ct)
	}
}

func TestMergeQueryBothNonEmpty(t *testing.T) {
	result := MergeQuery("a=1", "b=2")
	if result != "a=1&b=2" {
		t.Errorf("should append with &: got %q", result)
	}
}

func TestMergeQueryEmptyExisting(t *testing.T) {
	result := MergeQuery("", "b=2")
	if result != "b=2" {
		t.Errorf("should return incoming when existing is empty: got %q", result)
	}
}

func TestMergeQueryEmptyIncoming(t *testing.T) {
	result := MergeQuery("a=1", "")
	if result != "a=1" {
		t.Errorf("should return existing when incoming is empty: got %q", result)
	}
}

func TestMergeQueryBothEmpty(t *testing.T) {
	result := MergeQuery("", "")
	if result != "" {
		t.Errorf("should return empty when both empty: got %q", result)
	}
}

func TestMergeQueryVerbatim(t *testing.T) {
	// Values are used verbatim — no URL-encoding applied
	result := MergeQuery("a=1", "b=c d")
	if result != "a=1&b=c d" {
		t.Errorf("should keep incoming verbatim: got %q", result)
	}
}

func TestMergeQueryDictAddKeys(t *testing.T) {
	result, err := MergeQueryDict("a=1", map[string]any{"b": "2"})
	if err != nil {
		t.Errorf("MergeQueryDict should succeed: %v", err)
	}
	if !strings.Contains(result, "a=1") || !strings.Contains(result, "b=2") {
		t.Errorf("should contain both keys, got %q", result)
	}
}

func TestMergeQueryDictOverwrite(t *testing.T) {
	result, err := MergeQueryDict("a=1&b=2", map[string]any{"a": "3"})
	if err != nil {
		t.Errorf("MergeQueryDict should succeed: %v", err)
	}
	// 'a' should be overwritten to '3', 'b' should remain
	if strings.Contains(result, "a=1") {
		t.Errorf("a should be overwritten, got %q", result)
	}
	if !strings.Contains(result, "a=3") {
		t.Errorf("a should be updated to 3, got %q", result)
	}
	if !strings.Contains(result, "b=2") {
		t.Errorf("b should remain, got %q", result)
	}
}

func TestMergeQueryDictEmptyExisting(t *testing.T) {
	result, err := MergeQueryDict("", map[string]any{"a": "1"})
	if err != nil {
		t.Errorf("MergeQueryDict should succeed with empty existing: %v", err)
	}
	if result != "a=1" {
		t.Errorf("should return new key when existing is empty, got %q", result)
	}
}

func TestMergeQueryDictEmptyNew(t *testing.T) {
	result, err := MergeQueryDict("a=1", map[string]any{})
	if err != nil {
		t.Errorf("MergeQueryDict should succeed with empty dict: %v", err)
	}
	if !strings.Contains(result, "a=1") {
		t.Errorf("should keep existing keys, got %q", result)
	}
}