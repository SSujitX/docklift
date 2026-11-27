// Package terminal runs the interactive shell sessions of the WebSocket
// terminal (the session logic of backend/src/services/terminal.ts): the
// password prompt with per-session and per-client failure limits, at most
// three concurrent sessions, the PTY shell, resize, the 30-minute idle
// timeout, and the shutdown of every session when the server stops. Not in
// TypeScript: a session must pass the password step within AuthTimeout, and
// it ends when its context ends (the user's password changed).
//
// The HTTP upgrade (path, Origin and token checks) lives in
// internal/api/terminal.
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	"docklift/internal/auth"
	"docklift/internal/store"
)

// Limits of a Manager. DefaultLimits are the values of terminal.ts plus the
// transport bounds the Node ws server did not have.
type Limits struct {
	MaxSessions     int
	MaxAuthAttempts int
	IdleTimeout     time.Duration
	// AuthTimeout closes a session that has not passed the password step in
	// time, so unauthenticated sockets cannot hold the session slots
	// (0 = no deadline).
	AuthTimeout time.Duration
	// GlobalAuthWindow / GlobalAuthMaxFails rate-limit failed passwords per
	// client address and user across reconnects.
	GlobalAuthWindow   time.Duration
	GlobalAuthMaxFails int
	// MaxMessageBytes caps one client frame.
	MaxMessageBytes int64
	// PingInterval / PongWait detect dead peers; WriteWait bounds one write;
	// CloseGrace is how long a closing peer gets to answer the close frame.
	PingInterval time.Duration
	PongWait     time.Duration
	WriteWait    time.Duration
	CloseGrace   time.Duration
}

// DefaultLimits returns the production limits.
func DefaultLimits() Limits {
	return Limits{
		MaxSessions:        3,
		MaxAuthAttempts:    5,
		IdleTimeout:        30 * time.Minute,
		AuthTimeout:        60 * time.Second,
		GlobalAuthWindow:   15 * time.Minute,
		GlobalAuthMaxFails: 20,
		MaxMessageBytes:    1 << 20,
		PingInterval:       30 * time.Second,
		PongWait:           75 * time.Second,
		WriteWait:          10 * time.Second,
		CloseGrace:         5 * time.Second,
	}
}

// Users looks up accounts; *store.Store implements it.
type Users interface {
	GetUser(ctx context.Context, id string) (*store.User, error)
}

// User is the account a terminal token was issued to.
type User struct {
	ID    string
	Email string
}

// Manager owns every terminal session of the process.
type Manager struct {
	limits Limits
	log    *slog.Logger
	users  Users
	start  StartFunc
	now    func() time.Time

	mu       sync.Mutex
	sessions map[*session]struct{}
	closed   bool
	wg       sync.WaitGroup

	failMu sync.Mutex
	fails  map[string]*failWindow
}

type failWindow struct {
	count int
	start time.Time
}

// NewManager returns a session manager that starts shells with start.
func NewManager(log *slog.Logger, users Users, start StartFunc, limits Limits) *Manager {
	return &Manager{limits: limits, log: log, users: users, start: start, now: time.Now,
		sessions: map[*session]struct{}{}, fails: map[string]*failWindow{}}
}

// Sessions is the number of open sessions (authenticated or not).
func (m *Manager) Sessions() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *Manager) rateLimited(key string) bool {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	e, ok := m.fails[key]
	if !ok {
		return false
	}
	if m.now().Sub(e.start) > m.limits.GlobalAuthWindow {
		delete(m.fails, key)
		return false
	}
	return e.count >= m.limits.GlobalAuthMaxFails
}

func (m *Manager) recordFail(key string) {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	now := m.now()
	for k, e := range m.fails {
		if now.Sub(e.start) > m.limits.GlobalAuthWindow {
			delete(m.fails, k)
		}
	}
	if e, ok := m.fails[key]; ok {
		e.count++
		return
	}
	m.fails[key] = &failWindow{count: 1, start: now}
}

// Serve runs one upgraded connection until it closes. clientAddr is the
// socket peer address (the rate-limit key is "<address>:<user id>"). When
// ctx ends (auth.BindSession cancels it after a password change) the shell
// is killed and the socket closed.
func (m *Manager) Serve(ctx context.Context, conn *websocket.Conn, user User, clientAddr string) {
	s := &session{m: m, ctx: ctx, conn: conn, user: user, rateKey: clientAddr + ":" + user.ID, done: make(chan struct{})}
	conn.SetReadLimit(m.limits.MaxMessageBytes)

	if !m.admit(s) {
		return
	}
	defer m.wg.Done()
	m.log.Info("[TERMINAL] WebSocket connected for user: " + user.Email)
	s.run()
	m.log.Info("[TERMINAL] WebSocket disconnected for user: " + user.Email)
}

