package contract

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func wsURL(inst *Instance, path string, token string) string {
	u, _ := url.Parse(inst.BaseURL)
	u.Scheme = "ws"
	u.Path = path
	if token != "" {
		u.RawQuery = "token=" + url.QueryEscape(token)
	}
	return u.String()
}

func wsDial(t *testing.T, inst *Instance, path, token, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second, Proxy: nil}
	conn, resp, err := d.Dial(wsURL(inst, path, token), h)
	if conn != nil {
		t.Cleanup(func() { conn.Close() })
	}
	return conn, resp, err
}

// wsExpectStatus asserts the upgrade is refused with a bare HTTP status.
// The server writes the status line and destroys the socket immediately, so
// on some platforms the client occasionally sees a reset before it can read
// the response; such attempts are retried.
func wsExpectStatus(t *testing.T, inst *Instance, token, origin string, status int) {
	t.Helper()
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		conn, resp, err := wsDial(t, inst, "/ws/terminal", token, origin)
		if err == nil {
			conn.Close()
			t.Fatalf("upgrade with token=%v origin=%q: want HTTP %d, got a WebSocket", token != "", origin, status)
		}
		if resp == nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if resp.StatusCode != status {
			t.Fatalf("upgrade: want HTTP %d, got %d", status, resp.StatusCode)
		}
		return
	}
	t.Fatalf("upgrade: want HTTP %d, every attempt failed at transport level: %v", status, lastErr)
}

func wsRead(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	mt, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("ws message type %d, want text", mt)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("ws message %q: %v", data, err)
	}
	return m
}

func wsSend(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatalf("ws write: %v", err)
	}
}

