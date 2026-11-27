package terminal

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/auth"
	"docklift/internal/db"
	"docklift/internal/store"
	sessions "docklift/internal/terminal"
)

type server struct {
	d       *app.Deps
	srv     *httptest.Server
	m       *sessions.Manager
	session string
	term    string
	sse     string
}

func newServer(t *testing.T, mutate func(*app.Deps)) *server {
	t.Helper()
	d := apptest.NewDeps(t)
	d.Config.FrontendURL = "http://frontend.test:3000"
	d.Config.CORSOrigins = []string{" http://cors.test:4000"}
	if mutate != nil {
		mutate(d)
	}
	u, session := apptest.CreateAdmin(t, d, "ws@example.com", "ws-password-1")
	sub := auth.TokenSubject{UserID: u.ID, Email: u.Email, Role: u.Role, PasswordVersion: u.PasswordVersion()}
	term, _ := d.Auth.IssuePurpose(sub, auth.PurposeTerminal)
	sse, _ := d.Auth.IssuePurpose(sub, auth.PurposeSSE)
	r := chi.NewRouter()
	m := AttachWith(r, d, func(int, int) (sessions.Shell, error) { return nil, sessions.ErrUnsupported }, sessions.DefaultLimits())
	r.Get("/*", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("spa")) })
	s := &server{d: d, srv: httptest.NewServer(r), m: m, session: session, term: term, sse: sse}
	t.Cleanup(s.srv.Close)
	return s
}

func (s *server) url(path, token string) string {
	u, _ := url.Parse(s.srv.URL)
	u.Scheme, u.Path = "ws", path
	if token != "" {
		u.RawQuery = "token=" + url.QueryEscape(token)
	}
	return u.String()
}

func (s *server) self() string { return s.srv.URL }

func dial(t *testing.T, target, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	conn, resp, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial(target, h)
	if conn != nil {
		t.Cleanup(func() { conn.Close() })
	}
	return conn, resp, err
}

// rawUpgrade sends a handshake by hand and returns the raw response bytes,
// to check the bare status lines.
func rawUpgrade(t *testing.T, s *server, path, origin string) string {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(s.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req := "GET " + path + " HTTP/1.1\r\nHost: " + strings.TrimPrefix(s.srv.URL, "http://") +
		"\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
	if origin != "" {
		req += "Origin: " + origin + "\r\n"
	}
	_, _ = c.Write([]byte(req + "\r\n"))
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	var sb strings.Builder
	buf := make([]byte, 512)
	for {
		n, err := c.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			if sb.Len() == 0 {
				t.Logf("read error with no data: %v", err)
			}
			return sb.String()
		}
	}
}

func TestUpgradeChecks(t *testing.T) {
	s := newServer(t, nil)
	cases := []struct {
		name, path, origin, want string
	}{
		{"other path dropped", "/ws/other?token=" + s.term, s.self(), ""},
		{"api path dropped", "/api/health", s.self(), ""},
		{"missing origin", "/ws/terminal?token=" + s.term, "", "HTTP/1.1 403 Forbidden\r\n\r\n"},
		{"foreign origin", "/ws/terminal?token=" + s.term, "http://evil.example", "HTTP/1.1 403 Forbidden\r\n\r\n"},
		{"origin before token", "/ws/terminal", "http://evil.example", "HTTP/1.1 403 Forbidden\r\n\r\n"},
		{"missing token", "/ws/terminal", s.self(), "HTTP/1.1 401 Unauthorized\r\n\r\n"},
		{"empty token", "/ws/terminal?token=", s.self(), "HTTP/1.1 401 Unauthorized\r\n\r\n"},
		{"garbage token", "/ws/terminal?token=x.y.z", s.self(), "HTTP/1.1 401 Unauthorized\r\n\r\n"},
		{"session token", "/ws/terminal?token=" + s.session, s.self(), "HTTP/1.1 401 Unauthorized\r\n\r\n"},
		{"sse token", "/ws/terminal?token=" + s.sse, s.self(), "HTTP/1.1 401 Unauthorized\r\n\r\n"},
		{"encoded path is another path", "/ws/%74erminal?token=" + s.term, s.self(), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rawUpgrade(t, s, c.path, c.origin); got != c.want {
				t.Fatalf("response %q, want %q", got, c.want)
			}
		})
	}
}

func TestPlainRequestsPassThrough(t *testing.T) {
	s := newServer(t, nil)
	resp, err := http.Get(s.srv.URL + "/ws/terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := bufio.NewReader(resp.Body).ReadString('\n')
	if resp.StatusCode != 200 || body != "spa" {
		t.Fatalf("non-upgrade request: %d %q", resp.StatusCode, body)
	}
}

func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var m map[string]any
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAcceptedOrigins(t *testing.T) {
	s := newServer(t, nil)
	for _, origin := range []string{s.self(), "http://frontend.test:3000", "http://cors.test:4000"} {
		conn, resp, err := dial(t, s.url(Path, s.term), origin)
		if err != nil {
			t.Fatalf("origin %s: %v (%v)", origin, err, resp)
		}
		if m := readJSON(t, conn); m["type"] != "auth_required" {
			t.Fatalf("origin %s: %v", origin, m)
		}
		conn.Close()
	}
}

func TestNoOriginEscapeHatch(t *testing.T) {
	s := newServer(t, func(d *app.Deps) { d.Config.TerminalAllowNoOrigin = true })
	conn, _, err := dial(t, s.url(Path, s.term), "")
	if err != nil {
		t.Fatal(err)
	}
	if m := readJSON(t, conn); m["type"] != "auth_required" {
		t.Fatalf("%v", m)
	}
	if got := rawUpgrade(t, s, Path, ""); got != "HTTP/1.1 401 Unauthorized\r\n\r\n" {
		t.Fatalf("no origin, no token: %q", got)
	}
}

func TestPasswordChangeInvalidatesTerminalToken(t *testing.T) {
	s := newServer(t, nil)
	u, _ := s.d.Store.GetUserByEmail(t.Context(), "ws@example.com")
	hash, _ := auth.HashPassword("new-password-2")
	changed := db.NewTime(time.Now().Add(time.Second))
	if _, err := s.d.Store.UpdateUser(t.Context(), u.ID, store.UserUpdate{Password: store.Some(hash), PasswordChangedAt: store.Some(&changed)}); err != nil {
		t.Fatal(err)
	}
	if got := rawUpgrade(t, s, Path+"?token="+s.term, s.self()); got != "HTTP/1.1 401 Unauthorized\r\n\r\n" {
		t.Fatalf("stale terminal token: %q", got)
	}
}

func TestShutdownHookClosesSessions(t *testing.T) {
	s := newServer(t, nil)
	conn, _, err := dial(t, s.url(Path, s.term), s.self())
	if err != nil {
		t.Fatal(err)
	}
	readJSON(t, conn)
	if s.m.Sessions() != 1 {
		t.Fatalf("sessions %d", s.m.Sessions())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.d.Lifecycle.Shutdown(ctx)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("session must be closed by the shutdown hook")
	}
	if s.m.Sessions() != 0 {
		t.Fatalf("sessions %d after shutdown", s.m.Sessions())
	}
}
