package github

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/githubapp"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/store"
)

type enqueued struct {
	projectID, trigger, message string
}

type fakeDeployer struct {
	mu    sync.Mutex
	calls []enqueued
}

func (f *fakeDeployer) EnqueueDeploy(_ context.Context, projectID, trigger, commitMessage string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, enqueued{projectID, trigger, commitMessage})
	return nil
}

func (f *fakeDeployer) all() []enqueued {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]enqueued(nil), f.calls...)
}

// fakeGitHub answers the API calls the routes make.
type fakeGitHub struct {
	*httptest.Server
	pem          string
	conversions  atomic.Int32
	tokenCalls   atomic.Int32
	failInstalls bool
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGitHub{pem: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("code") != "good-code" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		f.conversions.Add(1)
		writeJSON(w, map[string]any{"id": 4242, "slug": "docklift-test", "name": "docklift-test", "client_id": "cid", "client_secret": "csecret",
			"pem": f.pem, "webhook_secret": "whsecret", "owner": map[string]any{"login": "owner1", "avatar_url": "https://avatars.test/o"}})
	})
	mux.HandleFunc("GET /app/installations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "31" {
			http.Error(w, "{}", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"id": 31, "account": map[string]any{"login": "inst-owner", "avatar_url": "https://avatars.test/i"}})
	})
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		if f.failInstalls {
			http.Error(w, "{}", http.StatusInternalServerError)
			return
		}
		writeJSON(w, []any{
			map[string]any{"id": 1, "account": map[string]any{"login": "alice", "type": "User"}},
			map[string]any{"id": 2, "account": map[string]any{"login": "org", "type": "Organization"}},
			map[string]any{"id": 3, "account": map[string]any{"login": "broken"}},
		})
	})
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "3" {
			http.Error(w, "suspended", http.StatusForbidden)
			return
		}
		f.tokenCalls.Add(1)
		writeJSON(w, map[string]any{"token": "tok" + r.PathValue("id"), "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	})
	mux.HandleFunc("GET /installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var repos []any
		switch r.Header.Get("Authorization") {
		case "token tok1":
			// Two pages: 100 + 1.
			n := map[int]int{1: 100, 2: 1}[page]
			for i := 0; i < n; i++ {
				id := (page-1)*100 + i + 1000
				repos = append(repos, map[string]any{"id": id, "name": fmt.Sprintf("r%d", id), "full_name": fmt.Sprintf("alice/r%d", id),
					"owner": map[string]any{"login": "alice"}, "updated_at": fmt.Sprintf("2026-01-01T00:%02d:00Z", id%60)})
			}
		case "token tok2":
			if page == 1 {
				repos = []any{
					map[string]any{"id": 1000, "name": "dup", "full_name": "alice/dup"},
					map[string]any{"id": 2000, "name": "orgrepo", "full_name": "Org/orgrepo", "updated_at": "2027-01-01T00:00:00Z", "private": true},
				}
			}
		}
		writeJSON(w, map[string]any{"repositories": repos})
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/installation", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "{}", http.StatusNotFound)
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("repo") == "missing" {
			http.Error(w, "{}", http.StatusNotFound)
			return
		}
		if r.PathValue("repo") == "teapot" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		writeJSON(w, []any{map[string]any{"name": "main"}, map[string]any{"name": "dev"}})
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/tags", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var tags []any
		n := map[int]int{1: 100, 2: 2}[page]
		for i := 0; i < n; i++ {
			tags = append(tags, map[string]any{"name": fmt.Sprintf("v%d", (page-1)*100+i)})
		}
		writeJSON(w, tags)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type env struct {
	t   *testing.T
	d   *app.Deps
	gh  *fakeGitHub
	app *githubapp.App
	dep *fakeDeployer
	hs  *handlers
	h   http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	d.Config.FrontendURL = "http://localhost:3000"
	gh := newFakeGitHub(t)
	a := githubapp.New(githubapp.Config{DataPath: d.Config.DataPath, PrivateKeyPath: filepath.Join(t.TempDir(), "none.pem")}, d.Store, githubapp.NewClient(gh.URL), d.Log)
	dep := &fakeDeployer{}
	hs := newHandlers(d, Options{App: a, Deployer: dep})
	r := chi.NewRouter()
	r.Use(httpx.ParseBody)
	mount(r, hs)
	return &env{t: t, d: d, gh: gh, app: a, dep: dep, hs: hs, h: r}
}

// do sends a request; trusted marks the peer as a trusted proxy.
func (e *env) do(method, target string, body []byte, headers ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req := httptest.NewRequest(method, target, rd)
	req.Host = "localhost:8080"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Add(headers[i], headers[i+1])
	}
	req = req.WithContext(httpx.WithPeerTrusted(req.Context(), true))
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) json(rec *httptest.ResponseRecorder) map[string]any {
	e.t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		e.t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return m
}

func (e *env) set(kv ...string) {
	for i := 0; i+1 < len(kv); i += 2 {
		if err := e.d.Store.SetSetting(context.Background(), kv[i], kv[i+1]); err != nil {
			e.t.Fatal(err)
		}
	}
}

func (e *env) setting(key string) string {
	v, _, _ := e.d.Store.GetSetting(context.Background(), key)
	return v
}

func (e *env) configureApp() {
	e.set(githubapp.KeyAppID, "4242", githubapp.KeyPrivateKey, e.gh.pem, githubapp.KeyAppSlug, "docklift-test")
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func stateFromSetCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	c := rec.Header().Values("Set-Cookie")
	if len(c) != 1 || !strings.HasPrefix(c[0], stateCookie+"=") {
		t.Fatalf("Set-Cookie = %q", c)
	}
	v, _, _ := strings.Cut(strings.TrimPrefix(c[0], stateCookie+"="), ";")
	return v
}

func TestManifestFlow(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/manifest", mustJSON(map[string]any{"appName": "My App", "returnUrl": "/projects/new"}))
	if rec.Code != 200 {
		t.Fatalf("manifest: %d %s", rec.Code, rec.Body)
	}
	body := e.json(rec)
	state := stateFromSetCookie(t, rec)
	if body["action"] != "https://github.com/settings/apps/new?state="+state || body["serverUrl"] != "http://localhost:8080" {
		t.Fatalf("manifest body = %v", body)
	}
	if !strings.Contains(body["manifest"].(string), `"name":"docklift-my-app"`) || e.setting(githubapp.KeyPendingAppName) != "my-app" {
		t.Fatalf("manifest = %v", body["manifest"])
	}

	// Bad state, bad code.
	rec = e.do("GET", "/manifest/callback?code=good-code&state=deadbeef", nil)
	expectLocation(t, rec, "http://localhost:3000/settings?github_error=invalid_state")
	rec = e.do("GET", "/manifest/callback?code=bad&state="+state, nil)
	expectLocation(t, rec, "http://localhost:3000/settings?github_error=conversion_failed")
	if e.setting(githubapp.KeyAppID) != "" {
		t.Fatal("credentials stored after a failed conversion")
	}

	rec = e.do("GET", "/manifest/callback?code=good-code&state="+state, nil)
	expectLocation(t, rec, "https://github.com/apps/docklift-test/installations/new")
	for k, want := range map[string]string{
		githubapp.KeyAppID: "4242", githubapp.KeyAppSlug: "docklift-test", githubapp.KeyClientSecret: "csecret",
		githubapp.KeyPrivateKey: e.gh.pem, githubapp.KeyWebhookSecret: "whsecret", githubapp.KeyUsername: "owner1",
	} {
		if got := e.setting(k); got != want {
			t.Errorf("setting %s = %q", k, got)
		}
	}
	installState := stateFromSetCookie(t, rec)
	if installState == state {
		t.Fatal("install state must be fresh")
	}
	// The manifest state is single-use.
	rec = e.do("GET", "/manifest/callback?code=good-code&state="+state, nil)
	expectLocation(t, rec, "http://localhost:3000/settings?github_error=invalid_state")

	// Installation callback: unknown installation, then the real one.
	rec = e.do("GET", "/setup?installation_id=99", nil, "Cookie", stateCookie+"="+installState)
	expectLocation(t, rec, "http://localhost:3000/settings?github_error=installation_verify_failed")
	rec = e.do("GET", "/setup?installation_id=31", nil, "Cookie", "a=b; "+stateCookie+"="+installState)
	expectLocation(t, rec, "http://localhost:8080/projects/new?github=connected")
	if e.setting(githubapp.KeyInstallationID) != "31" || e.setting(githubapp.KeyUsername) != "inst-owner" {
		t.Fatal("installation not stored")
	}
	if c := rec.Header().Values("Set-Cookie"); len(c) != 1 || !strings.Contains(c[0], "Max-Age=0") {
		t.Fatalf("cookie not cleared: %q", c)
	}
	rec = e.do("GET", "/setup?installation_id=31", nil, "Cookie", stateCookie+"="+installState)
	expectLocation(t, rec, "http://localhost:3000/settings?github_error=invalid_state")

	// Malformed cookie encoding fails closed.
	rec = e.do("GET", "/setup?installation_id=31", nil, "Cookie", stateCookie+"=%zz")
	expectLocation(t, rec, "http://localhost:3000/settings?github_error=setup_failed")

	rec = e.do("GET", "/status", nil)
	if got := e.json(rec); got["connected"] != true || got["installUrl"] != "https://github.com/apps/docklift-test/installations/new" {
		t.Fatalf("status = %v", got)
	}
}

func expectLocation(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
		t.Fatalf("got %d Location %q, want %q", rec.Code, rec.Header().Get("Location"), want)
	}
}

func TestRedirectBodies(t *testing.T) {
	e := newEnv(t)
	rec := e.do("GET", "/callback?installation_id=7&state=a%3Cb", nil, "Accept", "text/html")
	expectLocation(t, rec, "/api/github/setup?installation_id=7&state=a%3Cb")
	if !strings.Contains(rec.Body.String(), "Redirecting to /api/github/setup?installation_id=7&amp;state=a%3Cb") {
		t.Fatalf("html body = %q", rec.Body)
	}
	rec = e.do("GET", "/callback", nil)
	if rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" || !strings.HasPrefix(rec.Body.String(), "Found. Redirecting to ") {
		t.Fatalf("text body = %q", rec.Body)
	}
	rec = e.do("GET", "/callback", nil, "Accept", "application/json")
	if rec.Body.Len() != 0 {
		t.Fatalf("json accept body = %q", rec.Body)
	}
}

