package contract

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var stateCookieRe = regexp.MustCompile(`^docklift_github_state=([0-9a-f]{48}); Path=/; HttpOnly; SameSite=Lax; Max-Age=3600(; Secure)?$`)

func expectRedirect(t *testing.T, r *Resp, location string) {
	t.Helper()
	if r.Status != 302 {
		t.Fatalf("want 302 redirect, got %s", r)
	}
	if got := r.Header.Get("Location"); got != location {
		t.Fatalf("%s: Location = %q, want %q", r.Path, got, location)
	}
}

func stateFromCookie(t *testing.T, r *Resp) (string, string) {
	t.Helper()
	cookies := r.Header.Values("Set-Cookie")
	if len(cookies) != 1 {
		t.Fatalf("want one Set-Cookie, got %q", cookies)
	}
	m := stateCookieRe.FindStringSubmatch(cookies[0])
	if m == nil {
		t.Fatalf("Set-Cookie = %q", cookies[0])
	}
	return m[1], cookies[0]
}

func TestGithubUnconfigured(t *testing.T) {
	c := admin.Client()
	c.Get(t, "/api/github/app-status").Expect(t, 200, Obj{"configured": false, "installed": false, "appId": nil, "appName": nil,
		"appSlug": nil, "installationId": nil, "username": nil, "avatarUrl": nil, "installUrl": nil})
	c.Get(t, "/api/github/status").Expect(t, 200, Obj{"connected": false, "username": nil, "app_name": nil, "app_slug": nil})
	c.Post(t, "/api/github/install-session", map[string]any{}).ExpectError(t, 400, "GitHub App not configured. Please create one first.")
	c.Get(t, "/api/github/install").ExpectError(t, 400, "GitHub App not configured. Please create one first.")
	c.Post(t, "/api/github/check-installation", nil).ExpectError(t, 400, "GitHub App not configured")
	c.Get(t, "/api/github/installations").ExpectError(t, 500, "Failed to fetch installations")
	c.Get(t, "/api/github/repos").ExpectError(t, 500, "Failed to fetch repositories")
	for _, p := range []string{"/api/github/branches", "/api/github/tags"} {
		c.Get(t, p).ExpectError(t, 400, "Repo parameter is required (owner/name)")
		c.Get(t, p+"?repo=").ExpectError(t, 400, "Repo parameter is required (owner/name)")
		c.Get(t, p+"?repo=noslash").ExpectError(t, 400, "Invalid repo format. Expected owner/name")
		c.Get(t, p+"?repo=owner/").ExpectError(t, 400, "Invalid repo format. Expected owner/name")
		// Private repos need an App installation: refused before any GitHub call.
		c.Get(t, p+"?repo=contract-org/private-repo&type=private").ExpectError(t, 403, "Access denied: Could not verify GitHub App installation for this repository.")
	}
	c.Post(t, "/api/github/disconnect", nil).Expect(t, 200, Obj{"message": "GitHub disconnected"})
	for _, p := range []string{"/api/github/app-status", "/api/github/status", "/api/github/installations", "/api/github/repos", "/api/github/branches", "/api/github/install"} {
		mainInst.Anon().Get(t, p).ExpectError(t, 401, "Authentication required")
	}
	for _, p := range []string{"/api/github/manifest", "/api/github/install-session", "/api/github/check-installation", "/api/github/disconnect"} {
		mainInst.Anon().Post(t, p, nil).ExpectError(t, 401, "Authentication required")
	}
}