// admit registers s as a session, or rejects the connection when the server
// is stopping, the session limit is reached or the client is rate-limited
// (checked in that order, like terminal.ts).
func (m *Manager) admit(s *session) bool {
	m.mu.Lock()
	closed, full := m.closed, len(m.sessions) >= m.limits.MaxSessions
	m.mu.Unlock()
	switch {
	case closed:
		_ = s.conn.Close()
		return false
	case full:
		m.log.Warn("[TERMINAL] Max concurrent sessions reached — rejecting user", "max", m.limits.MaxSessions, "user", s.user.Email)
		s.reject("Too many active terminal sessions. Close one first.")
		return false
	case m.rateLimited(s.rateKey):
		s.reject("Too many failed attempts. Try again later.")
		return false
	}
	m.mu.Lock()
	if m.closed || len(m.sessions) >= m.limits.MaxSessions {
		// Lost a race with another connection or with shutdown.
		m.mu.Unlock()
		s.reject("Too many active terminal sessions. Close one first.")
		return false
	}
	m.sessions[s] = struct{}{}
	m.wg.Add(1)
	m.mu.Unlock()
	return true
}

func (m *Manager) remove(s *session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, s)
}

// CloseAll is cleanupAllSessions: it kills every shell, closes every socket,
// refuses new sessions and waits for the sessions to finish. When ctx ends
// first, the remaining connections are dropped.
func (m *Manager) CloseAll(ctx context.Context) {
	m.mu.Lock()
	m.closed = true
	all := make([]*session, 0, len(m.sessions))
	for s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		s.killShell()
		s.closeSocket()
	}
	finished := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
		// A peer that did not answer the close frame in time: drop the
		// connections, which ends each read loop, then let the sessions
		// unregister so none is still counted when CloseAll returns.
		for _, s := range all {
			_ = s.conn.Close()
		}
		<-finished
	}
}

// Messages sent to the browser.
type authMessage struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

type outputMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type exitMessage struct {
	Type   string `json:"type"`
	Code   int    `json:"code"`
	Reason string `json:"reason,omitempty"`
}

type session struct {
	m       *Manager
	ctx     context.Context
	conn    *websocket.Conn
	user    User
	rateKey string
	done    chan struct{}

	writeMu sync.Mutex

	mu            sync.Mutex
	authenticated bool
	authAttempts  int
	shell         Shell
	closing       bool
	idle          *time.Timer
	authDeadline  *time.Timer
}

// send writes one JSON text frame unless the socket is closing.
func (s *session) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	closing := s.closing
	s.mu.Unlock()
	if closing {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(s.m.limits.WriteWait))
	if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		_ = s.conn.Close()
	}
}

// closeSocket is ws.close(): a close frame without status, then the peer
// has CloseGrace to answer before the read loop gives up.
func (s *session) closeSocket() {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.closing = true
	s.mu.Unlock()
	_ = s.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(s.m.limits.WriteWait))
	_ = s.conn.SetReadDeadline(time.Now().Add(s.m.limits.CloseGrace))
}

// reject sends an auth_error to a connection that never became a session,
// closes it and drains it until the peer answers the close frame.
func (s *session) reject(msg string) {
	s.send(authMessage{Type: "auth_error", Message: msg})
	s.closeSocket()
	for {
		if _, _, err := s.conn.NextReader(); err != nil {
			break
		}
	}
	_ = s.conn.Close()
}

func (s *session) killShell() {
	s.mu.Lock()
	sh := s.shell
	s.shell = nil
	s.mu.Unlock()
	if sh != nil {
		sh.Close()
	}
}

