package httpx

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// bodyServer echoes what ParseBody produced, through a real HTTP server so
// header handling (Content-Length, Transfer-Encoding) is the production one.
func bodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(ParseBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := BodyFrom(r)
		rest, _ := io.ReadAll(r.Body)
		JSON(w, http.StatusOK, map[string]any{"defined": b.Defined, "value": b.Value().Raw(), "raw": string(b.Raw), "rest": string(rest)})
	})))
	t.Cleanup(srv.Close)
	return srv
}

type echo struct {
	status  int
	Defined bool   `json:"defined"`
	Value   any    `json:"value"`
	Raw     string `json:"raw"`
	Rest    string `json:"rest"`
	Error   string `json:"error"`
}

func post(t *testing.T, srv *httptest.Server, contentType string, body io.Reader, mutate func(*http.Request)) echo {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if mutate != nil {
		mutate(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var e echo
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		t.Fatal(err)
	}
	e.status = resp.StatusCode
	return e
}

func TestParseBody(t *testing.T) {
	srv := bodyServer(t)
	cases := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantDefined bool
		wantValue   string // the parsed value, or the error message for 4xx
	}{
		{"object", "application/json", `{"a":1,"b":"<x>"}`, 200, true, `{"a":1,"b":"<x>"}`},
		{"charset param", "application/json; charset=UTF-8", `{"a":true}`, 200, true, `{"a":true}`},
		{"array", "application/json", `[1,2]`, 200, true, `[1,2]`},
		{"empty json body is {}", "application/json", ``, 200, true, `{}`},
		{"leading whitespace", "application/json", " \n {\"a\":null}", 200, true, `{"a":null}`},
		{"strict rejects primitives", "application/json", `"str"`, 400, false, MsgInvalidJSON},
		{"strict rejects null", "application/json", `null`, 400, false, MsgInvalidJSON},
		{"whitespace only", "application/json", "  \n", 400, false, MsgInvalidJSON},
		{"malformed", "application/json", `{"a":`, 400, false, MsgInvalidJSON},
		{"trailing garbage", "application/json", `{} x`, 400, false, MsgInvalidJSON},
		{"two documents", "application/json", `{}{}`, 400, false, MsgInvalidJSON},
		{"latin1 charset", "application/json; charset=latin1", `{}`, 415, false, MsgUnsupportedCharset},
		{"urlencoded", "application/x-www-form-urlencoded", `email=a%40b.c&tag=1&tag=2`, 200, true, `{"email":"a@b.c","tag":["1","2"]}`},
		{"urlencoded plus and three repeats", "application/x-www-form-urlencoded", `n=a+b&t=1&t=2&t=3`, 200, true, `{"n":"a b","t":["1","2","3"]}`},
		{"urlencoded bad escapes kept like qs", "application/x-www-form-urlencoded", `a=%zz&b=100%&c=%E2%82%AC&d=%ff`, 200, true, `{"a":"%zz","b":"100%","c":"€","d":"%ff"}`},
		{"urlencoded empty parts and bare keys", "application/x-www-form-urlencoded", `&&flag&x=`, 200, true, `{"flag":"","x":""}`},
		{"urlencoded semicolons are not separators", "application/x-www-form-urlencoded", `a=1;b=2`, 200, true, `{"a":"1;b=2"}`},
		{"text/plain untouched", "text/plain", `{"a":1}`, 200, false, `null`},
		{"no content type", "", `{"a":1}`, 200, false, `null`},
		{"vendor json untouched", "application/vnd.api+json", `{"a":1}`, 200, false, `null`},
	}
	for _, c := range cases {
		e := post(t, srv, c.contentType, strings.NewReader(c.body), nil)
		if e.status != c.wantStatus || e.Defined != c.wantDefined {
			t.Errorf("%s: status=%d defined=%v error=%q", c.name, e.status, e.Defined, e.Error)
			continue
		}
		if c.wantStatus != 200 {
			if e.Error != c.wantValue {
				t.Errorf("%s: error body %q, want %q", c.name, e.Error, c.wantValue)
			}
			continue
		}
		got, _ := json.Marshal(e.Value)
		var want any
		_ = json.Unmarshal([]byte(c.wantValue), &want)
		wantJSON, _ := json.Marshal(want)
		if string(got) != string(wantJSON) {
			t.Errorf("%s: value %s, want %s", c.name, got, wantJSON)
		}
	}
}

