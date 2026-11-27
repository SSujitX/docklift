// Package terminal serves the WebSocket terminal at /ws/terminal (the port of
// backend/src/services/terminal.ts).
//
// The WebSocket handler is owned by the agent porting terminal.ts; until then
// Attach registers nothing and /ws/terminal answers 404.
package terminal

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Attach registers the terminal endpoint on the root router r. It runs outside
// the HTTP middleware stack (no body parsing, rate limits or maintenance
// gate), exactly like the Node upgrade listener did; it must check the Origin
// (httpx.IsTrustedOrigin with Config.FrontendURL and CORS_ORIGIN, or
// Config.TerminalAllowNoOrigin) and the `?token=` terminal purpose token
// (Auth.VerifyPurposeToken) itself, and register session cleanup with
// d.Lifecycle.OnShutdown.
func Attach(r chi.Router, d *app.Deps) {}