// run is the session: auth_required, then the message loop. It returns once
// the socket is closed and every helper goroutine has stopped.
func (s *session) run() {
	var helpers sync.WaitGroup
	defer func() {
		s.mu.Lock()
		if s.idle != nil {
			s.idle.Stop()
		}
		if s.authDeadline != nil {
			s.authDeadline.Stop()
		}
		s.mu.Unlock()
		s.killShell()
		_ = s.conn.Close()
		close(s.done)
		helpers.Wait()
		s.m.remove(s)
	}()

	_ = s.conn.SetReadDeadline(time.Now().Add(s.m.limits.PongWait))
	s.conn.SetPongHandler(func(string) error {
		s.mu.Lock()
		closing := s.closing
		s.mu.Unlock()
		if !closing {
			_ = s.conn.SetReadDeadline(time.Now().Add(s.m.limits.PongWait))
		}
		return nil
	})
	helpers.Add(2)
	go func() {
		defer helpers.Done()
		s.pinger()
	}()
	go func() {
		defer helpers.Done()
		select {
		case <-s.done:
		case <-s.ctx.Done():
			s.revoked()
		}
	}()

	s.send(authMessage{Type: "auth_required"})
	if s.m.limits.AuthTimeout > 0 {
		s.mu.Lock()
		s.authDeadline = time.AfterFunc(s.m.limits.AuthTimeout, s.authTimeout)
		s.mu.Unlock()
	}
	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		s.mu.Lock()
		closing := s.closing
		if !closing {
			_ = s.conn.SetReadDeadline(time.Now().Add(s.m.limits.PongWait))
		}
		s.mu.Unlock()
		if !closing {
			s.handle(data, &helpers)
		}
	}
}

func (s *session) pinger() {
	t := time.NewTicker(s.m.limits.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			if err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(s.m.limits.WriteWait)); err != nil {
				return
			}
		}
	}
}

// handle processes one client message. Malformed messages are ignored, as
// are input and resize before authentication.
func (s *session) handle(data []byte, helpers *sync.WaitGroup) {
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil || msg == nil {
		return
	}
	typ, _ := msg["type"].(string)
	if typ == "auth" {
		s.authenticate(msg, helpers)
		return
	}
	s.mu.Lock()
	sh := s.shell
	authed := s.authenticated
	s.mu.Unlock()
	if !authed || sh == nil {
		return
	}
	switch typ {
	case "input":
		if in, ok := msg["data"].(string); ok && in != "" {
			_, _ = sh.Write([]byte(in))
			s.resetIdle()
		}
	case "resize":
		if !truthy(msg["cols"]) || !truthy(msg["rows"]) {
			return
		}
		cols, rows := toNumber(msg["cols"]), toNumber(msg["rows"])
		if isInt(cols) && isInt(rows) && cols > 0 && cols < 500 && rows > 0 && rows < 500 {
			_ = sh.Resize(int(cols), int(rows))
		}
	}
}

func (s *session) authenticate(msg map[string]any, helpers *sync.WaitGroup) {
	s.mu.Lock()
	if s.authenticated {
		s.mu.Unlock()
		return
	}
	attempts := s.authAttempts
	s.mu.Unlock()

	if s.m.rateLimited(s.rateKey) || attempts >= s.m.limits.MaxAuthAttempts {
		s.m.log.Warn("[TERMINAL] Too many failed auth attempts for user: " + s.user.Email)
		s.send(authMessage{Type: "auth_error", Message: "Too many failed attempts. Reconnect later."})
		s.closeSocket()
		return
	}
	if !truthy(msg["password"]) {
		s.send(authMessage{Type: "auth_error", Message: "Password required"})
		return
	}
	// Verify the password of the token's user, not of an arbitrary account.
	ctx, cancel := context.WithTimeout(context.Background(), userLookupTimeout)
	defer cancel()
	u, err := s.m.users.GetUser(ctx, s.user.ID)
	if errors.Is(err, store.ErrNotFound) {
		s.send(authMessage{Type: "auth_error", Message: "No user account found"})
		return
	}
	if err != nil {
		// The TypeScript handler swallowed every failure without answering.
		s.m.log.Error("[TERMINAL] user lookup failed", "error", err)
		return
	}
	password, isString := msg["password"].(string)
	if !isString {
		// bcrypt.compare threw on a non-string, which was ignored silently.
		return
	}
	if !auth.ComparePassword(u.Password, password) {
		s.mu.Lock()
		s.authAttempts++
		attempts = s.authAttempts
		s.mu.Unlock()
		s.m.recordFail(s.rateKey)
		s.m.log.Warn("[TERMINAL] Failed auth attempt", "attempt", attempts, "max", s.m.limits.MaxAuthAttempts, "user", s.user.Email)
		s.send(authMessage{Type: "auth_error", Message: "Invalid password"})
		return
	}

	if s.ctx.Err() != nil {
		// Revoked while the password was being checked.
		return
	}
	s.mu.Lock()
	s.authenticated = true
	if s.authDeadline != nil {
		s.authDeadline.Stop()
	}
	s.mu.Unlock()
	s.m.log.Info("[TERMINAL] User authenticated for interactive terminal", "user", s.user.Email)

	cols, rows := initialSize(msg["cols"], 80), initialSize(msg["rows"], 24)
	sh, startErr := s.m.start(cols, rows)
	s.send(authMessage{Type: "auth_success"})
	if startErr != nil {
		// Like a failed spawn: the error event printed it into the terminal.
		s.m.log.Error("[TERMINAL] Shell error", "user", s.user.Email, "error", startErr)
		s.send(outputMessage{Type: "output", Data: "\r\nShell error: " + startErr.Error() + "\r\n"})
	} else {
		s.mu.Lock()
		s.shell = sh
		s.mu.Unlock()
		pumpDone := make(chan struct{})
		helpers.Add(2)
		go func() {
			defer helpers.Done()
			defer close(pumpDone)
			s.pump(sh)
		}()
		go func() {
			defer helpers.Done()
			s.waitShell(sh, pumpDone)
		}()
	}
	s.mu.Lock()
	s.idle = time.AfterFunc(s.m.limits.IdleTimeout, s.idleTimeout)
	s.mu.Unlock()
}