func TestCheckInstallationAndInstallations(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/check-installation", nil)
	if rec.Code != 400 {
		t.Fatalf("unconfigured: %d", rec.Code)
	}
	e.configureApp()
	e.set(githubapp.KeyInstallationID, "2")
	got := e.json(e.do("POST", "/check-installation", nil))
	if got["found"] != true || got["installationId"] != float64(2) || got["username"] != "org" || len(got["installations"].([]any)) != 3 {
		t.Fatalf("check-installation = %v", got)
	}
	got = e.json(e.do("GET", "/installations", nil))
	insts := got["installations"].([]any)
	if third := insts[2].(map[string]any); third["type"] != "User" || third["avatar_url"] != "" || third["login"] != "broken" {
		t.Fatalf("installations = %v", insts)
	}
}

func TestRepos(t *testing.T) {
	e := newEnv(t)
	if rec := e.do("GET", "/repos", nil); rec.Code != 500 {
		t.Fatalf("unconfigured repos: %d", rec.Code)
	}
	e.configureApp()
	rec := e.do("GET", "/repos", nil)
	if rec.Code != 200 {
		t.Fatalf("repos: %d %s", rec.Code, rec.Body)
	}
	got := e.json(rec)
	repos := got["repositories"].([]any)
	if len(repos) != 102 {
		t.Fatalf("want 101 + 1 deduplicated repos, got %d", len(repos))
	}
	first := repos[0].(map[string]any)
	if first["name"] != "orgrepo" || first["owner"] != "Org" || first["private"] != true {
		t.Fatalf("newest first: %v", first)
	}
	if _, has := first["permissions"]; has {
		t.Fatal("absent fields must be omitted")
	}
	failed := got["failedInstallations"].([]any)
	if len(failed) != 1 || failed[0].(map[string]any)["login"] != "broken" || !strings.HasPrefix(failed[0].(map[string]any)["error"].(string), "Failed to get installation token: ") {
		t.Fatalf("failed = %v", failed)
	}
	tokenCalls := e.gh.tokenCalls.Load()
	got = e.json(e.do("GET", "/repos?owner=%20ORG%20", nil))
	if repos := got["repositories"].([]any); len(repos) != 1 {
		t.Fatalf("owner filter: %v", repos)
	}
	if e.gh.tokenCalls.Load() != tokenCalls {
		t.Fatal("installation tokens not cached between requests")
	}

	// Installations list failing: fall back to the saved installation.
	e.gh.failInstalls = true
	if rec := e.do("GET", "/repos", nil); rec.Code != 401 {
		t.Fatalf("fallback without installation: %d", rec.Code)
	}
	e.set(githubapp.KeyInstallationID, "2")
	got = e.json(e.do("GET", "/repos", nil))
	if got["fallbackSingle"] != true || len(got["repositories"].([]any)) != 2 {
		t.Fatalf("fallback = %v", got)
	}
	e.set(githubapp.KeyInstallationID, "3")
	rec = e.do("GET", "/repos", nil)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "Failed to get installation token") {
		t.Fatalf("fallback failure = %d %s", rec.Code, rec.Body)
	}
}

