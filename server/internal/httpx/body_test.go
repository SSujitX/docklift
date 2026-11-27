package httpx

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		wantValue   string
	}{
		{"object", "application/json", `{"a":1,"b":"<x>"}`, 200, true, `{"a":1,"b":"<x>"}`},
		{"charset param", "application/json; charset=UTF-8", `{"a":true}`, 200, true, `{"a":true}`},
		{"array", "application/json", `[1,2]`, 200, true, `[1,2]`},
		{"empty json body is {}", "application/json", ``, 200, true, `{}`},
		{"leading whitespace", "application/json", " \n {\"a\":null}", 200, true, `{"a":null}`},
		{"strict rejects primitives", "application/json", `"str"`, 500, false, ``},
		{"strict rejects null", "application/json", `null`, 500, false, ``},
		{"malformed", "application/json", `{"a":`, 500, false, ``},
		{"trailing garbage", "application/json", `{} x`, 500, false, ``},
		{"latin1 charset", "application/json; charset=latin1", `{}`, 500, false, ``},
		{"urlencoded", "application/x-www-form-urlencoded", `email=a%40b.c&tag=1&tag=2`, 200, true, `{"email":"a@b.c","tag":["1","2"]}`},
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
		if c.wantStatus == 500 {
			if e.Error != InternalErrorMessage {
				t.Errorf("%s: error body %q", c.name, e.Error)
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
	// type-is counts as a body: express.json yields {}.
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
		if rec.Code != 500 {
			t.Errorf("oversize body (chunked=%v) status %d", chunked, rec.Code)
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
	if e := post(t, srv, "application/json", strings.NewReader(`{}`), func(r *http.Request) { r.Header.Set("Content-Encoding", "br") }); e.status != 500 {
		t.Errorf("unsupported encoding status %d", e.status)
	}
	many := strings.TrimSuffix(strings.Repeat("a=1&", 1001), "&")
	if e := post(t, srv, "application/x-www-form-urlencoded", strings.NewReader(many), nil); e.status != 500 {
		t.Errorf("parameter limit status %d", e.status)
	}
}

func valueOf(t *testing.T, raw string) Value {
	t.Helper()
	b, err := parseJSONBody([]byte(`{"v":` + raw + `}`))
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
	b, _ := parseJSONBody([]byte(`{"n":null}`))
	if !b.Get("n").IsNull() || !b.Get("n").Defined() || b.Get("missing").Defined() {
		t.Error("null vs undefined")
	}
	arr, _ := parseJSONBody([]byte(`[{"a":1}]`))
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
