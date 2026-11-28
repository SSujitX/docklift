package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"docklift/internal/app/apptest"
	"docklift/internal/store"
)

const password = "terminal-pass-123"

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeShell is a shell whose output the test writes and whose input it reads.
type fakeShell struct {
	outR    *io.PipeReader
	outW    *io.PipeWriter
	input   chan string
	mu      sync.Mutex
	sizes   [][2]int
	exited  chan int
	closed  chan struct{}
	closeMu sync.Once
}

func newFakeShell() *fakeShell {
	r, w := io.Pipe()
	return &fakeShell{outR: r, outW: w, input: make(chan string, 16), exited: make(chan int, 1), closed: make(chan struct{})}
}

func (f *fakeShell) Read(p []byte) (int, error) { return f.outR.Read(p) }
func (f *fakeShell) Write(p []byte) (int, error) {
	f.input <- string(p)
	return len(p), nil
}
func (f *fakeShell) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{cols, rows})
	return nil
}
func (f *fakeShell) resizes() [][2]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]int(nil), f.sizes...)
}
func (f *fakeShell) Wait() int {
	select {
	case code := <-f.exited:
		return code
	case <-f.closed:
		return 0
	}
}
func (f *fakeShell) Close() {
	f.closeMu.Do(func() {
		close(f.closed)
		_ = f.outW.Close()
	})
}

// exit simulates the shell exiting on its own with a code.
func (f *fakeShell) exit(code int) {
	f.exited <- code
	_ = f.outW.Close()
}

type harness struct {
	t      *testing.T
	m      *Manager
	srv    *httptest.Server
	user   User
	mu     sync.Mutex
	shells []*fakeShell
	sizes  [][2]int
	err    error
	// ctx is the session context of the next connection.
	ctx context.Context
}

func newHarness(t *testing.T, mutate func(*Limits)) *harness {
	t.Helper()
	d := apptest.NewDeps(t)
	u, _ := apptest.CreateAdmin(t, d, "term@example.com", password)
	limits := DefaultLimits()
	limits.CloseGrace = time.Second
	if mutate != nil {
		mutate(&limits)
	}
	h := &harness{t: t, user: User{ID: u.ID, Email: u.Email}, ctx: context.Background()}
	h.m = NewManager(quiet, d.Store, h.start, limits)
	up := websocket.Upgrader{}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.mu.Lock()
		ctx := h.ctx
		h.mu.Unlock()
		h.m.Serve(ctx, conn, h.user, "203.0.113.7")
	}))
	t.Cleanup(func() {
		h.m.CloseAll(context.Background())
		h.srv.Close()
	})
	return h
}

func (h *harness) start(cols, rows int) (Shell, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sizes = append(h.sizes, [2]int{cols, rows})
	if h.err != nil {
		return nil, h.err
	}
	s := newFakeShell()
	h.shells = append(h.shells, s)
	return s, nil
}

func (h *harness) shell(i int) *fakeShell {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.shells[i]
}

func (h *harness) dial() *websocket.Conn {
	h.t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.srv.URL, "http"), nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { conn.Close() })
	return conn
}

func read(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("message %q: %v", data, err)
	}
	return m
}

func expectMsg(t *testing.T, conn *websocket.Conn, want string) {
	t.Helper()
	got, _ := json.Marshal(read(t, conn))
	var w map[string]any
	_ = json.Unmarshal([]byte(want), &w)
	wb, _ := json.Marshal(w)
	if string(got) != string(wb) {
		t.Fatalf("got %s, want %s", got, wb)
	}
}

func expectClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("socket must be closed, got %s", data)
	}
	var ce *websocket.CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("want a close frame, got %v", err)
	}
}