// authTimeout closes a session still waiting for its password.
func (s *session) authTimeout() {
	s.mu.Lock()
	authed := s.authenticated
	s.mu.Unlock()
	if authed {
		return
	}
	s.m.log.Warn("[TERMINAL] Authentication timeout for user: " + s.user.Email)
	s.send(exitMessage{Type: "exit", Code: -1, Reason: "auth_timeout"})
	s.closeSocket()
}

// revoked ends the session because its context ended (password changed).
func (s *session) revoked() {
	s.m.log.Info("[TERMINAL] Closing session after password change for user: " + s.user.Email)
	s.killShell()
	s.send(exitMessage{Type: "exit", Code: -1, Reason: "session_revoked"})
	s.closeSocket()
}

func (s *session) resetIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idle != nil {
		s.idle.Reset(s.m.limits.IdleTimeout)
	}
}

func (s *session) idleTimeout() {
	s.m.log.Info("[TERMINAL] Idle timeout for user: " + s.user.Email)
	s.killShell()
	s.send(exitMessage{Type: "exit", Code: -1, Reason: "idle_timeout"})
	s.closeSocket()
}

// pump forwards shell output as `output` messages, never splitting a UTF-8
// sequence across two messages.
func (s *session) pump(sh Shell) {
	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		n, err := sh.Read(buf)
		if n > 0 {
			chunk := append(pending, buf[:n]...)
			cut := completeUTF8(chunk)
			pending = append([]byte(nil), chunk[cut:]...)
			if cut > 0 {
				s.send(outputMessage{Type: "output", Data: string(chunk[:cut])})
			}
		}
		if err != nil {
			if len(pending) > 0 {
				s.send(outputMessage{Type: "output", Data: string(pending)})
			}
			return
		}
	}
}

// userLookupTimeout bounds the account lookup of a password attempt.
const userLookupTimeout = 10 * time.Second

// shellDrainGrace is how long output may still arrive after the shell exited
// (a background job can keep the terminal open).
const shellDrainGrace = 250 * time.Millisecond

// waitShell reports the shell's exit to the browser and closes the socket,
// unless the session ended it (close, idle timeout, shutdown).
func (s *session) waitShell(sh Shell, pumpDone <-chan struct{}) {
	code := sh.Wait()
	select {
	case <-pumpDone:
	case <-time.After(shellDrainGrace):
	}
	sh.Close()
	<-pumpDone
	s.mu.Lock()
	ours := s.shell == sh
	if ours {
		s.shell = nil
	}
	s.mu.Unlock()
	if !ours {
		return
	}
	s.m.log.Info("[TERMINAL] Shell exited", "code", code, "user", s.user.Email)
	s.send(exitMessage{Type: "exit", Code: code})
	s.closeSocket()
}

// completeUTF8 returns the length of the longest prefix of b that does not
// end inside a UTF-8 sequence (invalid bytes count as complete).
func completeUTF8(b []byte) int {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if !utf8.RuneStart(b[i]) {
			continue
		}
		if !utf8.FullRune(b[i:]) {
			return i
		}
		return len(b)
	}
	return len(b)
}

// truthy is JavaScript truthiness of a decoded JSON value.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && !math.IsNaN(x)
	}
	return true
}

// toNumber is Number(v) for JSON scalars; NaN for objects and arrays.
func toNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return x
	case string:
		t := trimSpace(x)
		if t == "" {
			return 0
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	}
	return math.NaN()
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func isInt(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) }

// initialSize is `msg.cols || 80` turned into a usable PTY dimension.
func initialSize(v any, def int) int {
	if !truthy(v) {
		return def
	}
	n := toNumber(v)
	if !isInt(n) || n <= 0 || n >= 500 {
		return def
	}
	return int(n)
}
