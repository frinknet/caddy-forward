package caddyforward

import (
	"bytes"
	"fmt"
	"text/template"
	"testing"

	"github.com/you/caddy-forward/pkg/subreq"
)

func TestTemplateGetRendersBody(t *testing.T) {
	funcs := template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "hello from test", Status: 200}
		},
	}

	tmpl := template.New("test").Funcs(funcs)
	tmpl, parseErr := tmpl.Parse("{{ GET \"http://example.com\" nil nil }}")
	if parseErr != nil {
		t.Errorf("Parse should succeed: %v", parseErr)
		return
	}
	buf := &bytes.Buffer{}
	execErr := tmpl.Execute(buf, nil)
	if execErr != nil {
		t.Errorf("Execute should succeed: %v", execErr)
		return
	}
	if string(buf.Bytes()) != "hello from test" {
		t.Errorf("should render the body, got %q", string(buf.Bytes()))
	}
}

func TestTemplateGetWithDict(t *testing.T) {
	funcs := template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "ok", Status: 200}
		},
	}

	tmpl := template.New("test").Funcs(funcs)
	tmpl, parseErr := tmpl.Parse("{{ GET \"http://example.com\" nil nil }}")
	if parseErr != nil {
		t.Errorf("Parse should succeed: %v", parseErr)
		return
	}
	buf := &bytes.Buffer{}
	execErr := tmpl.Execute(buf, nil)
	if execErr != nil {
		t.Errorf("Execute should succeed: %v", execErr)
		return
	}
	if string(buf.Bytes()) != "ok" {
		t.Errorf("should render with piped data, got %q", string(buf.Bytes()))
	}
}

func TestTemplateAccessStatus(t *testing.T) {
	funcs := template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "body", Status: 201}
		},
	}

	tmpl := template.New("test").Funcs(funcs)
	tmpl, parseErr := tmpl.Parse("{{ (GET \"http://example.com\" nil nil).Status }}")
	if parseErr != nil {
		t.Errorf("Parse should succeed: %v", parseErr)
		return
	}
	buf := &bytes.Buffer{}
	execErr := tmpl.Execute(buf, nil)
	if execErr != nil {
		t.Errorf("Execute should succeed: %v", execErr)
		return
	}
	if string(buf.Bytes()) != "201" {
		t.Errorf("should render the status code, got %q", string(buf.Bytes()))
	}
}

func TestTemplateContinuesOnError(t *testing.T) {
	funcs := template.FuncMap{
		"GET": func(url string, headers, data any) subreq.Response {
			return subreq.Response{Body: "", Status: 0, Err: fmt.Errorf("network error")}
		},
	}

	tmpl := template.New("test").Funcs(funcs)
	tmpl, parseErr := tmpl.Parse("before{{ GET \"http://example.com\" nil nil }}after")
	if parseErr != nil {
		t.Errorf("Parse should succeed: %v", parseErr)
		return
	}
	buf := &bytes.Buffer{}
	execErr := tmpl.Execute(buf, nil)
	if execErr != nil {
		t.Errorf("Execute should continue on sub-request error: %v", execErr)
		return
	}
	if string(buf.Bytes()) != "beforeafter" {
		t.Errorf("should concatenate around error, got %q", string(buf.Bytes()))
	}
}