func TestGithubManifestAndSetupState(t *testing.T) {
	c := admin.Client()
	anon := mainInst.Anon()
	c.Post(t, "/api/github/manifest", map[string]any{}).ExpectError(t, 400, "appName is required")
	c.Post(t, "/api/github/manifest", map[string]any{"appName": 42}).ExpectError(t, 400, "appName is required")
	c.Post(t, "/api/github/manifest", map[string]any{"appName": ""}).ExpectError(t, 400, "appName is required")

	var state string
	t.Run("manifest on a loopback host", func(t *testing.T) {
		r := c.Post(t, "/api/github/manifest", map[string]any{"appName": "My App!! ünïcode and a very long name indeed", "returnUrl": "/settings?tab=github"})
		got := r.Expect(t, 200, Obj{"manifest": Str, "action": Re(`^https://github\.com/settings/apps/new\?state=[0-9a-f]{48}$`), "serverUrl": mainInst.BaseURL}).(map[string]any)
		var cookie string
		state, cookie = stateFromCookie(t, r)
		if strings.HasSuffix(cookie, "; Secure") {
			t.Fatal("cookie must not be Secure over http")
		}
		if got["action"] != "https://github.com/settings/apps/new?state="+state {
			t.Fatalf("action state differs from cookie state")
		}
		var manifest any
		if err := json.Unmarshal([]byte(got["manifest"].(string)), &manifest); err != nil {
			t.Fatal(err)
		}
		base := mainInst.BaseURL
		match(t, "manifest", Obj{
			"name": "docklift-my-app----n-code-and-a-very-long-n", "url": "https://github.com/SSujitX/docklift",
			"redirect_url": base + "/api/github/manifest/callback", "callback_urls": Arr{base + "/api/github/manifest/callback"},
			"setup_url": base + "/api/github/setup", "hook_attributes": Obj{"url": base + "/api/github/webhook", "active": true},
			"default_events": Arr{"push"}, "public": true, "default_permissions": Obj{"contents": "read", "metadata": "read"},
		}, manifest)
		if mainInst.Booted() {
			var st map[string]any
			b := readFileBytes(t, filepath.Join(mainInst.DataDir, "github-setup", state+".json"))
			json.Unmarshal(b, &st)
			match(t, "state file", Obj{"createdAt": Int, "returnUrl": base + "/settings?tab=github"}, st)
		}
	})
	t.Run("manifest behind a public proxy", func(t *testing.T) {
		r := c.Post(t, "/api/github/manifest", map[string]any{"appName": "proxy", "returnUrl": "https://evil.example/steal"},
			"X-Forwarded-Host", "panel.contract.test", "X-Forwarded-Proto", "https")
		got := r.Expect(t, 200, Obj{"manifest": Str, "action": Str, "serverUrl": "https://panel.contract.test"}).(map[string]any)
		st, cookie := stateFromCookie(t, r)
		if !strings.HasSuffix(cookie, "; Secure") {
			t.Fatalf("cookie behind https must be Secure: %q", cookie)
		}
		if !strings.Contains(got["manifest"].(string), `"setup_url":"https://panel.contract.test/api/github/setup"`) {
			t.Fatalf("manifest = %s", got["manifest"])
		}
		if mainInst.Booted() {
			var sf map[string]any
			json.Unmarshal(readFileBytes(t, filepath.Join(mainInst.DataDir, "github-setup", st+".json")), &sf)
			// Untrusted return URLs fall back to the configured frontend.
			match(t, "state file", Obj{"createdAt": Int, "returnUrl": frontendURL + "/settings"}, sf)
		}
		// Non-loopback hosts are always https, even when the proxy says http.
		c.Post(t, "/api/github/manifest", map[string]any{"appName": "p"}, "X-Forwarded-Host", "panel.contract.test", "X-Forwarded-Proto", "http").
			Expect(t, 200, Obj{"manifest": Str, "action": Str, "serverUrl": "https://panel.contract.test"})
		c.Post(t, "/api/github/manifest", map[string]any{"appName": "p"}, "X-Forwarded-Host", "10.1.2.3:8080").
			Expect(t, 200, Obj{"manifest": Str, "action": Str, "serverUrl": "http://10.1.2.3:8080"})
	})

	fe := frontendURL + "/settings?github_error="
	t.Run("manifest callback (public)", func(t *testing.T) {
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback"), fe+"invalid_state")
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?code=abc"), fe+"invalid_state")
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?state="+strings.Repeat("0", 48)), fe+"invalid_state")
		for _, evil := range []string{"../../.secrets", "..%2F..%2Fdata%2F.secrets", "a/b", "x.json", strings.Repeat("a", 129)} {
			expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?state="+evil), fe+"invalid_state")
			expectRedirect(t, anon.Get(t, "/api/github/manifest/callback", "Cookie", "docklift_github_state="+url.QueryEscape(evil)), fe+"invalid_state")
		}
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?state="+state), fe+"no_code")
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback", "Cookie", "other=1; docklift_github_state="+state), fe+"no_code")
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?state="+state+"&installation_id=42"), "/api/github/setup?installation_id=42")
		expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?state="+state+"&installation_id=a%26b"), "/api/github/setup?installation_id=a%26b")
		t.Run("code exchange", func(t *testing.T) {
			requireNetwork(t)
			expectRedirect(t, anon.Get(t, "/api/github/manifest/callback?state="+state+"&code=bogus-contract-code"), fe+"conversion_failed")
		})
	})
	t.Run("setup (public)", func(t *testing.T) {
		expectRedirect(t, anon.Get(t, "/api/github/setup"), fe+"invalid_installation")
		expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=12a"), fe+"invalid_installation")
		expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=12"), fe+"invalid_state")
		expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=12&state=../../.secrets"), fe+"invalid_state")
		expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=12", "Cookie", "docklift_github_state=..%2F..%2F.secrets"), fe+"invalid_state")
		// Valid state, but no App private key: fails closed before calling GitHub.
		expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=12&state="+state), fe+"setup_failed")
		expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=12", "Cookie", "docklift_github_state="+state), fe+"setup_failed")
		c.Get(t, "/api/github/status").Expect(t, 200, Obj{"connected": false, "username": nil, "app_name": nil, "app_slug": nil})
	})
	t.Run("legacy callback (public)", func(t *testing.T) {
		expectRedirect(t, anon.Get(t, "/api/github/callback"), fe+"legacy_oauth_disabled")
		expectRedirect(t, anon.Get(t, "/api/github/callback?code=x&state=y"), fe+"legacy_oauth_disabled")
		expectRedirect(t, anon.Get(t, "/api/github/callback?installation_id=x1"), fe+"legacy_oauth_disabled")
		expectRedirect(t, anon.Get(t, "/api/github/callback?installation_id=77"), "/api/github/setup?installation_id=77")
		expectRedirect(t, anon.Get(t, "/api/github/callback?installation_id=77&state=a+b%26c"), "/api/github/setup?installation_id=77&state=a+b%26c")
	})
	if mainInst.Booted() {
		if _, err := os.Stat(filepath.Join(mainInst.DataDir, ".secrets")); err != nil {
			t.Fatalf(".secrets must survive traversal attempts: %v", err)
		}
	}
}

func TestGithubWebhookWithoutSecret(t *testing.T) {
	anon := mainInst.Anon()
	body := map[string]any{"ref": "refs/heads/main", "repository": map[string]any{"clone_url": "https://github.com/a/b.git"}, "head_commit": map[string]any{"id": "abc"}}
	anon.Post(t, "/api/github/webhook", map[string]any{"zen": "hi"}, "X-GitHub-Event", "ping").Expect(t, 200, Obj{"message": "Ignoring ping event"})
	anon.Post(t, "/api/github/webhook", map[string]any{}, "X-GitHub-Event", "pull_request").Expect(t, 200, Obj{"message": "Ignoring pull_request event"})
	// Missing event header: the TS code interpolates undefined.
	anon.Post(t, "/api/github/webhook", map[string]any{}).Expect(t, 200, Obj{"message": "Ignoring undefined event"})
	anon.Post(t, "/api/github/webhook", body, "X-GitHub-Event", "push").ExpectError(t, 401, "Webhook secret not configured")
	anon.Post(t, "/api/github/webhook", body, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256=00").ExpectError(t, 401, "Webhook secret not configured")
}
