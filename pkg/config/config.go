package config

import (
	"fmt"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

const (
	DefaultTimeout         = 10 * time.Second
	DefaultMaxResponseSize = 10 * 1024 * 1024 // 10 MiB
)

func init() {
	caddy.RegisterModule(App{})
	httpcaddyfile.RegisterGlobalOption("forward", parseGlobalForward)
}

func parseGlobalForward(d *caddyfile.Dispenser, existingVal any) (any, error) {
	app := &App{}
	if err := app.UnmarshalCaddyfile(d); err != nil {
		return nil, err
	}
	return app, nil
}

// App is the optional global app module `forward`.
//
//	{
//	    forward {
//	        timeout           10s
//	        max_response_size 10485760
//	    }
//	}
type App struct {
	Timeout         caddy.Duration `json:"timeout,omitempty"`
	MaxResponseSize int64          `json:"max_response_size,omitempty"`
}

func (App) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "forward",
		New: func() caddy.Module { return new(App) },
	}
}

func (a *App) Provision(ctx caddy.Context) error {
	if a.Timeout <= 0 {
		a.Timeout = caddy.Duration(DefaultTimeout)
	}
	if a.MaxResponseSize <= 0 {
		a.MaxResponseSize = DefaultMaxResponseSize
	}
	return nil
}

func (a *App) Validate() error {
	if a.Timeout < 0 {
		return fmt.Errorf("forward: timeout must be non-negative")
	}
	if a.MaxResponseSize < 0 {
		return fmt.Errorf("forward: max_response_size must be non-negative")
	}
	return nil
}

func (a *App) Start() error { return nil }
func (a *App) Stop() error  { return nil }

// UnmarshalCaddyfile parses the global options block.
func (a *App) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "timeout":
				if !d.NextArg() {
					return d.ArgErr()
				}
				dur, err := time.ParseDuration(d.Val())
				if err != nil {
					return d.Errf("invalid timeout: %v", err)
				}
				a.Timeout = caddy.Duration(dur)
			case "max_response_size":
				if !d.NextArg() {
					return d.ArgErr()
				}
				size, err := strconv.ParseInt(d.Val(), 10, 64)
				if err != nil {
					return d.Errf("invalid max_response_size: %v", err)
				}
				a.MaxResponseSize = size
			default:
				return d.Errf("unrecognized option: %s", d.Val())
			}
		}
	}
	return nil
}

var (
	_ caddy.App             = (*App)(nil)
	_ caddy.Module          = (*App)(nil)
	_ caddy.Provisioner     = (*App)(nil)
	_ caddy.Validator       = (*App)(nil)
	_ caddyfile.Unmarshaler = (*App)(nil)
)