func TestParseBodyKeepsRawAndRereadableBody(t *testing.T) {
	srv := bodyServer(t)
	raw := `{"b":2,  "a":1}`
	e := post(t, srv, "application/json", strings.NewReader(raw), nil)
	if e.Raw != raw || e.Rest != raw {
		t.Errorf("raw=%q rest=%q, want exact bytes for webhook HMAC", e.Raw, e.Rest)
	}
	multipart := post(t, srv, "multipart/form-data; boundary=x", strings.NewReader("--x--"), nil)
	if multipart.Defined || multipart.Rest != "--x--" {
		t.Errorf("multipart must be left for the route: %+v", multipart)
	}
}

func TestParseBodyNoBody(t *testing.T) {
	srv := bodyServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var e echo
	_ = json.NewDecoder(resp.Body).Decode(&e)
	// net/http sends Content-Length: 0 for a POST without body, which
	// type-is counts as a body: the JSON parser yields {}.
	if !e.Defined {
		t.Errorf("POST with Content-Length: 0 must parse as {}: %+v", e)
	}

	get, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var g echo
	_ = json.NewDecoder(get.Body).Decode(&g)
	if g.Defined {
		t.Errorf("GET without body must leave req.body undefined: %+v", g)
	}
}

func TestParseBodyLimitsAndEncodings(t *testing.T) {
	srv := bodyServer(t)
	big := `{"a":"` + strings.Repeat("x", MaxBodyBytes) + `"}`
	for _, chunked := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
		req.Header.Set("Content-Type", "application/json")
		if chunked {
			req.ContentLength = -1
			req.TransferEncoding = []string{"chunked"}
		}
		rec := httptest.NewRecorder()
		ParseBody(ok).ServeHTTP(rec, req)
		if rec.Code != 413 || rec.Body.String() != `{"error":"Request body too large"}` {
			t.Errorf("oversize body (chunked=%v): %d %s", chunked, rec.Code, rec.Body)
		}
	}
	// Over a real connection the client gets the 413 rather than a reset,
	// even while it is still sending (the rest of the body is drained).
	for _, ct := range []string{"application/json", "application/x-www-form-urlencoded"} {
		over := strings.Repeat("a", MaxBodyBytes+1024*1024)
		if e := post(t, srv, ct, strings.NewReader(over), nil); e.status != 413 || e.Error != MsgBodyTooLarge {
			t.Errorf("%s over the limit: %d %q", ct, e.status, e.Error)
		}
		if e := post(t, srv, ct, io.MultiReader(strings.NewReader(over)), func(r *http.Request) { r.ContentLength = -1 }); e.status != 413 {
			t.Errorf("%s chunked over the limit: %d %q", ct, e.status, e.Error)
		}
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte(`{"zipped":true}`))
	_ = zw.Close()
	e := post(t, srv, "application/json", &gz, func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") })
	if e.status != 200 || !e.Defined {
		t.Errorf("gzip body: %+v", e)
	}
	if e := post(t, srv, "application/json", strings.NewReader(`{}`), func(r *http.Request) { r.Header.Set("Content-Encoding", "br") }); e.status != 415 || e.Error != MsgUnsupportedEncoding {
		t.Errorf("unsupported encoding: %d %q", e.status, e.Error)
	}
	if e := post(t, srv, "application/json", strings.NewReader(`not gzip`), func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }); e.status != 400 || e.Error != MsgInvalidBody {
		t.Errorf("corrupt gzip: %d %q", e.status, e.Error)
	}
	// A compression bomb is cut off at the limit after decompression.
	var bomb bytes.Buffer
	bw := gzip.NewWriter(&bomb)
	_, _ = bw.Write([]byte(`{"a":"`))
	_, _ = bw.Write(bytes.Repeat([]byte("x"), MaxBodyBytes+1))
	_, _ = bw.Write([]byte(`"}`))
	_ = bw.Close()
	if e := post(t, srv, "application/json", &bomb, func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }); e.status != 413 || e.Error != MsgBodyTooLarge {
		t.Errorf("gzip bomb: %d %q", e.status, e.Error)
	}
	many := strings.TrimSuffix(strings.Repeat("a=1&", 1001), "&")
	if e := post(t, srv, "application/x-www-form-urlencoded", strings.NewReader(many), nil); e.status != 413 || e.Error != MsgTooManyParameters {
		t.Errorf("parameter limit: %d %q", e.status, e.Error)
	}
	limit := strings.TrimSuffix(strings.Repeat("a=1&", 1000), "&")
	if e := post(t, srv, "application/x-www-form-urlencoded", strings.NewReader(limit), nil); e.status != 200 {
		t.Errorf("1000 parameters are allowed: %d %q", e.status, e.Error)
	}
}

