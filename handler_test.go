package caddyforward

import (
	"testing"

	"github.com/you/caddy-forward/pkg/forward"
)

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