package forward

import (
	"strings"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/you/caddy-forward/pkg/subreq"
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("forward", parseCaddyfile)
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var f ForwardHandler
	if err := f.UnmarshalCaddyfile(h.Dispenser); err != nil {
		return nil, err
	}
	return &f, nil
}

// UnmarshalCaddyfile parses:
//
//	forward METHOD url {
//	    param  NAME VALUE
//	    header NAME VALUE
//	    body   STRING
//	}
func (h *ForwardHandler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	if !d.Next() {
		return d.ArgErr()
	}

	if !d.NextArg() {
		return d.ArgErr()
	}
	method := strings.ToUpper(d.Val())
	if !validMethod(method) {
		return d.Errf("invalid method: %s", d.Val())
	}
	h.Method = method

	if !d.NextArg() {
		return d.ArgErr()
	}
	h.URL = d.Val()

	if d.NextArg() {
		return d.ArgErr()
	}

	for d.NextBlock(0) {
		switch d.Val() {
		case "param":
			var name, value string
			if !d.Args(&name, &value) {
				return d.ArgErr()
			}
			h.Params = append(h.Params, Param{Name: name, Value: value})
		case "header":
			var name, value string
			if !d.Args(&name, &value) {
				return d.ArgErr()
			}
			h.Headers = append(h.Headers, Header{Name: name, Value: value})
		case "body":
			if !d.NextArg() {
				return d.ArgErr()
			}
			h.Body = d.Val()
			if d.NextArg() {
				return d.ArgErr()
			}
		default:
			return d.Errf("unrecognized subdirective: %s", d.Val())
		}
	}

	return nil
}

func validMethod(m string) bool {
	return subreq.IsQueryMethod(m) || subreq.IsBodyMethod(m)
}

var (
	_ caddyfile.Unmarshaler = (*ForwardHandler)(nil)
)