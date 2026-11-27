package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"docklift/internal/db"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Fixtures produced by the TypeScript backend's own libraries (jsonwebtoken
// 9 with Date.now() pinned to 1795000000000, bcrypt 6).
const (
	tsSecret       = "test-secret-0123456789"
	tsSessionToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOiJ1LTEiLCJlbWFpbCI6ImFkYUBleGFtcGxlLmNvbSIsInJvbGUiOiJhZG1pbiIsInB3ZHYiOjE3OTUwMDAwMDAxMjMsImlhdCI6MTc5NTAwMDAwMCwiZXhwIjoxNzk1NjA0ODAwfQ.sbK8BMFzHx1fCqL1hL-GMwARUNR3D-5M7P8tfqN6n00"
	tsSSEToken     = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOiJ1LTEiLCJlbWFpbCI6ImFkYUBleGFtcGxlLmNvbSIsInJvbGUiOiJhZG1pbiIsInB1cnBvc2UiOiJzc2UiLCJwd2R2IjowLCJpYXQiOjE3OTUwMDAwMDAsImV4cCI6MTc5NTAwMDMwMH0.5IflaxpUtK75-UI3_TuQNrL2AQ8Dnw6TdV22QAg_Jhw"
	tsHash         = "$2b$12$sZSa/UYWabCyqaBbObc2vu3Pgkb1JM6nA0pxemgBtoJaJK6ZfTfK." // "password123"
	tsLongHash     = "$2b$04$axh2IyVpsG8E/ahsDtasB.X5gWt7JifTVSVoVerRFwOAD2LnyoK.2" // "p" x 100
)

var tsIssued = time.Unix(1795000000, 0)

func TestSigningMatchesJsonwebtoken(t *testing.T) {
	sub := TokenSubject{UserID: "u-1", Email: "ada@example.com", Role: "admin", PasswordVersion: 1795000000123}
	got, err := sign(tsSecret, sub, "", SessionTTL, tsIssued)
	if err != nil || got != tsSessionToken {
		t.Errorf("session token differs from jsonwebtoken:\n got %s\nwant %s (%v)", got, tsSessionToken, err)
	}
	sub.PasswordVersion = 0
	got, err = sign(tsSecret, sub, PurposeSSE, PurposeTTL, tsIssued)
	if err != nil || got != tsSSEToken {
		t.Errorf("sse token differs from jsonwebtoken:\n got %s\nwant %s (%v)", got, tsSSEToken, err)
	}
}

func TestVerifyJsonwebtokenTokens(t *testing.T) {
	c, err := verify(tsSecret, tsSessionToken, tsIssued.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != "u-1" || c.Email != "ada@example.com" || c.Role != "admin" || c.Purpose != "" || !c.HasPwdv || c.Pwdv != 1795000000123 {
		t.Errorf("claims = %+v", c)
	}
	sse, err := verify(tsSecret, tsSSEToken, tsIssued)
	if err != nil || sse.Purpose != PurposeSSE {
		t.Errorf("sse claims = %+v, %v", sse, err)
	}
	if _, err := verify(tsSecret, tsSessionToken, tsIssued.Add(SessionTTL)); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("at exp: %v, want expired (jsonwebtoken expires when now >= exp)", err)
	}
	if _, err := verify(tsSecret, tsSessionToken, tsIssued.Add(SessionTTL-time.Millisecond)); err != nil {
		t.Errorf("just before exp: %v", err)
	}
	if _, err := verify("other", tsSessionToken, tsIssued); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("wrong secret: %v", err)
	}
}