func TestTerminalWebSocket(t *testing.T) {
	c := admin.Client()
	terminalToken := func(t *testing.T) string {
		return c.Post(t, "/api/auth/terminal-token", nil).Obj(t)["token"].(string)
	}
	self := mainInst.BaseURL

	t.Run("missing token", func(t *testing.T) { wsExpectStatus(t, mainInst, "", self, 401) })
	t.Run("garbage token", func(t *testing.T) { wsExpectStatus(t, mainInst, "x.y.z", self, 401) })
	t.Run("session token is not a terminal token", func(t *testing.T) { wsExpectStatus(t, mainInst, admin.Token, self, 401) })
	t.Run("sse token is not a terminal token", func(t *testing.T) {
		sse := c.Post(t, "/api/auth/sse-token", nil).Obj(t)["token"].(string)
		wsExpectStatus(t, mainInst, sse, self, 401)
	})
	t.Run("missing Origin", func(t *testing.T) { wsExpectStatus(t, mainInst, terminalToken(t), "", 403) })
	t.Run("foreign Origin", func(t *testing.T) { wsExpectStatus(t, mainInst, terminalToken(t), "http://evil.example", 403) })
	t.Run("app port Origin", func(t *testing.T) { wsExpectStatus(t, mainInst, terminalToken(t), "http://127.0.0.1:5500", 403) })
	t.Run("origin check runs before token check", func(t *testing.T) { wsExpectStatus(t, mainInst, "", "http://evil.example", 403) })
	t.Run("other upgrade paths are dropped", func(t *testing.T) {
		conn, resp, err := wsDial(t, mainInst, "/ws/other", terminalToken(t), self)
		if err == nil {
			conn.Close()
			t.Fatal("upgrade on /ws/other must fail")
		}
		if resp != nil {
			t.Fatalf("upgrade on /ws/other: want the socket closed without a response, got HTTP %d", resp.StatusCode)
		}
	})
	t.Run("token flow up to the password prompt", func(t *testing.T) {
		conn, resp, err := wsDial(t, mainInst, "/ws/terminal", terminalToken(t), self)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("dial: %v (HTTP %d)", err, status)
		}
		match(t, "first message", Obj{"type": "auth_required"}, wsRead(t, conn))
		wsSend(t, conn, map[string]any{"type": "auth"})
		match(t, "auth without password", Obj{"type": "auth_error", "message": "Password required"}, wsRead(t, conn))
		wsSend(t, conn, map[string]any{"type": "auth", "password": "definitely-wrong"})
		match(t, "wrong password", Obj{"type": "auth_error", "message": "Invalid password"}, wsRead(t, conn))
		// Input before authentication and malformed frames are ignored silently.
		wsSend(t, conn, map[string]any{"type": "input", "data": "id\n"})
		conn.WriteMessage(websocket.TextMessage, []byte("{not json"))
		wsSend(t, conn, map[string]any{"type": "auth", "password": ""})
		match(t, "empty password", Obj{"type": "auth_error", "message": "Password required"}, wsRead(t, conn))
	})
	t.Run("five failed passwords close the session", func(t *testing.T) {
		conn, _, err := wsDial(t, mainInst, "/ws/terminal", terminalToken(t), self)
		if err != nil {
			t.Fatal(err)
		}
		wsRead(t, conn)
		for i := 0; i < 5; i++ {
			wsSend(t, conn, map[string]any{"type": "auth", "password": "nope"})
			match(t, "wrong password", Obj{"type": "auth_error", "message": "Invalid password"}, wsRead(t, conn))
		}
		wsSend(t, conn, map[string]any{"type": "auth", "password": "nope"})
		match(t, "lockout", Obj{"type": "auth_error", "message": "Too many failed attempts. Reconnect later."}, wsRead(t, conn))
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		if _, _, err := conn.ReadMessage(); err == nil {
			t.Fatal("socket must be closed after lockout")
		}
	})
	t.Run("allowlisted origins", func(t *testing.T) {
		requireBoot(t)
		for _, origin := range []string{frontendURL, corsAllowedOrigin} {
			conn, resp, err := wsDial(t, mainInst, "/ws/terminal", terminalToken(t), origin)
			if err != nil {
				status := 0
				if resp != nil {
					status = resp.StatusCode
				}
				t.Fatalf("origin %s: %v (HTTP %d)", origin, err, status)
			}
			match(t, "first message", Obj{"type": "auth_required"}, wsRead(t, conn))
			conn.Close()
		}
	})
	t.Run("max three concurrent sessions", func(t *testing.T) {
		time.Sleep(300 * time.Millisecond) // let earlier sockets finish closing server-side
		var conns []*websocket.Conn
		for i := 0; i < 3; i++ {
			conn, _, err := wsDial(t, mainInst, "/ws/terminal", terminalToken(t), self)
			if err != nil {
				t.Fatal(err)
			}
			match(t, "session", Obj{"type": "auth_required"}, wsRead(t, conn))
			conns = append(conns, conn)
		}
		conn, _, err := wsDial(t, mainInst, "/ws/terminal", terminalToken(t), self)
		if err != nil {
			t.Fatal(err)
		}
		match(t, "fourth session", Obj{"type": "auth_error", "message": "Too many active terminal sessions. Close one first."}, wsRead(t, conn))
		for _, c := range conns {
			c.Close()
		}
	})
	t.Run("terminal token reuse", func(t *testing.T) {
		time.Sleep(300 * time.Millisecond) // let the sessions above finish closing server-side
		tok := terminalToken(t)
		conn, _, err := wsDial(t, mainInst, "/ws/terminal", tok, self)
		if err != nil {
			t.Fatal(err)
		}
		match(t, "first use", Obj{"type": "auth_required"}, wsRead(t, conn))
		conn.Close()
		if targetIsGo() {
			// Deliberate fix: terminal tokens are single-use on Go.
			wsExpectStatus(t, mainInst, tok, self, 401)
			return
		}
		time.Sleep(300 * time.Millisecond)
		again, _, err := wsDial(t, mainInst, "/ws/terminal", tok, self)
		if err != nil {
			t.Fatal(err)
		}
		match(t, "second use", Obj{"type": "auth_required"}, wsRead(t, again))
		again.Close()
	})
	t.Run("expired terminal token", func(t *testing.T) {
		secret, _ := readSecrets(t, mainInst)
		claims := jwtClaims(t, terminalToken(t))
		claims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
		wsExpectStatus(t, mainInst, signJWT(secret, claims), self, 401)
	})
}