func TestBranchesAndTags(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct {
		target string
		status int
		body   string
	}{
		{"/branches", 400, `{"error":"Repo parameter is required (owner/name)"}`},
		{"/branches?repo=a", 400, `{"error":"Invalid repo format. Expected owner/name"}`},
		{"/branches?repo=a/b&type=private", 403, `{"error":"Access denied: Could not verify GitHub App installation for this repository."}`},
		{"/branches?repo=a/b/../../x", 404, `{"error":"Repository not found"}`},
		{"/branches?repo=../b", 404, `{"error":"Repository not found"}`},
		{"/branches?repo=a/b", 200, `["main","dev"]`},
		{"/branches?repo=a/missing", 404, `{"error":"Repository not found"}`},
		{"/branches?repo=a/teapot", 500, `{"error":"GitHub API Error: I'm a teapot"}`},
	} {
		rec := e.do("GET", tc.target, nil)
		if rec.Code != tc.status || rec.Body.String() != tc.body {
			t.Errorf("%s: %d %s", tc.target, rec.Code, rec.Body)
		}
	}
	var tags []string
	rec := e.do("GET", "/tags?repo=a/b", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil || len(tags) != 102 || tags[0] != "v0" {
		t.Fatalf("tags: %d %v", len(tags), err)
	}
}

func TestDisconnectAndStatus(t *testing.T) {
	e := newEnv(t)
	e.set(githubapp.KeyInstallationID, "5", githubapp.KeyUsername, "u", githubapp.KeyAppName, "n")
	if got := e.json(e.do("GET", "/status", nil)); got["warning"] != "Private key not found" {
		t.Fatalf("status = %v", got)
	}
	if rec := e.do("POST", "/disconnect", nil); rec.Body.String() != `{"message":"GitHub disconnected"}` {
		t.Fatalf("disconnect = %s", rec.Body)
	}
	if rec := e.do("GET", "/status", nil); rec.Body.String() != `{"connected":false,"username":null,"app_name":"n","app_slug":null}` {
		t.Fatalf("status = %s", rec.Body)
	}
}

func TestInstallSession(t *testing.T) {
	e := newEnv(t)
	if rec := e.do("POST", "/install-session", mustJSON(map[string]any{})); rec.Code != 400 {
		t.Fatalf("unconfigured: %d", rec.Code)
	}
	e.configureApp()
	for _, tc := range []struct {
		body any
		want string
	}{
		{map[string]any{"returnUrl": "/projects/new"}, "http://localhost:8080/projects/new"},
		{map[string]any{"returnUrl": nil, "return_url": "http://127.0.0.1:3000/x"}, "http://127.0.0.1:3000/x"},
		{map[string]any{"returnUrl": "https://evil.example/x"}, "http://localhost:3000/settings"},
		{map[string]any{"returnUrl": `/\evil.example`}, "http://localhost:3000/settings"},
		{map[string]any{"returnUrl": "javascript:alert(1)"}, "http://localhost:3000/settings"},
	} {
		rec := e.do("POST", "/install-session", mustJSON(tc.body))
		if rec.Code != 200 || rec.Body.String() != `{"installUrl":"https://github.com/apps/docklift-test/installations/new"}` {
			t.Fatalf("install-session: %d %s", rec.Code, rec.Body)
		}
		state := stateFromSetCookie(t, rec)
		if got := e.app.States().ReturnURL(state); got != tc.want {
			t.Errorf("return URL for %v = %q, want %q", tc.body, got, tc.want)
		}
	}
	rec := e.do("GET", "/install?return_url=/x", nil)
	expectLocation(t, rec, "https://github.com/apps/docklift-test/installations/new")
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func push(ref, url string, extra map[string]any) []byte {
	p := map[string]any{
		"ref":         ref,
		"repository":  map[string]any{"clone_url": url},
		"head_commit": map[string]any{"id": "abcdef1234567890", "message": "fix things"},
		"pusher":      map[string]any{"name": "dev"},
	}
	for k, v := range extra {
		p[k] = v
	}
	return mustJSON(p)
}

func (e *env) project(name, repoURL, branch string, autoDeploy bool) *store.Project {
	e.t.Helper()
	src := "github"
	p, err := e.d.Store.CreateProject(context.Background(), store.ProjectCreate{Name: name, SourceType: &src, GithubURL: &repoURL, GithubBranch: &branch, AutoDeploy: &autoDeploy})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func TestWebhook(t *testing.T) {
	e := newEnv(t)
	repo := "https://github.com/acme/app"
	main := e.project("app-main", repo+".git", "main", true)
	e.project("app-dev", repo, "develop", true)
	e.project("app-off", repo, "main", false)
	e.project("other", "https://github.com/acme/other", "main", true)

	body := push("refs/heads/main", repo+".git", nil)
	hook := func(b []byte, headers ...string) *httptest.ResponseRecorder {
		return e.do("POST", "/webhook", b, append([]string{"X-GitHub-Event", "push"}, headers...)...)
	}
	if rec := e.do("POST", "/webhook", body, "X-GitHub-Event", "ping"); rec.Body.String() != `{"message":"Ignoring ping event"}` {
		t.Fatalf("ping = %s", rec.Body)
	}
	if rec := e.do("POST", "/webhook", body); rec.Body.String() != `{"message":"Ignoring undefined event"}` {
		t.Fatalf("no event = %s", rec.Body)
	}
	if rec := hook(body); rec.Code != 401 || rec.Body.String() != `{"error":"Webhook secret not configured"}` {
		t.Fatalf("no secret = %d %s", rec.Code, rec.Body)
	}
	e.set(githubapp.KeyWebhookSecret, "s3cret")
	for _, tc := range []struct {
		name string
		sig  []string
		body []byte
		want string
	}{
		{"missing", nil, body, `{"error":"Missing webhook signature"}`},
		{"empty", []string{"X-Hub-Signature-256", ""}, body, `{"error":"Missing webhook signature"}`},
		{"wrong secret", []string{"X-Hub-Signature-256", sign("nope", body)}, body, `{"error":"Invalid signature"}`},
		{"no prefix", []string{"X-Hub-Signature-256", strings.TrimPrefix(sign("s3cret", body), "sha256=")}, body, `{"error":"Invalid signature"}`},
		{"uppercase", []string{"X-Hub-Signature-256", strings.ToUpper(sign("s3cret", body))}, body, `{"error":"Invalid signature"}`},
		{"tampered", []string{"X-Hub-Signature-256", sign("s3cret", body)}, append(append([]byte(nil), body...), ' '), `{"error":"Invalid signature"}`},
	} {
		if rec := hook(tc.body, tc.sig...); rec.Code != 401 || rec.Body.String() != tc.want {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
	if n := len(e.dep.all()); n != 0 {
		t.Fatalf("deploys queued by rejected webhooks: %d", n)
	}

	send := func(b []byte) map[string]any { return e.json(hook(b, "X-Hub-Signature-256", sign("s3cret", b))) }
	if got := send(push("refs/heads/main", "https://github.com/acme/unknown", nil)); got["message"] != "No matching projects with auto-deploy enabled" {
		t.Fatalf("unknown repo = %v", got)
	}
	if got := send(push("refs/heads/main", repo, map[string]any{"deleted": true})); got["message"] != "Ignoring branch deletion or empty push" {
		t.Fatalf("deleted = %v", got)
	}
	if got := send(push("refs/heads/feature", repo, nil)); got["message"] != "No deployments triggered" || len(got["triggered"].([]any)) != 0 {
		t.Fatalf("non-matching branch = %v", got)
	}
	if n := len(e.dep.all()); n != 0 {
		t.Fatalf("deploy queued for a non-matching branch: %d", n)
	}

	start := time.Now()
	got := send(body)
	if time.Since(start) > 2*time.Second {
		t.Fatal("webhook blocked")
	}
	want := `{"branch":"main","commit":"abcdef1","message":"Auto-deploy triggered","skipped":["app-dev (ref mismatch: main != develop)"],"triggered":["app-main"]}`
	if b, _ := json.Marshal(got); string(b) != want {
		t.Fatalf("push = %s", b)
	}
	calls := e.dep.all()
	if len(calls) != 1 || calls[0] != (enqueued{main.ID, "webhook", "fix things"}) {
		t.Fatalf("queue calls = %+v", calls)
	}
	// Second push inside the cooldown: skipped, no second queue call.
	got = send(body)
	if got["message"] != "No deployments triggered" || len(e.dep.all()) != 1 {
		t.Fatalf("cooldown push = %v (calls %d)", got, len(e.dep.all()))
	}
	// After the cooldown the project is queued again.
	later := time.Now().Add(deployCooldown + time.Second)
	e.hs.cooldown.now = func() time.Time { return later }
	send(body)
	if len(e.dep.all()) != 2 {
		t.Fatalf("push after cooldown not queued: %d", len(e.dep.all()))
	}
}

func TestWebhookWithoutDeployer(t *testing.T) {
	e := newEnv(t)
	r := chi.NewRouter()
	r.Use(httpx.ParseBody)
	MountWith(r, e.d, Options{App: e.app})
	e.h = r
	e.set(githubapp.KeyWebhookSecret, "k")
	e.project("p", "https://github.com/a/b", "", true)
	b := push("refs/tags/v1", "https://github.com/a/b.git", nil)
	got := e.json(e.do("POST", "/webhook", b, "X-GitHub-Event", "push", "X-Hub-Signature-256", sign("k", b)))
	if got["message"] != "Auto-deploy triggered" || got["branch"] != "v1" {
		t.Fatalf("tag push = %v", got)
	}
	b = push("refs/pull/1/head", "https://github.com/a/b", nil)
	got = e.json(e.do("POST", "/webhook", b, "X-GitHub-Event", "push", "X-Hub-Signature-256", sign("k", b)))
	if sk := got["skipped"].([]any); len(sk) != 1 || sk[0] != "p (unsupported ref: refs/pull/1/head)" {
		t.Fatalf("pull ref = %v", got)
	}
}

func TestPublicBaseURL(t *testing.T) {
	e := newEnv(t)
	h := &handlers{d: e.d}
	for _, tc := range []struct {
		host, xfh, xfp string
		trusted        bool
		want           string
	}{
		{"localhost:8080", "", "", true, "http://localhost:8080"},
		{"localhost:8080", "", "https", true, "https://localhost:8080"},
		{"127.0.0.1", "panel.example.com", "http", true, "https://panel.example.com"},
		{"127.0.0.1", "panel.example.com", "http", false, "http://127.0.0.1"},
		{"10.0.0.5:8080", "", "", true, "http://10.0.0.5:8080"},
		{"Panel.Example.com:443/evil?x", "", "", true, "https://panel.example.com"},
		{"", "", "", true, "http://localhost:3000"},
		{"bad host", "", "", true, "http://localhost:3000"},
		{"[::1]:9000", "", "", true, "http://[::1]:9000"},
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = tc.host
		if tc.xfh != "" {
			req.Header.Set("X-Forwarded-Host", tc.xfh)
		}
		if tc.xfp != "" {
			req.Header.Set("X-Forwarded-Proto", tc.xfp)
		}
		req = req.WithContext(httpx.WithPeerTrusted(req.Context(), tc.trusted))
		if got := h.publicBaseURL(req); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc, got, tc.want)
		}
	}
}

func TestSanitizeReturnURL(t *testing.T) {
	e := newEnv(t)
	e.d.Config.CORSOrigins = []string{" http://vite.test:3600 "}
	h := &handlers{d: e.d}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "panel.test:8080"
	fb := "http://localhost:3000/settings"
	for _, tc := range []struct {
		in       string
		isString bool
		want     string
	}{
		{"", true, fb},
		{"  ", true, fb},
		{"/x", false, fb},
		{"/settings?tab=github", true, "http://panel.test:8080/settings?tab=github"},
		{"//evil.example/x", true, fb},
		{`/\evil.example/x`, true, fb},
		{"/a/../b", true, "http://panel.test:8080/b"},
		{"http://LOCALHOST:3000", true, "http://localhost:3000/"},
		{"http://[::1]:3000/p", true, "http://[::1]:3000/p"},
		{"http://127.0.0.1:3000/p", true, "http://127.0.0.1:3000/p"},
		{"http://localhost:3001/p", true, fb},
		{"http://vite.test:3600/new", true, "http://vite.test:3600/new"},
		{"https://panel.test:8080/x", true, fb},
		{"http://panel.test:8080/x", true, "http://panel.test:8080/x"},
		{"javascript:alert(1)", true, fb},
		{"http://user@evil.example/", true, fb},
		{"ftp://localhost:3000/", true, fb},
	} {
		if got := h.sanitizeReturnURL(tc.in, tc.isString, req); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHelpers(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"My App!! ünïcode and a very long name indeed", "my-app----n-code-and-a-very-long-n"},
		{"İx", "i-x"},
		{"a😀b", "a--b"},
		{"KELVIN", "kelvin"},
	} {
		if got := sanitizeAppName(tc.in); got != tc.want {
			t.Errorf("sanitizeAppName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"/a b?c=<d>&e=%20%zz", "/a%20b?c=%3Cd%3E&e=%20%25zz"},
		{"https://x/ü", "https://x/%C3%BC"},
		{"a\r\nb", "a%0D%0Ab"},
	} {
		if got := encodeURL(tc.in); got != tc.want {
			t.Errorf("encodeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := js.EncodeURIComponent("a b&c/~*"); got != "a%20b%26c%2F~*" {
		t.Errorf("js.EncodeURIComponent = %q", got)
	}
	if got := formEncode("a b&c~*"); got != "a+b%26c%7E*" {
		t.Errorf("formEncode = %q", got)
	}
	for _, tc := range []struct{ accept, want string }{
		{"", "text"}, {"*/*", "text"}, {"text/html", "html"}, {"text/html,application/xhtml+xml,*/*;q=0.8", "html"},
		{"application/json", ""}, {"text/plain;q=0.5, text/html", "html"},
	} {
		if got := negotiateRedirect(tc.accept); got != tc.want {
			t.Errorf("negotiate(%q) = %q, want %q", tc.accept, got, tc.want)
		}
	}
	if _, err := decodeURIComponent("%E0%A4%A"); err == nil {
		t.Error("truncated escape accepted")
	}
	if v, err := decodeURIComponent("a%2Fb"); err != nil || v != "a/b" {
		t.Errorf("decode = %q %v", v, err)
	}
}

func TestStateFilesStayInside(t *testing.T) {
	e := newEnv(t)
	secret := filepath.Join(e.d.Config.DataPath, ".secrets")
	if err := os.WriteFile(secret, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, evil := range []string{"../.secrets", "..%2F.secrets", "../../" + filepath.Base(e.d.Config.DataPath) + "/.secrets"} {
		rec := e.do("GET", "/manifest/callback?state="+url.QueryEscape(evil), nil, "Cookie", stateCookie+"="+url.QueryEscape(evil))
		expectLocation(t, rec, "http://localhost:3000/settings?github_error=invalid_state")
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatal(".secrets removed")
	}
}
