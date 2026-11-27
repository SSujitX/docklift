// Package terminal serves the WebSocket terminal at /ws/terminal (the
// upgrade listener of backend/src/services/terminal.ts). The sessions
// themselves run in internal/terminal.
package terminal

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"docklift/internal/app"
	"docklift/internal/auth"
	"docklift/internal/httpx"
	sessions "docklift/internal/terminal"
)

// Path is the only path that accepts a WebSocket upgrade.
const Path = "/ws/terminal"

// Attach installs the upgrade handler on the root router r. Like the Node
// 'upgrade' listener it sees every upgrade request, before the HTTP
// middleware stack (body parsing, rate limits, maintenance gate): an upgrade
// on any other path is dropped without a response; on /ws/terminal the
// Origin and then the `?token=` terminal purpose token are checked and a
// failure is answered with a bare 403 or 401 status line. Every session is
// closed by a shutdown hook. r must already carry the trusted-proxy
// middleware, which IsTrustedOrigin relies on.
func Attach(r chi.Router, d *app.Deps) {
	AttachWith(r, d, sessions.StartShell, sessions.DefaultLimits())
}

// AttachWith is Attach with an explicit shell starter and limits.
func AttachWith(r chi.Router, d *app.Deps, start sessions.StartFunc, limits sessions.Limits) *sessions.Manager {
	m := sessions.NewManager(d.Log, d.Store, start, limits)
	d.Lifecycle.OnShutdown("terminal sessions", func(ctx context.Context) {
		m.CloseAll(ctx)
		d.Log.Info("   Terminal sessions cleaned up")
	})
	u := &upgrader{d: d, m: m, ws: websocket.Upgrader{
		// The Origin was checked against the trusted origins before Upgrade.
		CheckOrigin: func(*http.Request) bool { return true },
	}}
	r.Use(u.middleware)
	return m
}

type upgrader struct {
	d  *app.Deps
	m  *sessions.Manager
	ws websocket.Upgrader
}

// isUpgrade is Node's upgrade detection: an Upgrade header together with a
// Connection header listing the "upgrade" token.
func isUpgrade(r *http.Request) bool {
	if len(r.Header.Values("Upgrade")) == 0 {
		return false
	}
	for _, v := range r.Header.Values("Connection") {
		for _, tok := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
				return true
			}
		}
	}
	return false
}

func (u *upgrader) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}
		u.serve(w, r)
	})
}

func (u *upgrader) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.EscapedPath() != Path {
		refuse(w, "")
		return
	}
	if !u.allowedOrigin(r) {
		u.d.Log.Warn("[TERMINAL] Rejected upgrade — Origin not allowed: " + r.Header.Get("Origin"))
		refuse(w, "HTTP/1.1 403 Forbidden\r\n\r\n")
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		refuse(w, "HTTP/1.1 401 Unauthorized\r\n\r\n")
		return
	}
	claims, ok := u.d.Auth.VerifyPurposeToken(r.Context(), token, auth.PurposeTerminal)
	if !ok {
		refuse(w, "HTTP/1.1 401 Unauthorized\r\n\r\n")
		return
	}
	conn, err := u.ws.Upgrade(w, r, nil)
	if err != nil {
		// The upgrader already answered 400 (bad handshake).
		return
	}
	u.m.Serve(conn, sessions.User{ID: claims.UserID, Email: claims.Email}, peerAddress(r))
}

// allowedOrigin is isAllowedOrigin: a request without Origin passes only
// with TERMINAL_ALLOW_NO_ORIGIN=true; otherwise the Origin must be the
// dashboard itself or exactly match CORS_ORIGIN / DOCKLIFT_FRONTEND_URL.
func (u *upgrader) allowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return u.d.Config.TerminalAllowNoOrigin
	}
	allow := append(append([]string(nil), u.d.Config.CORSOrigins...), u.d.Config.FrontendURL)
	return httpx.IsTrustedOrigin(origin, r, "http", allow)
}

// refuse writes a raw status line (or nothing) on the bare connection and
// destroys it, as the Node listener did with socket.write + socket.destroy.
func refuse(w http.ResponseWriter, statusLine string) {
	conn, buf, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	defer conn.Close()
	if statusLine != "" {
		_, _ = io.WriteString(buf, statusLine)
		_ = buf.Flush()
	}
	// Half-close and drain briefly so the client reads the status line
	// instead of a connection reset (an abrupt close with unread input
	// makes the kernel send RST).
	if hc, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = hc.CloseWrite()
		_ = conn.SetReadDeadline(time.Now().Add(refuseDrainTimeout))
		_, _ = io.Copy(io.Discard, io.LimitReader(conn, refuseDrainLimit))
	}
}

// Bounds of the drain after a refused upgrade.
const (
	refuseDrainTimeout = time.Second
	refuseDrainLimit   = 64 << 10
)

// peerAddress is the socket peer (request.socket.remoteAddress).
func peerAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