func valueOf(t *testing.T, raw string) Value {
	t.Helper()
	b, err := parseJSONBody([]byte(`{"v":`+raw+`}`), true)
	if err != nil {
		t.Fatal(err)
	}
	return b.Get("v")
}

func TestValueSemantics(t *testing.T) {
	truthy := map[string]bool{
		`""`: false, `"a"`: true, `0`: false, `0.0`: false, `-0`: false, `1`: true, `-1.5`: true,
		`true`: true, `false`: false, `null`: false, `[]`: true, `{}`: true,
	}
	for raw, want := range truthy {
		if got := valueOf(t, raw).Truthy(); got != want {
			t.Errorf("Truthy(%s) = %v", raw, got)
		}
	}
	if (Body{}).Get("v").Defined() || (Body{}).Get("v").Truthy() {
		t.Error("undefined body fields are undefined")
	}
	b, _ := parseJSONBody([]byte(`{"n":null}`), true)
	if !b.Get("n").IsNull() || !b.Get("n").Defined() || b.Get("missing").Defined() {
		t.Error("null vs undefined")
	}
	arr, _ := parseJSONBody([]byte(`[{"a":1}]`), true)
	if arr.Get("a").Defined() {
		t.Error("property access on an array body is undefined")
	}
	lengths := map[string]float64{`"abc"`: 3, `"😀"`: 2, `"é"`: 1, `[1,2]`: 2, `{"length":7.5}`: 7.5}
	for raw, want := range lengths {
		if n, ok := valueOf(t, raw).Length(); !ok || n != want {
			t.Errorf("Length(%s) = %v, %v; want %v", raw, n, ok, want)
		}
	}
	for _, raw := range []string{`12345678`, `true`, `{}`, `null`} {
		if _, ok := valueOf(t, raw).Length(); ok {
			t.Errorf("Length(%s) must be undefined", raw)
		}
	}
	if s, ok := valueOf(t, `"x"`).String(); !ok || s != "x" {
		t.Error("String()")
	}
	if _, ok := valueOf(t, `5`).String(); ok {
		t.Error("numbers are not strings")
	}
	if v, ok := valueOf(t, `true`).Bool(); !ok || !v {
		t.Error("Bool()")
	}
}

func TestJSONEncoding(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusCreated, map[string]string{"html": "<a&b>"})
	if rec.Code != 201 || rec.Body.String() != `{"html":"<a&b>"}` {
		t.Errorf("JSON = %d %q (must match JSON.stringify)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content type %q", ct)
	}
	rec = httptest.NewRecorder()
	JSON(rec, http.StatusOK, func() {})
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), InternalErrorMessage) {
		t.Errorf("unencodable value: %d %s", rec.Code, rec.Body.String())
	}
}

func TestParseBodyReadDeadline(t *testing.T) {
	old := bodyReadTimeout
	bodyReadTimeout = 200 * time.Millisecond
	t.Cleanup(func() { bodyReadTimeout = old })
	srv := bodyServer(t)
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Announce a body and then stall: the server must give up on its own.
	if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{\"a\":"); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("no response to a stalled body: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("stalled body: status %d", resp.StatusCode)
	}
}

func TestNewBody(t *testing.T) {
	b := NewBody(map[string]any{"password": "pw", "tags": []any{"a", "b"}})
	if !b.Defined || len(b.Raw) != 0 {
		t.Fatalf("NewBody = %+v", b)
	}
	if s, ok := b.Get("password").String(); !ok || s != "pw" {
		t.Errorf("password = %q, %v", s, ok)
	}
	if b.Get("missing").Defined() || !b.Get("tags").Truthy() {
		t.Error("property access")
	}
	ctx := WithBody(t.Context(), b)
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
	if got, _ := BodyFrom(req).Get("password").String(); got != "pw" {
		t.Errorf("BodyFrom = %q", got)
	}
}
