package template

import (
	"context"
	"text/template"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/templates"
	"github.com/you/caddy-forward/pkg/config"
	"github.com/you/caddy-forward/pkg/subreq"
)

func init() {
	caddy.RegisterModule(Functions{})
}

// Functions implements caddyhttp.CustomFunctions, exposing GET/HEAD/POST/PUT/
// PATCH/DELETE/OPTIONS as template functions.
type Functions struct {
	requester *subreq.Requester
}

func (Functions) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.template_functions.methods",
		New: func() caddy.Module { return new(Functions) },
	}
}

func (f *Functions) Provision(ctx caddy.Context) error {
	timeout := config.DefaultTimeout
	maxSize := int64(config.DefaultMaxResponseSize)

	if appIface, err := ctx.App("forward"); err == nil {
		if app, ok := appIface.(*config.App); ok {
			timeout = time.Duration(app.Timeout)
			maxSize = app.MaxResponseSize
		}
	}

	f.requester = subreq.NewRequester(timeout, maxSize)
	return nil
}

func (f *Functions) CustomTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		"GET":     func(url string, headers, data any) subreq.Response { return f.send("GET", url, headers, data) },
		"HEAD":    func(url string, headers, data any) subreq.Response { return f.send("HEAD", url, headers, data) },
		"POST":    func(url string, headers, data any) subreq.Response { return f.send("POST", url, headers, data) },
		"PUT":     func(url string, headers, data any) subreq.Response { return f.send("PUT", url, headers, data) },
		"PATCH":   func(url string, headers, data any) subreq.Response { return f.send("PATCH", url, headers, data) },
		"DELETE":  func(url string, headers, data any) subreq.Response { return f.send("DELETE", url, headers, data) },
		"OPTIONS": func(url string, headers, data any) subreq.Response { return f.send("OPTIONS", url, headers, data) },
	}
}

func (f *Functions) send(method, url string, headers, data any) subreq.Response {
	return f.requester.Send(context.Background(), method, url, headers, data)
}

var (
	_ caddy.Module              = (*Functions)(nil)
	_ caddy.Provisioner         = (*Functions)(nil)
	_ templates.CustomFunctions = (*Functions)(nil)
)