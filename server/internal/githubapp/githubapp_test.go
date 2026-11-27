package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// memSettings is an in-memory settings table.
type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemSettings() *memSettings { return &memSettings{m: map[string]string{}} }

func (s *memSettings) GetSetting(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok, nil
}

func (s *memSettings) SetSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

func (s *memSettings) DeleteSetting(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

func testKeyPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestStateFilePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "github-setup")
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	state := hex.EncodeToString(buf)
	if got, want := StateFilePath(dir, state), filepath.Join(dir, state+".json"); got != want {
		t.Fatalf("StateFilePath = %q, want %q", got, want)
	}
	for _, bad := range []string{
		"../../package", `..\..\package`, "..", ".", "a/b", `a\b`, "/etc/passwd", `C:\x`, "state.json",
		"a b", "a\x00b", "%2e%2e%2fpackage", "", strings.Repeat("a", 129),
	} {
		if got := StateFilePath(dir, bad); got != "" {
			t.Errorf("StateFilePath(%q) = %q, want rejected", bad, got)
		}
	}
	if !IsSafeStateToken(strings.Repeat("a", 128)) || !IsSafeStateToken("A-b_9") {
		t.Fatal("plain tokens must be accepted")
	}
}

func TestSetupStates(t *testing.T) {
	ctx := context.Background()
	settings := newMemSettings()
	now := time.UnixMilli(1_700_000_000_000)
	a := New(Config{DataPath: t.TempDir()}, settings, NewClient(""), discard())
	a.SetClock(func() time.Time { return now })
	st := a.States()

	s1, err := st.Create("http://localhost:3000/settings")
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) != 48 || !IsSafeStateToken(s1) {
		t.Fatalf("state %q", s1)
	}
	fi, err := os.Stat(filepath.Join(st.Dir(), s1+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := fi.Mode().Perm(); mode&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("state file mode %v", mode)
	}
	raw, _ := os.ReadFile(filepath.Join(st.Dir(), s1+".json"))
	if string(raw) != `{"createdAt":1700000000000,"returnUrl":"http://localhost:3000/settings"}` {
		t.Fatalf("state file = %s", raw)
	}
	s2, err := st.Create("")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(st.Dir(), s2+".json"))
	if string(raw) != `{"createdAt":1700000000000}` {
		t.Fatalf("state file without return URL = %s", raw)
	}

	for _, tc := range []struct {
		state string
		want  bool
	}{{s1, true}, {s2, true}, {"", false}, {strings.Repeat("0", 48), false}, {"../" + s1, false}} {
		ok, err := st.Verify(ctx, tc.state)
		if err != nil || ok != tc.want {
			t.Errorf("Verify(%q) = %v, %v; want %v", tc.state, ok, err, tc.want)
		}
	}
	// Verification does not consume the state.
	if ok, _ := st.Verify(ctx, s1); !ok {
		t.Fatal("state consumed by Verify")
	}
	if got := st.ReturnURL(s1); got != "http://localhost:3000/settings" {
		t.Fatalf("ReturnURL = %q", got)
	}
	if got := st.ReturnURL("../x"); got != "" {
		t.Fatalf("ReturnURL of unsafe state = %q", got)
	}

	// Junk files are pruned; non-JSON files are left alone.
	_ = os.WriteFile(filepath.Join(st.Dir(), "junk.json"), []byte("not json"), 0o600)
	_ = os.WriteFile(filepath.Join(st.Dir(), "keep.txt"), []byte("x"), 0o600)
	if err := st.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "junk.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("junk state not pruned")
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "keep.txt")); err != nil {
		t.Fatal("non-state file removed")
	}

	// Clear consumes the states and the legacy keys.
	_ = settings.SetSetting(ctx, "github_install_pending", "1")
	if err := st.Clear(ctx, s1, "", "../../etc"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.Verify(ctx, s1); ok {
		t.Fatal("cleared state still valid")
	}
	if _, ok, _ := settings.GetSetting(ctx, "github_install_pending"); ok {
		t.Fatal("legacy pending key not cleared")
	}

	// Expiry.
	now = now.Add(StateTTL + time.Millisecond)
	if ok, _ := st.Verify(ctx, s2); ok {
		t.Fatal("expired state accepted")
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), s2+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expired state file kept")
	}
}

func TestLegacyState(t *testing.T) {
	ctx := context.Background()
	settings := newMemSettings()
	now := time.UnixMilli(1_700_000_000_000)
	a := New(Config{DataPath: t.TempDir()}, settings, NewClient(""), discard())
	a.SetClock(func() time.Time { return now })
	_ = settings.SetSetting(ctx, keyLegacyState, "legacy-state")
	_ = settings.SetSetting(ctx, keyLegacyStateAt, strconv.FormatInt(now.UnixMilli(), 10))
	for _, tc := range []struct {
		state string
		want  bool
	}{{"legacy-state", true}, {"legacy-statf", false}, {"legacy", false}, {"../legacy-state", false}} {
		if ok, err := a.States().Verify(ctx, tc.state); err != nil || ok != tc.want {
			t.Errorf("Verify(%q) = %v, %v; want %v", tc.state, ok, err, tc.want)
		}
	}
	now = now.Add(2 * StateTTL)
	if ok, _ := a.States().Verify(ctx, "legacy-state"); ok {
		t.Fatal("expired legacy state accepted")
	}
	if _, ok, _ := settings.GetSetting(ctx, keyLegacyState); ok {
		t.Fatal("expired legacy state not deleted")
	}
}

