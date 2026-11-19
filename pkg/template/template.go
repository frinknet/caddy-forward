package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	caddytpl "github.com/caddyserver/caddy/v2/modules/caddyhttp/templates"
	"github.com/frinknet/caddy-forward/pkg/config"
	"github.com/frinknet/caddy-forward/pkg/subreq"
)

func init() {
	caddy.RegisterModule(Functions{})
}

// Functions implements caddyhttp.CustomFunctions, exposing GET/HEAD/POST/PUT/
// PATCH/DELETE/OPTIONS plus USE as template functions.
type Functions struct {
	ctx       context.Context
	requester *subreq.Requester
}

func (Functions) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.templates.functions.forward_methods",
		New: func() caddy.Module { return new(Functions) },
	}
}

func (f *Functions) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	return nil
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

	f.ctx = context.Background()
	if ctx.Context != nil {
		f.ctx = ctx
	}
	f.requester = subreq.NewRequester(timeout, maxSize)
	return nil
}

func (f *Functions) CustomTemplateFunctions() template.FuncMap {
	return MethodFuncs(f.ctx, f.requester)
}

// MethodFuncs builds the template functions exposed under the
// `forward_methods` extension: one per HTTP method, plus USE. Calls use ctx,
// so canceling it cancels an in-flight outbound request.
func MethodFuncs(ctx context.Context, r *subreq.Requester) template.FuncMap {
	if ctx == nil {
		ctx = context.Background()
	}
	send := func(method string) func(string, any, any) subreq.Response {
		return func(url string, headers, data any) subreq.Response {
			return r.Send(ctx, method, url, headers, data)
		}
	}
	return template.FuncMap{
		"GET":     send("GET"),
		"HEAD":    send("HEAD"),
		"POST":    send("POST"),
		"PUT":     send("PUT"),
		"PATCH":   send("PATCH"),
		"DELETE":  send("DELETE"),
		"OPTIONS": send("OPTIONS"),
		"USE":     Use,
	}
}

// Use renders another template file in place, like `include`, but it carries
// the parent template's context through so existing variables are already in
// scope without remapping them.
//
// The parent context must be passed explicitly, since Go template functions
// never see the implicit dot:
//
//	{{ USE "partials/row.html" . }}
//	{{ . | USE "partials/row.html" }}
//
// To give the child a different dot while still resolving files against the
// site root, pass the context first and the child data last:
//
//	{{ . | USE "partials/row.html" $ctx }}
func Use(filename string, args ...any) (string, error) {
	var ctx *caddytpl.TemplateContext
	var data any
	for _, arg := range args {
		if c, ok := arg.(*caddytpl.TemplateContext); ok && ctx == nil {
			ctx = c
			continue
		}
		data = arg
	}
	if ctx == nil {
		return "", fmt.Errorf("USE %s: needs the template context; call it as {{ USE %q . }}", filename, filename)
	}
	if data == nil {
		data = ctx
	}
	return RenderFile(ctx, filename, data)
}

// RenderFile parses and executes the template file at path, using data as the
// child's dot. All built-in Caddy functions and the context's custom functions
// (including GET/POST/... and USE) are available to the child.
func RenderFile(ctx *caddytpl.TemplateContext, path string, data any) (string, error) {
	if ctx.Root == nil {
		return "", fmt.Errorf("forward: no file root configured for %s", path)
	}

	file, err := ctx.Root.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	src, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	local := *ctx
	tpl := local.NewTemplate(path)
	if _, err := tpl.Parse(string(src)); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var (
	_ caddy.Module             = (*Functions)(nil)
	_ caddy.Provisioner        = (*Functions)(nil)
	_ caddyfile.Unmarshaler    = (*Functions)(nil)
	_ caddytpl.CustomFunctions = (*Functions)(nil)
)