func send(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSessionLifecycle(t *testing.T) {
	h := newHarness(t, nil)
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)

	send(t, conn, map[string]any{"type": "input", "data": "ignored\n"})
	_ = conn.WriteMessage(websocket.TextMessage, []byte("{not json"))
	_ = conn.WriteMessage(websocket.TextMessage, []byte("null"))
	send(t, conn, map[string]any{"type": "auth"})
	expectMsg(t, conn, `{"type":"auth_error","message":"Password required"}`)
	send(t, conn, map[string]any{"type": "auth", "password": 0})
	expectMsg(t, conn, `{"type":"auth_error","message":"Password required"}`)
	// A truthy non-string password made bcrypt throw, which was silent.
	send(t, conn, map[string]any{"type": "auth", "password": 12345})
	send(t, conn, map[string]any{"type": "auth", "password": "wrong"})
	expectMsg(t, conn, `{"type":"auth_error","message":"Invalid password"}`)
	send(t, conn, map[string]any{"type": "auth", "password": password, "cols": 132, "rows": "50"})
	expectMsg(t, conn, `{"type":"auth_success"}`)
	if h.sizes[0] != [2]int{132, 50} {
		t.Fatalf("initial size %v", h.sizes[0])
	}
	sh := h.shell(0)

	send(t, conn, map[string]any{"type": "auth", "password": password}) // ignored once authenticated
	send(t, conn, map[string]any{"type": "input", "data": 5})
	send(t, conn, map[string]any{"type": "input", "data": "ls -la\n"})
	if got := <-sh.input; got != "ls -la\n" {
		t.Fatalf("input %q", got)
	}
	for _, rs := range []map[string]any{
		{"type": "resize", "cols": 0, "rows": 10}, {"type": "resize", "cols": 500, "rows": 10}, {"type": "resize", "cols": 10.5, "rows": 10},
		{"type": "resize", "cols": "abc", "rows": 10}, {"type": "resize", "cols": 100}, {"type": "resize", "cols": "120", "rows": 40},
	} {
		send(t, conn, rs)
	}
	waitFor(t, "resize", func() bool { return len(sh.resizes()) == 1 })
	if sh.resizes()[0] != [2]int{120, 40} {
		t.Fatalf("resizes %v", sh.resizes())
	}

	// A multi-byte character split across reads arrives whole.
	go func() {
		_, _ = sh.outW.Write([]byte("h\xc3"))
		time.Sleep(20 * time.Millisecond)
		_, _ = sh.outW.Write([]byte("\xa9llo\r\n"))
	}()
	var out strings.Builder
	for out.Len() < len("héllo\r\n") {
		m := read(t, conn)
		if m["type"] != "output" {
			t.Fatalf("message %v", m)
		}
		out.WriteString(m["data"].(string))
	}
	if out.String() != "héllo\r\n" {
		t.Fatalf("output %q", out.String())
	}

	sh.exit(3)
	expectMsg(t, conn, `{"type":"exit","code":3}`)
	expectClosed(t, conn)
	waitFor(t, "session removal", func() bool { return h.m.Sessions() == 0 })
}

func TestLockoutAfterFiveFailures(t *testing.T) {
	h := newHarness(t, nil)
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	for i := 0; i < 5; i++ {
		send(t, conn, map[string]any{"type": "auth", "password": "nope"})
		expectMsg(t, conn, `{"type":"auth_error","message":"Invalid password"}`)
	}
	send(t, conn, map[string]any{"type": "auth", "password": password})
	expectMsg(t, conn, `{"type":"auth_error","message":"Too many failed attempts. Reconnect later."}`)
	expectClosed(t, conn)
}

func TestGlobalRateLimitSurvivesReconnect(t *testing.T) {
	h := newHarness(t, func(l *Limits) { l.GlobalAuthMaxFails = 2 })
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	for i := 0; i < 2; i++ {
		send(t, conn, map[string]any{"type": "auth", "password": "nope"})
		expectMsg(t, conn, `{"type":"auth_error","message":"Invalid password"}`)
	}
	send(t, conn, map[string]any{"type": "auth", "password": password})
	expectMsg(t, conn, `{"type":"auth_error","message":"Too many failed attempts. Reconnect later."}`)
	expectClosed(t, conn)

	again := h.dial()
	expectMsg(t, again, `{"type":"auth_error","message":"Too many failed attempts. Try again later."}`)
	expectClosed(t, again)

	h.m.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	later := h.dial()
	expectMsg(t, later, `{"type":"auth_required"}`)
}

func TestMaxSessions(t *testing.T) {
	h := newHarness(t, func(l *Limits) { l.MaxSessions = 2 })
	a, b := h.dial(), h.dial()
	expectMsg(t, a, `{"type":"auth_required"}`)
	expectMsg(t, b, `{"type":"auth_required"}`)
	c := h.dial()
	expectMsg(t, c, `{"type":"auth_error","message":"Too many active terminal sessions. Close one first."}`)
	expectClosed(t, c)
	a.Close()
	waitFor(t, "a slot", func() bool { return h.m.Sessions() == 1 })
	d := h.dial()
	expectMsg(t, d, `{"type":"auth_required"}`)
}

func authenticate(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	send(t, conn, map[string]any{"type": "auth", "password": password})
	expectMsg(t, conn, `{"type":"auth_success"}`)
}

func TestIdleTimeout(t *testing.T) {
	h := newHarness(t, func(l *Limits) { l.IdleTimeout = 300 * time.Millisecond })
	conn := h.dial()
	authenticate(t, conn)
	sh := h.shell(0)
	time.Sleep(150 * time.Millisecond)
	send(t, conn, map[string]any{"type": "input", "data": "x"}) // input resets the timer
	<-sh.input
	expectMsg(t, conn, `{"type":"exit","code":-1,"reason":"idle_timeout"}`)
	expectClosed(t, conn)
	select {
	case <-sh.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("idle shell was not killed")
	}
}