func TestJWT(t *testing.T) {
	ctx := context.Background()
	key, pemText := testKeyPEM(t)
	settings := newMemSettings()
	a := New(Config{DataPath: t.TempDir(), PrivateKeyPath: filepath.Join(t.TempDir(), "missing.pem")}, settings, NewClient(""), discard())
	now := time.Unix(1_700_000_000, 0)
	a.SetClock(func() time.Time { return now })

	if _, err := a.JWT(ctx); !errors.Is(err, ErrNoPrivateKey) {
		t.Fatalf("no key: %v", err)
	}
	_ = settings.SetSetting(ctx, KeyPrivateKey, pemText)
	if _, err := a.JWT(ctx); !errors.Is(err, ErrNoAppID) {
		t.Fatalf("no app id: %v", err)
	}
	_ = settings.SetSetting(ctx, KeyAppID, "123456")
	signed, err := a.JWT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.Parse(signed, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["iss"] != float64(123456) || claims["iat"] != float64(now.Unix()-60) || claims["exp"] != float64(now.Unix()+600) {
		t.Fatalf("claims = %v", claims)
	}

	_ = settings.SetSetting(ctx, KeyPrivateKey, "not a pem")
	if _, err := a.JWT(ctx); err == nil {
		t.Fatal("invalid PEM accepted")
	}
}

func TestPrivateKeyFileFallback(t *testing.T) {
	ctx := context.Background()
	_, pemText := testKeyPEM(t)
	path := filepath.Join(t.TempDir(), "github-app.pem")
	if err := os.WriteFile(path, []byte(pemText), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New(Config{DataPath: t.TempDir(), PrivateKeyPath: path, AppID: "42"}, newMemSettings(), NewClient(""), discard())
	got, ok, err := a.PrivateKey(ctx)
	if err != nil || !ok || got != pemText {
		t.Fatalf("PrivateKey = %v, %v", ok, err)
	}
	if id, ok, _ := a.AppID(ctx); !ok || id != "42" {
		t.Fatalf("AppID = %q", id)
	}
	if _, err := a.JWT(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestJSParseInt(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
		ok   bool
	}{{"123", 123, true}, {" 42abc", 42, true}, {"-7", -7, true}, {"+8", 8, true}, {"abc", 0, false}, {"", 0, false}} {
		got, ok := jsParseInt(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("jsParseInt(%q) = %d, %v", tc.in, got, ok)
		}
	}
}

// fakeGitHub is an httptest GitHub API.
type fakeGitHub struct {
	*httptest.Server
	tokenCalls atomic.Int32
	expiresIn  time.Duration
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{expiresIn: time.Hour}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no jwt", http.StatusUnauthorized)
			return
		}
		if r.PathValue("id") == "500" {
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
			return
		}
		n := f.tokenCalls.Add(1)
		fmt.Fprintf(w, `{"token":"tok-%s-%d","expires_at":%q}`, r.PathValue("id"), n, time.Now().Add(f.expiresIn).UTC().Format(time.RFC3339))
	})
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
		}
		n := 0
		switch page {
		case 1:
			n = 100
		case 2:
			n = 3
		}
		items := make([]string, n)
		for i := range items {
			items[i] = fmt.Sprintf(`{"id":%d,"account":{"login":"acct%d","type":"User"}}`, (page-1)*100+i+1, i)
		}
		fmt.Fprint(w, "["+strings.Join(items, ",")+"]")
	})
	mux.HandleFunc("GET /installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		n := 0
		if page == 1 {
			n = 100
		} else if page == 2 {
			n = 1
		}
		items := make([]string, n)
		for i := range items {
			items[i] = fmt.Sprintf(`{"id":%d,"name":"r%d"}`, (page-1)*100+i, i)
		}
		fmt.Fprint(w, `{"repositories":[`+strings.Join(items, ",")+`]}`)
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/installation", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("owner") == "known" {
			fmt.Fprint(w, `{"id":777}`)
			return
		}
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":1,"slug":"s","pem":"p","code":%q}`, r.PathValue("code"))
	})
	mux.HandleFunc("GET /redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/x", http.StatusFound)
	})
	mux.HandleFunc("GET /big", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("["))
		chunk := []byte(strings.Repeat(" ", 1<<20))
		for i := 0; i < MaxResponseBytes>>20+1; i++ {
			_, _ = w.Write(chunk)
		}
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func newTestApp(t *testing.T, api API) (*App, *memSettings) {
	t.Helper()
	_, pemText := testKeyPEM(t)
	settings := newMemSettings()
	_ = settings.SetSetting(context.Background(), KeyPrivateKey, pemText)
	_ = settings.SetSetting(context.Background(), KeyAppID, "99")
	return New(Config{DataPath: t.TempDir()}, settings, api, discard()), settings
}

