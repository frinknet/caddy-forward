// Package caddyforward imports the subpackages so their init() functions
// register the plugin's modules with Caddy.
package caddyforward

import (
	_ "github.com/frinknet/caddy-forward/pkg/config"
	_ "github.com/frinknet/caddy-forward/pkg/forward"
	_ "github.com/frinknet/caddy-forward/pkg/template"
)