// A session that never passes the password step is closed after AuthTimeout.
func TestAuthTimeout(t *testing.T) {
	h := newHarness(t, func(l *Limits) { l.AuthTimeout = time.Second })
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	expectMsg(t, conn, `{"type":"exit","code":-1,"reason":"auth_timeout"}`)
	expectClosed(t, conn)
	waitFor(t, "session removed", func() bool { return h.m.Sessions() == 0 })

	// An authenticated session is not affected by the deadline.
	conn = h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	send(t, conn, map[string]any{"type": "auth", "password": password})
	expectMsg(t, conn, `{"type":"auth_success"}`)
	time.Sleep(1200 * time.Millisecond)
	send(t, conn, map[string]any{"type": "input", "data": "ls\n"})
	select {
	case in := <-h.shell(0).input:
		if in != "ls\n" {
			t.Fatalf("input = %q", in)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("authenticated session closed by the auth deadline")
	}
}

// Ending the session context (auth.BindSession after a password change)
// kills the shell and closes the socket.
func TestSessionContextRevokes(t *testing.T) {
	h := newHarness(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.ctx = ctx
	h.mu.Unlock()
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	send(t, conn, map[string]any{"type": "auth", "password": password})
	expectMsg(t, conn, `{"type":"auth_success"}`)
	sh := h.shell(0)
	cancel()
	expectMsg(t, conn, `{"type":"exit","code":-1,"reason":"session_revoked"}`)
	expectClosed(t, conn)
	select {
	case <-sh.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("shell of a revoked session still running")
	}
}

func TestClientDisconnectKillsShell(t *testing.T) {
	h := newHarness(t, nil)
	conn := h.dial()
	authenticate(t, conn)
	conn.Close()
	select {
	case <-h.shell(0).closed:
	case <-time.After(5 * time.Second):
		t.Fatal("shell survived the disconnect")
	}
	waitFor(t, "session removal", func() bool { return h.m.Sessions() == 0 })
}

func TestCloseAll(t *testing.T) {
	h := newHarness(t, nil)
	authed, waiting := h.dial(), h.dial()
	authenticate(t, authed)
	expectMsg(t, waiting, `{"type":"auth_required"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.m.CloseAll(ctx)
	if h.m.Sessions() != 0 {
		t.Fatalf("%d sessions after CloseAll", h.m.Sessions())
	}
	select {
	case <-h.shell(0).closed:
	default:
		t.Fatal("shell still running after CloseAll")
	}
	expectClosed(t, authed)
	expectClosed(t, waiting)
	late, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.srv.URL, "http"), nil)
	if err == nil {
		_ = late.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, _, err := late.ReadMessage(); err == nil {
			t.Fatal("new sessions must be refused after CloseAll")
		}
		late.Close()
	}
}

func TestShellStartFailure(t *testing.T) {
	h := newHarness(t, nil)
	h.err = ErrUnsupported
	conn := h.dial()
	authenticate(t, conn)
	expectMsg(t, conn, `{"type":"output","data":"\r\nShell error: interactive terminal is only supported on Linux\r\n"}`)
	send(t, conn, map[string]any{"type": "input", "data": "ls\n"}) // no shell: ignored
}

func TestUnknownUser(t *testing.T) {
	h := newHarness(t, nil)
	h.user = User{ID: "00000000-0000-4000-8000-000000000000", Email: "ghost@example.com"}
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	send(t, conn, map[string]any{"type": "auth", "password": password})
	expectMsg(t, conn, `{"type":"auth_error","message":"No user account found"}`)
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]int{"abc": 3, "h\xc3": 1, "h\xe2\x82": 1, "€": 3, "\xff": 1, "": 0, "a\xf0\x9f\x98": 1} {
		if got := completeUTF8([]byte(in)); got != want {
			t.Errorf("completeUTF8(%q) = %d, want %d", in, got, want)
		}
	}
	for _, c := range []struct {
		v    any
		want bool
	}{{nil, false}, {"", false}, {0.0, false}, {false, false}, {"0", true}, {1.0, true}, {map[string]any{}, true}, {[]any{}, true}} {
		if truthy(c.v) != c.want {
			t.Errorf("truthy(%#v)", c.v)
		}
	}
	if initialSize(nil, 80) != 80 || initialSize("100", 80) != 100 || initialSize(1000.0, 24) != 24 || initialSize(true, 24) != 1 {
		t.Error("initialSize")
	}
	if toNumber(" 42 ") != 42 || !isInt(toNumber(true)) || isInt(toNumber("4x")) || isInt(toNumber([]any{})) {
		t.Error("toNumber")
	}
	var _ Users = (*store.Store)(nil)
}

func TestOversizedMessageEndsSession(t *testing.T) {
	h := newHarness(t, func(l *Limits) { l.MaxMessageBytes = 64 })
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"`+strings.Repeat("x", 200)+`"}`))
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("an oversized frame must close the socket")
	}
	waitFor(t, "session removal", func() bool { return h.m.Sessions() == 0 })
}

func TestDeadPeerIsDropped(t *testing.T) {
	h := newHarness(t, func(l *Limits) {
		l.PingInterval = 20 * time.Millisecond
		l.PongWait = 150 * time.Millisecond
	})
	conn := h.dial()
	expectMsg(t, conn, `{"type":"auth_required"}`)
	// The client stops reading, so it never answers a ping.
	waitFor(t, "session removal", func() bool { return h.m.Sessions() == 0 })
	_ = conn.Close()
}