func TestInstallationTokenCaching(t *testing.T) {
	ctx := context.Background()
	gh := newFakeGitHub(t)
	a, settings := newTestApp(t, NewClient(gh.URL))

	tok1, err := a.InstallationToken(ctx, "11")
	if err != nil {
		t.Fatal(err)
	}
	tok2, _ := a.InstallationToken(ctx, "11")
	if tok1 != tok2 || gh.tokenCalls.Load() != 1 {
		t.Fatalf("cached token not reused: %q %q calls=%d", tok1, tok2, gh.tokenCalls.Load())
	}
	if tok3, _ := a.InstallationToken(ctx, "12"); tok3 == tok1 || gh.tokenCalls.Load() != 2 {
		t.Fatal("tokens must be per installation")
	}
	// Near expiry: refreshed.
	a.SetClock(func() time.Time { return time.Now().Add(56 * time.Minute) })
	if tok4, _ := a.InstallationToken(ctx, "11"); tok4 == tok1 || gh.tokenCalls.Load() != 3 {
		t.Fatalf("token near expiry not refreshed (calls=%d)", gh.tokenCalls.Load())
	}
	a.SetClock(time.Now)
	// A different App never reuses the cache.
	_ = settings.SetSetting(ctx, KeyAppID, "100")
	if _, err := a.InstallationToken(ctx, "11"); err != nil || gh.tokenCalls.Load() != 4 {
		t.Fatalf("new App reused a cached token (calls=%d, %v)", gh.tokenCalls.Load(), err)
	}
	a.ForgetTokens()
	if _, err := a.InstallationToken(ctx, "11"); err != nil || gh.tokenCalls.Load() != 5 {
		t.Fatal("ForgetTokens did not empty the cache")
	}
	_, err = a.InstallationToken(ctx, "500")
	if err == nil || !strings.HasPrefix(err.Error(), "Failed to get installation token: ") || StatusOf(err) != 500 {
		t.Fatalf("error = %v", err)
	}
}

func TestClientPagination(t *testing.T) {
	ctx := context.Background()
	gh := newFakeGitHub(t)
	c := NewClient(gh.URL)
	insts, err := c.ListInstallations(ctx, "jwt")
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 103 || insts[102].ID.String() != "103" || *insts[0].Account.Login != "acct0" {
		t.Fatalf("installations = %d", len(insts))
	}
	repos, err := c.ListInstallationRepositories(ctx, "tok")
	if err != nil || len(repos) != 101 {
		t.Fatalf("repos = %d, %v", len(repos), err)
	}
}

func TestClientSafety(t *testing.T) {
	ctx := context.Background()
	gh := newFakeGitHub(t)
	c := NewClient(gh.URL)
	m, err := c.ConvertManifest(ctx, "../../x")
	if err != nil {
		t.Fatal(err)
	}
	if m.Slug != "s" {
		t.Fatalf("conversion = %+v", m)
	}
	for _, tc := range []struct{ in, want string }{{"..", "%2E%2E"}, {".", "%2E"}, {"a/b", "a%2Fb"}, {"a..b", "a..b"}} {
		if got := PathSegment(tc.in); got != tc.want {
			t.Errorf("PathSegment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if _, err := c.do(ctx, http.MethodGet, "/redirect", authNone, "", func(int, string, string) string { return "" }); !errors.Is(err, ErrFetchFailed) {
		t.Fatalf("cross-host redirect followed: %v", err)
	}
	if _, err := c.do(ctx, http.MethodGet, "/big", authNone, "", func(int, string, string) string { return "" }); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("oversized response: %v", err)
	}
}

func TestInstallationIDForRepo(t *testing.T) {
	ctx := context.Background()
	gh := newFakeGitHub(t)
	a, settings := newTestApp(t, NewClient(gh.URL))
	if id, err := a.InstallationIDForRepo(ctx, "known", "repo"); err != nil || id != "777" {
		t.Fatalf("known = %q, %v", id, err)
	}
	_, err := a.InstallationIDForRepo(ctx, "other", "repo")
	if StatusOf(err) != 404 || !strings.HasPrefix(err.Error(), "Could not find installation for other/repo: 404") {
		t.Fatalf("unknown without fallback = %v", err)
	}
	_ = settings.SetSetting(ctx, KeyInstallationID, "555")
	if id, err := a.InstallationIDForRepo(ctx, "other", "repo"); err != nil || id != "555" {
		t.Fatalf("fallback = %q, %v", id, err)
	}
}

func TestManifestConversionDecode(t *testing.T) {
	var m ManifestConversion
	if err := decode([]byte(`{"id":12345678901,"slug":"x","owner":{"login":"me"}}`), &m); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m.ID)
	if string(b) != "12345678901" || *m.Owner.Login != "me" || m.Owner.AvatarURL != nil {
		t.Fatalf("decoded %+v", m)
	}
}
