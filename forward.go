// Package caddyforward imports the subpackages so their init() functions
// register the plugin's modules with Caddy.
package caddyforward

import (
	_ "github.com/you/caddy-forward/pkg/config"
	_ "github.com/you/caddy-forward/pkg/forward"
	_ "github.com/you/caddy-forward/pkg/template"
)