func rawToken(t *testing.T, header, payload map[string]any, secret string) string {
	t.Helper()
	enc := func(v map[string]any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	unsigned := enc(header) + "." + enc(payload)
	if header["alg"] == "none" {
		return unsigned + "."
	}
	method := jwt.GetSigningMethod(header["alg"].(string))
	sig, err := method.Sign(unsigned, []byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerifyEdgeCases(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := func() map[string]any {
		return map[string]any{"userId": "u", "email": "e", "role": "admin", "pwdv": 0, "iat": now.Unix(), "exp": now.Unix() + 60}
	}
	with := func(k string, v any) map[string]any {
		p := base()
		if v == nil {
			delete(p, k)
		} else {
			p[k] = v
		}
		return p
	}
	hs := func(alg string) map[string]any { return map[string]any{"alg": alg, "typ": "JWT"} }
	cases := []struct {
		name  string
		token string
		want  error
	}{
		{"HS256 accepted", rawToken(t, hs("HS256"), base(), "s"), nil},
		{"HS384 rejected", rawToken(t, hs("HS384"), base(), "s"), ErrInvalidToken},
		{"HS512 rejected", rawToken(t, hs("HS512"), base(), "s"), ErrInvalidToken},
		{"alg none rejected", rawToken(t, hs("none"), base(), "s"), ErrInvalidToken},
		{"HS256 wrong secret", rawToken(t, hs("HS256"), base(), "other"), ErrInvalidToken},
		{"no exp is valid", rawToken(t, hs("HS256"), with("exp", nil), "s"), nil},
		{"exp string", rawToken(t, hs("HS256"), with("exp", "soon"), "s"), ErrInvalidToken},
		{"nbf future", rawToken(t, hs("HS256"), with("nbf", now.Unix()+10), "s"), ErrTokenNotActive},
		{"nbf string", rawToken(t, hs("HS256"), with("nbf", "x"), "s"), ErrInvalidToken},
		{"nbf checked before exp", rawToken(t, hs("HS256"), map[string]any{"nbf": now.Unix() + 10, "exp": now.Unix() - 10}, "s"), ErrTokenNotActive},
		{"expired", rawToken(t, hs("HS256"), with("exp", now.Unix()), "s"), ErrTokenExpired},
		{"malformed", "abc.def", ErrInvalidToken},
		{"garbage", "not.a.jwt", ErrInvalidToken},
	}
	for _, c := range cases {
		_, err := verify("s", c.token, now)
		if !errors.Is(err, c.want) && !(c.want == nil && err == nil) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	claims := []struct {
		payload map[string]any
		state   userIDState
	}{
		{with("userId", nil), userIDFalsy},
		{with("userId", ""), userIDFalsy},
		{with("userId", false), userIDFalsy},
		{with("userId", 0), userIDFalsy},
		{with("userId", 5), userIDInvalid},
		{with("userId", true), userIDInvalid},
		{with("userId", []any{"x"}), userIDInvalid},
		{with("userId", "abc"), userIDString},
	}
	for _, c := range claims {
		got, err := verify("s", rawToken(t, hs("HS256"), c.payload, "s"), now)
		if err != nil || got.userIDState != c.state {
			t.Errorf("userId %#v: state %v, %v; want %v", c.payload["userId"], got, err, c.state)
		}
	}
	if c, _ := verify("s", rawToken(t, hs("HS256"), with("pwdv", "1"), "s"), now); c.HasPwdv {
		t.Error("a string pwdv is not a number")
	}
}

func TestPasswords(t *testing.T) {
	if !ComparePassword(tsHash, "password123") || ComparePassword(tsHash, "password124") {
		t.Error("node bcrypt $2b$ hash must verify")
	}
	if !ComparePassword(tsLongHash, strings.Repeat("p", 72)) || !ComparePassword(tsLongHash, strings.Repeat("p", 200)) {
		t.Error("passwords over 72 bytes must be truncated like node bcrypt")
	}
	for _, bad := range []string{"", "not-a-hash", "$2b$12$short"} {
		if ComparePassword(bad, "x") {
			t.Errorf("malformed hash %q must not match", bad)
		}
	}
	hash, err := HashPassword(strings.Repeat("x", 100))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2b$12$") || !ComparePassword(hash, strings.Repeat("x", 72)) {
		t.Errorf("hash %q: want $2b$ cost 12 with 72-byte truncation", hash)
	}
	EqualizeLoginTiming("whatever")
}

func TestLoadOrCreateSecrets(t *testing.T) {
	dir := t.TempDir()
	env, err := LoadOrCreateSecrets(dir, "envj", "envi", quiet())
	if err != nil || env != (Secrets{"envj", "envi"}) {
		t.Fatalf("env priority: %+v %v", env, err)
	}
	if _, err := os.Stat(filepath.Join(dir, SecretsFileName)); !os.IsNotExist(err) {
		t.Error("env secrets must not be persisted")
	}

	gen, err := LoadOrCreateSecrets(dir, "", "", quiet())
	if err != nil || len(gen.JWTSecret) != 128 || len(gen.InternalAPISecret) != 128 {
		t.Fatalf("generated: %+v %v", gen, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, SecretsFileName))
	if want := "{\n  \"jwtSecret\": \"" + gen.JWTSecret + "\",\n  \"internalApiSecret\": \"" + gen.InternalAPISecret + "\"\n}"; string(raw) != want {
		t.Errorf("file format = %q", raw)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(filepath.Join(dir, SecretsFileName)); st.Mode().Perm() != 0o600 {
			t.Errorf("secrets mode %v", st.Mode().Perm())
		}
	}
	again, err := LoadOrCreateSecrets(dir, "partial", "", quiet())
	if err != nil || again != gen {
		t.Errorf("persisted file wins over a partial env: %+v %v", again, err)
	}

	if err := os.WriteFile(filepath.Join(dir, SecretsFileName), []byte(`{"jwtSecret":"only"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mixed, err := LoadOrCreateSecrets(dir, "fromenv", "", quiet())
	if err != nil || mixed.JWTSecret != "fromenv" || len(mixed.InternalAPISecret) != 128 {
		t.Errorf("incomplete file: env half kept, other generated: %+v %v", mixed, err)
	}
}

type fixture struct {
	svc   *Service
	store *store.Store
	user  *store.User
	dir   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	d, err := db.Open(ctx, filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(d)
	hash, _ := HashPassword("password123")
	changed := db.FromUnixMilli(1795000000123)
	u, err := st.CreateUser(ctx, store.UserCreate{Name: "Ada", Email: "ada@example.com", Password: hash, PasswordChangedAt: &changed})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: New(st, Secrets{JWTSecret: "jwt-secret", InternalAPISecret: "internal-secret"}, dir, quiet()), store: st, user: u, dir: dir}
}

type authResult struct {
	code int
	body string
	user User
}

func run(mw func(http.Handler) http.Handler, req *http.Request) authResult {
	var res authResult
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res.user, _ = UserFrom(r.Context())
		w.WriteHeader(299)
	})).ServeHTTP(rec, req)
	res.code, res.body = rec.Code, strings.TrimSpace(rec.Body.String())
	return res
}

func errJSON(msg string) string { return `{"error":"` + msg + `"}` }

func TestRequireSession(t *testing.T) {
	f := newFixture(t)
	session, _ := f.svc.IssueSession(f.user)
	sse, _ := f.svc.IssuePurpose(TokenSubject{UserID: f.user.ID, PasswordVersion: f.user.PasswordVersion()}, PurposeSSE)
	term, _ := f.svc.IssuePurpose(TokenSubject{UserID: f.user.ID, PasswordVersion: f.user.PasswordVersion()}, PurposeTerminal)
	stale, _ := sign("jwt-secret", TokenSubject{UserID: f.user.ID, PasswordVersion: 1}, "", time.Hour, time.Now())
	ghost, _ := sign("jwt-secret", TokenSubject{UserID: "ghost"}, "", time.Hour, time.Now())
	expired, _ := sign("jwt-secret", TokenSubject{UserID: f.user.ID}, "", time.Second, time.Now().Add(-time.Hour))

	cases := []struct {
		name    string
		headers map[string]string
		code    int
		body    string
	}{
		{"no header", nil, 401, errJSON("Authentication required")},
		{"wrong scheme", map[string]string{"Authorization": "Token " + session}, 401, errJSON("Authentication required")},
		{"empty bearer", map[string]string{"Authorization": "Bearer "}, 401, errJSON("Authentication required")},
		{"double space", map[string]string{"Authorization": "Bearer  " + session}, 401, errJSON("Authentication required")},
		{"garbage", map[string]string{"Authorization": "Bearer abc"}, 401, errJSON("Invalid token")},
		{"expired", map[string]string{"Authorization": "Bearer " + expired}, 401, errJSON("Token expired")},
		{"sse purpose", map[string]string{"Authorization": "Bearer " + sse}, 401, errJSON("Invalid token for this endpoint")},
		{"terminal purpose", map[string]string{"Authorization": "Bearer " + term}, 401, errJSON("Invalid token for this endpoint")},
		{"stale password version", map[string]string{"Authorization": "Bearer " + stale}, 401, errJSON("Session expired. Please log in again.")},
		{"deleted user", map[string]string{"Authorization": "Bearer " + ghost}, 401, errJSON("User not found")},
		{"valid", map[string]string{"Authorization": "Bearer " + session}, 299, ""},
		{"trailing segment ignored", map[string]string{"Authorization": "Bearer " + session + " extra"}, 299, ""},
		{"internal secret", map[string]string{"X-Internal-Secret": "internal-secret"}, 299, ""},
		{"wrong internal secret", map[string]string{"X-Internal-Secret": "internal-secreT"}, 401, errJSON("Authentication required")},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/projects?token="+session, nil)
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		res := run(f.svc.RequireSession, req)
		if res.code != c.code || (c.body != "" && res.body != c.body) {
			t.Errorf("%s: %d %s", c.name, res.code, res.body)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+session)
	if res := run(f.svc.RequireSession, req); res.user != (User{UserID: f.user.ID, Email: "ada@example.com", Role: "admin"}) {
		t.Errorf("attached user = %+v", res.user)
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Internal-Secret", "internal-secret")
	if res := run(f.svc.RequireSession, req); res.user != (User{UserID: InternalUserID, Email: "internal@docklift", Role: "admin"}) {
		t.Errorf("internal user = %+v", res.user)
	}
}

func TestRequireSSE(t *testing.T) {
	f := newFixture(t)
	session, _ := f.svc.IssueSession(f.user)
	sse, _ := f.svc.IssuePurpose(TokenSubject{UserID: f.user.ID, Email: f.user.Email, Role: "admin", PasswordVersion: f.user.PasswordVersion()}, PurposeSSE)
	cases := []struct {
		query string
		code  int
		body  string
	}{
		{"", 401, errJSON("SSE token required")},
		{"?token=", 401, errJSON("SSE token required")},
		{"?token=" + session, 401, errJSON("SSE token required")},
		{"?token=abc", 401, errJSON("Invalid token")},
		{"?token=" + sse + "&token=" + sse, 401, errJSON("Invalid token")},
		{"?token=" + sse, 299, ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/system/logs/x"+c.query, nil)
		req.Header.Set("Authorization", "Bearer "+session)
		res := run(f.svc.RequireSSE, req)
		if res.code != c.code || (c.body != "" && res.body != c.body) {
			t.Errorf("query %q: %d %s", c.query, res.code, res.body)
		}
		if res.code == 299 && res.user.Purpose != PurposeSSE {
			t.Errorf("sse user = %+v", res.user)
		}
	}
	if _, err := f.store.UpdateUser(context.Background(), f.user.ID, store.UserUpdate{PasswordChangedAt: store.Some(&db.Time{})}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?token="+sse, nil)
	if res := run(f.svc.RequireSSE, req); res.body != errJSON("Session expired. Please log in again.") {
		t.Errorf("sse token after password change: %d %s", res.code, res.body)
	}
	if _, ok := f.svc.VerifyPurposeToken(context.Background(), sse, PurposeSSE); ok {
		t.Error("revoked purpose token accepted")
	}
}

func TestVerifyPurposeToken(t *testing.T) {
	f := newFixture(t)
	sub := TokenSubject{UserID: f.user.ID, PasswordVersion: f.user.PasswordVersion()}
	term, _ := f.svc.IssuePurpose(sub, PurposeTerminal)
	sse, _ := f.svc.IssuePurpose(sub, PurposeSSE)
	if c, ok := f.svc.VerifyPurposeToken(context.Background(), term, PurposeTerminal); !ok || c.UserID != f.user.ID {
		t.Error("terminal token rejected")
	}
	if _, ok := f.svc.VerifyPurposeToken(context.Background(), sse, PurposeTerminal); ok {
		t.Error("sse token accepted for terminal")
	}
	internal, _ := f.svc.IssuePurpose(TokenSubject{UserID: InternalUserID}, PurposeTerminal)
	if _, ok := f.svc.VerifyPurposeToken(context.Background(), internal, PurposeTerminal); !ok {
		t.Error("internal purpose tokens skip the password check, as in TypeScript")
	}
}

func TestRequireStepUpPassword(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name string
		user *User
		body string
		ok   bool
		code int
		resp string
	}{
		{"no body password", &User{UserID: f.user.ID}, `{}`, false, 403, `{"error":"Password confirmation required for this operation","requirePassword":true}`},
		{"non-string password", &User{UserID: f.user.ID}, `{"password":123}`, false, 403, `{"error":"Password confirmation required for this operation","requirePassword":true}`},
		{"internal caller", &User{UserID: InternalUserID}, `{"password":"x"}`, false, 401, `{"error":"Unauthorized"}`},
		{"unknown user", &User{UserID: "ghost"}, `{"password":"x"}`, false, 403, `{"error":"Unable to verify password","requirePassword":true}`},
		{"wrong password", &User{UserID: f.user.ID}, `{"password":"nope"}`, false, 403, `{"error":"Invalid password","requirePassword":true}`},
		{"right password", &User{UserID: f.user.ID}, `{"password":"password123"}`, true, 200, ``},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		var ok bool
		var err error
		httpx.ParseBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c.user != nil {
				r = r.WithContext(WithUser(r.Context(), *c.user))
			}
			ok, err = f.svc.RequireStepUpPassword(w, r)
		})).ServeHTTP(rec, req)
		if err != nil || ok != c.ok || rec.Code != c.code || strings.TrimSpace(rec.Body.String()) != c.resp {
			t.Errorf("%s: ok=%v err=%v %d %s", c.name, ok, err, rec.Code, rec.Body.String())
		}
	}
}

// stepUp runs RequireStepUpPassword for f.user from remote with body.
func (f *fixture) stepUp(t *testing.T, remote, password string) (bool, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	var ok bool
	httpx.ParseBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		ok, err = f.svc.RequireStepUpPassword(w, r.WithContext(WithUser(r.Context(), User{UserID: f.user.ID})))
		if err != nil {
			t.Fatal(err)
		}
	})).ServeHTTP(rec, req)
	return ok, rec
}

func TestStepUpPasswordGuessesAreLimited(t *testing.T) {
	f := newFixture(t)
	now := time.Now()
	f.svc.now = func() time.Time { return now }
	const client, other = "198.51.100.7:4000", "198.51.100.8:4000"

	for i := 0; i < StepUpFailureLimit; i++ {
		if ok, rec := f.stepUp(t, client, "wrong"); ok || rec.Code != 403 {
			t.Fatalf("guess %d: %v %d %s", i+1, ok, rec.Code, rec.Body.String())
		}
	}
	// Further attempts are refused without comparing, even the right password.
	for _, pw := range []string{"wrong", "password123"} {
		ok, rec := f.stepUp(t, client, pw)
		if ok || rec.Code != 429 || rec.Header().Get("Retry-After") != "900" ||
			strings.TrimSpace(rec.Body.String()) != `{"error":"Too many failed password attempts. Try again later.","requirePassword":true}` {
			t.Fatalf("throttled %q: %v %d %v %s", pw, ok, rec.Code, rec.Header(), rec.Body.String())
		}
	}
	// Another client of the same user is counted separately.
	if ok, _ := f.stepUp(t, other, "password123"); !ok {
		t.Fatal("other client must not be throttled")
	}
	// The window ends 15 minutes after the first failure.
	now = now.Add(StepUpFailureWindow)
	if ok, _ := f.stepUp(t, client, "password123"); !ok {
		t.Fatal("window over: the right password must pass")
	}
	// A success resets the count.
	for i := 0; i < StepUpFailureLimit-1; i++ {
		f.stepUp(t, client, "wrong")
	}
	f.stepUp(t, client, "password123")
	for i := 0; i < StepUpFailureLimit-1; i++ {
		f.stepUp(t, client, "wrong")
	}
	if ok, _ := f.stepUp(t, client, "password123"); !ok {
		t.Fatal("success must reset the failure count")
	}
}

// Ports of backend/src/lib/setupRestoreAuth.test.ts.

func TestValidateSetupTokenDoesNotConsume(t *testing.T) {
	b := NewBootstrap(t.TempDir(), quiet())
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(b.SetupTokenPath(), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	if !b.ValidateSetupToken(token) {
		t.Error("valid token rejected")
	}
	if _, err := os.Stat(b.SetupTokenPath()); err != nil {
		t.Error("validation must not consume the token")
	}
	if b.ValidateSetupToken("wrong") || b.ValidateSetupToken("") {
		t.Error("wrong token accepted")
	}
	if _, err := os.Stat(b.SetupTokenPath()); err != nil {
		t.Error("failed validation must not consume the token")
	}
}

func TestConsumeSetupRestoreSecrets(t *testing.T) {
	b := NewBootstrap(t.TempDir(), quiet())
	if err := os.WriteFile(b.SetupTokenPath(), []byte(strings.Repeat("b", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureSecret(); err != nil {
		t.Fatal(err)
	}
	if b.ReadSecret() == "" {
		t.Fatal("bootstrap secret missing")
	}
	b.ConsumeSetupRestoreSecrets()
	if _, err := os.Stat(b.SetupTokenPath()); !os.IsNotExist(err) {
		t.Error("setup token not removed")
	}
	if b.ReadSecret() != "" {
		t.Error("bootstrap secret not removed")
	}
}

func TestBootstrapClaimLifecycle(t *testing.T) {
	dir := t.TempDir()
	b := NewBootstrap(dir, quiet())
	if b.TryClaim() != "" {
		t.Error("claim without a secret must fail")
	}
	secret, err := b.EnsureSecret()
	if err != nil || len(secret) != 64 {
		t.Fatalf("secret %q %v", secret, err)
	}
	if again, _ := b.EnsureSecret(); again != secret {
		t.Error("EnsureSecret must reuse the file")
	}
	// Verifying twice proves verification does not consume the secret.
	first, second := b.VerifySecret(secret), b.VerifySecret(secret)
	if !first || !second || b.VerifySecret(secret[:63]) || b.VerifySecret("") {
		t.Error("VerifySecret")
	}
	claim := b.TryClaim()
	if claim == "" || !strings.HasPrefix(filepath.Base(claim), ".claimed-") || b.ReadSecret() != "" {
		t.Fatalf("claim %q", claim)
	}
	if b.TryClaim() != "" {
		t.Error("second concurrent claim must lose")
	}
	if !VerifySecretAt(claim, secret) {
		t.Error("claimed secret must verify at the claim path")
	}
	b.AbortClaim(claim)
	if b.ReadSecret() != secret {
		t.Error("abort must restore the secret")
	}
	claim = b.TryClaim()
	b.FinalizeClaim(claim)
	if _, err := os.Stat(claim); !os.IsNotExist(err) || b.ReadSecret() != "" {
		t.Error("finalize must drop the claim")
	}
}

func TestAbortClaimWinsOverRecreatedSecret(t *testing.T) {
	b := NewBootstrap(t.TempDir(), quiet())
	secret, err := b.EnsureSecret()
	if err != nil {
		t.Fatal(err)
	}
	claim := b.TryClaim()
	// GET /status during the registration sees no user and re-creates a secret.
	if recreated, err := b.EnsureSecret(); err != nil || recreated == secret {
		t.Fatalf("recreated %q %v", recreated, err)
	}
	b.AbortClaim(claim)
	if b.ReadSecret() != secret {
		t.Error("the operator's secret must be restored after a failed registration")
	}
	if _, err := os.Stat(claim); !os.IsNotExist(err) {
		t.Error("the claim file must be gone")
	}
}

func TestEnsureTokenFileConcurrent(t *testing.T) {
	b := NewBootstrap(t.TempDir(), quiet())
	const n = 32
	tokens := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := b.EnsureSetupToken()
			if err != nil {
				t.Error(err)
			}
			tokens[i] = tok
		}()
	}
	wg.Wait()
	for i, tok := range tokens {
		if len(tok) != 64 || tok != tokens[0] {
			t.Fatalf("caller %d got %q, caller 0 got %q: concurrent callers must agree", i, tok, tokens[0])
		}
	}
	if !b.ValidateSetupToken(tokens[0]) {
		t.Error("the returned token must be the stored one")
	}
}

func TestEnsureTokenFileReplacesEmptyFile(t *testing.T) {
	b := NewBootstrap(t.TempDir(), quiet())
	if err := os.WriteFile(b.SecretPath(), []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := b.EnsureSecret()
	if err != nil || len(secret) != 64 || b.ReadSecret() != secret {
		t.Fatalf("empty secret file must be replaced: %q %v (stored %q)", secret, err, b.ReadSecret())
	}
	if b.VerifySecret("") || VerifySecretAt(filepath.Join(t.TempDir(), "missing"), "") {
		t.Error("empty values never verify")
	}
}

func TestRecoverStaleClaims(t *testing.T) {
	dir := t.TempDir()
	b := NewBootstrap(dir, quiet())
	old := filepath.Join(dir, ".claimed-old")
	newest := filepath.Join(dir, ".claimed-new")
	_ = os.WriteFile(old, []byte("old-secret"), 0o600)
	_ = os.WriteFile(newest, []byte("new-secret"), 0o600)
	past := time.Now().Add(-time.Hour)
	_ = os.Chtimes(old, past, past)
	b.RecoverStaleClaims()
	if b.ReadSecret() != "new-secret" {
		t.Errorf("recovered %q, want the newest claim", b.ReadSecret())
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("older claims must be removed")
	}
}

type counter struct {
	n   int
	err error
}

func (c counter) CountUsers(context.Context) (int, error) { return c.n, c.err }

func TestLogIfNeeded(t *testing.T) {
	dir := t.TempDir()
	b := NewBootstrap(dir, quiet())
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() { os.Stdout = stdout; devnull.Close() }()

	_ = os.WriteFile(filepath.Join(dir, ".claimed-x"), []byte("s"), 0o600)
	if err := b.LogIfNeeded(context.Background(), counter{n: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claimed-x")); !os.IsNotExist(err) || b.ReadSecret() != "" {
		t.Error("with an admin present, leftover claims are deleted and no secret is created")
	}
	if err := b.LogIfNeeded(context.Background(), counter{err: errors.New("no such table")}); err != nil {
		t.Fatal(err)
	}
	if b.ReadSecret() == "" {
		t.Error("unreadable database is treated as a fresh install")
	}
}
