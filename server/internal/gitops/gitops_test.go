package gitops

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var ctx = context.Background()

func requireGit(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	return p
}

// git runs a setup command in dir with a fixed identity.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// core.autocrlf may be on (Git for Windows default).
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func commit(t *testing.T, dir, file, body, msg string) string {
	t.Helper()
	writeFile(t, filepath.Join(dir, file), body)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", msg)
	return git(t, dir, "rev-parse", "HEAD")
}

// upstream creates a repository with main (two commits), a "dev" branch and
// a tag v1 on the first commit.
type upstream struct {
	dir, url      string
	first, second string
	dev           string
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	requireGit(t)
	dir := filepath.Join(t.TempDir(), "upstream")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	u := &upstream{dir: dir, url: "file:///" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}
	u.first = commit(t, dir, "app.txt", "one\n", "first commit")
	git(t, dir, "tag", "v1")
	u.second = commit(t, dir, "app.txt", "two\n", "second commit")
	git(t, dir, "checkout", "-q", "-b", "dev")
	u.dev = commit(t, dir, "dev.txt", "dev\n", "dev commit")
	git(t, dir, "checkout", "-q", "main")
	return u
}

func fileRunner() *Runner { return &Runner{Protocols: []string{"file"}} }

func TestCloneDefaultBranchTagAndBranch(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	writeFile(t, filepath.Join(dest, "stale.txt"), "old")
	var out bytes.Buffer
	if err := r.Clone(ctx, u.url, dest, CloneOptions{Out: &out}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dest, "app.txt")) != "two\n" {
		t.Error("default branch content")
	}
	if _, err := os.Stat(filepath.Join(dest, "stale.txt")); !os.IsNotExist(err) {
		t.Error("existing directory must be removed before cloning")
	}
	if want := "📥 Cloning repository...\n   URL: " + u.url + "\n✅ Repository cloned successfully\n\n"; out.String() != want {
		t.Errorf("out = %q", out.String())
	}
	if b, ok := r.CurrentBranch(ctx, dest); !ok || b != "main" {
		t.Errorf("CurrentBranch = %q %v", b, ok)
	}
	if sha, ok := r.CommitSHA(ctx, dest); !ok || sha != u.second {
		t.Errorf("CommitSHA = %q %v", sha, ok)
	}
	if msg, ok := r.LastCommitMessage(ctx, dest); !ok || msg != "second commit" {
		t.Errorf("LastCommitMessage = %q %v", msg, ok)
	}

	if err := r.Clone(ctx, u.url, dest, CloneOptions{Ref: "v1"}); err != nil {
		t.Fatal(err)
	}
	if sha, _ := r.CommitSHA(ctx, dest); sha != u.first {
		t.Errorf("tag clone HEAD = %s", sha)
	}
	if b, _ := r.CurrentBranch(ctx, dest); b != "HEAD" {
		t.Errorf("tag clone is detached, branch = %q", b)
	}
	if err := r.Clone(ctx, u.url, dest, CloneOptions{Ref: "dev"}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dest, "dev.txt")) != "dev\n" {
		t.Error("dev branch content")
	}
	if err := r.Clone(ctx, u.url, dest, CloneOptions{Ref: "nope"}); err == nil {
		t.Error("clone of a missing ref must fail")
	}
}

func TestPullBranch(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	third := commit(t, u.dir, "app.txt", "three\n", "third")
	writeFile(t, filepath.Join(dest, "untracked.txt"), "junk")
	writeFile(t, filepath.Join(dest, "app.txt"), "local edit\n")

	var out bytes.Buffer
	if err := r.Pull(ctx, dest, &out, PullOptions{}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	want := "📥 Fetching latest changes...\n   Ref: main\n   ✅ Fetched from origin (branches + tags)\n   ✅ Reset to origin/main\n   ✅ Cleaned untracked files\n   📝 Updated: " + u.second[:7] + " → " + third[:7] + "\n\n"
	if out.String() != want {
		t.Errorf("out =\n%q\nwant\n%q", out.String(), want)
	}
	if readFile(t, filepath.Join(dest, "app.txt")) != "three\n" {
		t.Error("local edit must be reset")
	}
	if _, err := os.Stat(filepath.Join(dest, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked files must be cleaned")
	}

	out.Reset()
	if err := r.Pull(ctx, dest, &out, PullOptions{Ref: "main"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "   Already up to date ("+third[:7]+")\n") {
		t.Errorf("out = %q", out.String())
	}

	// Switch to another branch by name.
	out.Reset()
	if err := r.Pull(ctx, dest, &out, PullOptions{Ref: "dev"}); err != nil {
		t.Fatal(err)
	}
	if sha, _ := r.CommitSHA(ctx, dest); sha != u.dev {
		t.Errorf("HEAD = %s, want dev %s", sha, u.dev)
	}
}

func TestPullTag(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := r.Pull(ctx, dest, &out, PullOptions{Ref: "v1"}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "   ✅ Reset to tag v1\n") {
		t.Errorf("out = %q", out.String())
	}
	if sha, _ := r.CommitSHA(ctx, dest); sha != u.first {
		t.Errorf("HEAD = %s", sha)
	}
	// Moving the tag on the remote is picked up (fetch --force).
	git(t, u.dir, "tag", "-f", "v1", u.second)
	if err := r.Pull(ctx, dest, &out, PullOptions{Ref: "v1"}); err != nil {
		t.Fatal(err)
	}
	if sha, _ := r.CommitSHA(ctx, dest); sha != u.second {
		t.Errorf("moved tag HEAD = %s", sha)
	}
	// A checkout detached on a tag with no ref: the SHA is used, which is
	// neither a branch nor a tag, so the sync fails closed like 2.x.
	if err := r.Clone(ctx, u.url, dest, CloneOptions{Ref: "v1"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err := r.Pull(ctx, dest, &out, PullOptions{})
	if err == nil || !strings.HasPrefix(err.Error(), "Git sync failed: Ref \""+u.second+"\" not found as remote branch (origin/"+u.second+") or tag (refs/tags/"+u.second+")") {
		t.Errorf("detached pull err = %v", err)
	}
	if !strings.Contains(out.String(), "   Ref: "+u.second+"\n") || !strings.Contains(out.String(), "   ❌ Git sync failed: ") {
		t.Errorf("out = %q", out.String())
	}
}

func TestPullDeletedTagFailsClosed(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	git(t, u.dir, "tag", "-d", "v1")
	err := r.Pull(ctx, dest, nil, PullOptions{Ref: "v1"})
	if err == nil || err.Error() != `Git sync failed: Ref "v1" not found as remote branch (origin/v1) or tag (refs/tags/v1)` {
		t.Errorf("err = %v", err)
	}
}

func TestHostileRefsRejected(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "pwned")
	for _, ref := range []string{
		"--upload-pack=touch " + marker,
		"-b", "--help", "main;touch x", "a..b", "a b", "refs/../x", "x.lock", "@{-1}", ".hidden", "a//b", "/abs", "x/", "x\x00y", "a~1", "a^", "a:b",
	} {
		if err := ValidateRef(ref); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateRef(%q) = %v", ref, err)
		}
		if err := r.Clone(ctx, u.url, filepath.Join(t.TempDir(), "c"), CloneOptions{Ref: ref}); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("Clone ref %q: %v", ref, err)
		}
		if err := r.Pull(ctx, dest, nil, PullOptions{Ref: ref}); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("Pull ref %q: %v", ref, err)
		}
	}
	for _, ok := range []string{"main", "feature/x-1", "v1.2.3", "release_2026", u.first, "renovate/@types-node"} {
		if err := ValidateRef(ok); err != nil {
			t.Errorf("ValidateRef(%q) = %v", ok, err)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a hostile ref executed a command")
	}
}

func TestHostileURLsRejected(t *testing.T) {
	u := newUpstream(t)
	def := &Runner{}
	dest := filepath.Join(t.TempDir(), "c")
	for _, bad := range []string{
		"--upload-pack=touch /tmp/pwned",
		"-oProxyCommand=x",
		"ext::sh -c touch% /tmp/pwned",
		"fd::17",
		u.url,                       // file:// is not allowed by default
		"http://github.com/a/b.git", // plain http is not allowed by default
		u.dir,                       // local path
		"git@github.com:a/b.git",
		"ssh://git@github.com/a/b.git",
		"https:///nohost",
		"https://github.com/a/b c",
		"https://github.com/a/b\n",
		"",
	} {
		err := def.Clone(ctx, bad, dest, CloneOptions{})
		if !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Clone(%q) = %v", bad, err)
		}
	}
	if err := def.ValidateURL("https://github.com/a/b"); err != nil {
		t.Error(err)
	}
	if err := def.ValidateURL("http://github.com/a/b.git"); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("ValidateURL(http) = %v", err)
	}
}

// An origin URL set to a disallowed transport (e.g. through a PATCHed
// github_url) is refused by the protocol allow-list at fetch time.
func TestProtocolAllowListAtFetch(t *testing.T) {
	u := newUpstream(t)
	dest := filepath.Join(t.TempDir(), "proj")
	if err := fileRunner().Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	r := &Runner{}
	err := r.Pull(ctx, dest, nil, PullOptions{Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), "transport 'file' not allowed") {
		t.Errorf("err = %v", err)
	}
	if err := r.SetOriginURL(ctx, dest, "ext::sh -c id"); err != nil {
		t.Fatal(err)
	}
	if err := r.Pull(ctx, dest, nil, PullOptions{Ref: "main"}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("ext transport err = %v", err)
	}
}

func TestResetToCommit(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "xyz", "abc12", "--hard", "HEAD~1", strings.Repeat("a", 41)} {
		if err := r.ResetToCommit(ctx, dest, bad); !errors.Is(err, ErrInvalidCommit) || err.Error() != "Invalid commit SHA" {
			t.Errorf("ResetToCommit(%q) = %v", bad, err)
		}
	}
	if err := r.ResetToCommit(ctx, t.TempDir(), u.first); !errors.Is(err, ErrNotRepository) || err.Error() != "Project path is not a git repository" {
		t.Errorf("not a repo: %v", err)
	}
	if err := r.ResetToCommit(ctx, dest, u.first[:10]); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dest, "app.txt")) != "one\n" {
		t.Error("reset content")
	}
	if err := r.ResetToCommit(ctx, dest, strings.Repeat("0", 40)); err == nil {
		t.Error("unknown commit must fail")
	}
}

func TestNoParentRepositoryDiscovery(t *testing.T) {
	requireGit(t)
	parent := t.TempDir()
	git(t, parent, "init", "-q", "-b", "outer")
	commit(t, parent, "x.txt", "x", "outer")
	proj := filepath.Join(parent, "deployments", "p1")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{}
	if b, ok := r.CurrentBranch(ctx, proj); ok {
		t.Errorf("CurrentBranch found the parent repository: %q", b)
	}
	if _, ok := r.CommitSHA(ctx, proj); ok {
		t.Error("CommitSHA without .git")
	}
	if _, ok := r.LastCommitMessage(ctx, proj); ok {
		t.Error("LastCommitMessage without .git")
	}
	if err := r.Pull(ctx, proj, nil, PullOptions{Ref: "outer"}); err == nil {
		t.Error("Pull outside a repository must fail")
	}
}

func TestScrubAndVerifyOrigin(t *testing.T) {
	u := newUpstream(t)
	r := fileRunner()
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyNoCredentials(ctx, dest); err != nil {
		t.Fatal(err)
	}
	dirty := "https://x-access-token:ghs_SECRET@github.com/a/b.git"
	if err := r.SetOriginURL(ctx, dest, dirty); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyNoCredentials(ctx, dest); !errors.Is(err, ErrCredentialsRemain) || err.Error() != "Origin remote still contains credentials after scrub" {
		t.Errorf("verify dirty = %v", err)
	}
	if err := r.ScrubOrigin(ctx, dest, "https://github.com/a/b"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.OriginURL(ctx, dest); got != "https://github.com/a/b" {
		t.Errorf("origin = %q", got)
	}
	if strings.Contains(readFile(t, filepath.Join(dest, ".git", "config")), "ghs_SECRET") {
		t.Error("token left in .git/config")
	}
	if err := r.VerifyNoCredentials(ctx, dest); err != nil {
		t.Error(err)
	}
	// An option-looking URL is stored literally, never parsed as a flag.
	if err := r.SetOriginURL(ctx, dest, "--push"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.OriginURL(ctx, dest); got != "--push" {
		t.Errorf("origin = %q", got)
	}
	if err := r.ScrubOrigin(ctx, t.TempDir(), "https://github.com/a/b"); err != nil {
		t.Errorf("scrub without checkout is a no-op: %v", err)
	}
	if err := r.ScrubOrigin(ctx, dest, ""); err != nil {
		t.Errorf("scrub with empty URL is a no-op: %v", err)
	}
	git(t, dest, "remote", "remove", "origin")
	if err := r.VerifyNoCredentials(ctx, dest); err != nil {
		t.Errorf("missing origin counts as clean: %v", err)
	}
}

func TestRedactAndHasCredentials(t *testing.T) {
	tests := []struct{ in, want string }{
		{"fatal: unable to access 'https://x-access-token:ghs_abc@github.com/a/b.git/': 403", "fatal: unable to access 'https://***@github.com/a/b.git/': 403"},
		{"https://user@github.com/a", "https://***@github.com/a"},
		{"http://u:p@host:8080/x and https://k:v@h/y", "http://***@host:8080/x and https://***@h/y"},
		{"https://github.com/a/b", "https://github.com/a/b"},
		{"email me at a@b.com", "email me at a@b.com"},
	}
	for _, tc := range tests {
		if got := Redact(tc.in); got != tc.want {
			t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := Redact("token ghs_abc leaked", "ghs_abc"); got != "token *** leaked" {
		t.Errorf("secret redaction = %q", got)
	}
	for u, want := range map[string]bool{
		"https://x-access-token:t@github.com/a/b": true,
		"https://user:pat@github.com/a/b":         true,
		"https://user@github.com/a/b":             false,
		"https://github.com/a/b":                  false,
	} {
		if got := HasCredentials(u); got != want {
			t.Errorf("HasCredentials(%q) = %v", u, got)
		}
	}
}

func TestGitHubOwnerRepo(t *testing.T) {
	tests := []struct{ in, owner, repo string }{
		{"https://github.com/acme/app", "acme", "app"},
		{"https://github.com/acme/app.git", "acme", "app"},
		{"https://github.com/acme/app/tree/main", "acme", "app"},
		{"HTTPS://GitHub.com/acme/app", "acme", "app"},
	}
	for _, tc := range tests {
		o, r, ok := GitHubOwnerRepo(tc.in)
		if !ok || o != tc.owner || r != tc.repo {
			t.Errorf("GitHubOwnerRepo(%q) = %q %q %v", tc.in, o, r, ok)
		}
	}
	// Only https://github.com/<owner>/<repo> qualifies for a token: the 2.x
	// regex matched "github.com/" anywhere in the string.
	for _, bad := range []string{
		"https://gitlab.com/a/b",
		"https://evil.test/github.com/a/b",
		"https://github.com.evil.test/a/b",
		"https://evilgithub.com/a/b",
		"https://www.github.com/a/b",
		"http://github.com/a/b",
		"https://github.com:8443/a/b",
		"https://u:p@github.com/a/b",
		"git@github.com:a/b.git",
		"https://github.com/only-owner",
	} {
		if o, r, ok := GitHubOwnerRepo(bad); ok {
			t.Errorf("GitHubOwnerRepo(%q) = %q %q, want no match", bad, o, r)
		}
	}
}

func TestNormalizeGitHubURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://github.com/a/b.git":      "https://github.com/a/b.git",
		"HTTP://WWW.GitHub.com/a/b":      "https://github.com/a/b",
		"https://www.github.com/a/b":     "https://github.com/a/b",
		"https://github.com/a/b":         "https://github.com/a/b",
		"https://evil.test/github.com/a": "https://evil.test/github.com/a",
		"http://github.com:80/a/b":       "https://github.com/a/b",
		"https://github.com:443/a/b":     "https://github.com/a/b",
		"https://github.com:8443/a/b":    "https://github.com:8443/a/b",
		"http://u:p@github.com/a/b":      "http://u:p@github.com/a/b",
		"git@github.com:a/b.git":         "git@github.com:a/b.git",
	} {
		if got := NormalizeGitHubURL(in); got != want {
			t.Errorf("NormalizeGitHubURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// Credentials are never sent to a remote outside the credential origins:
// Clone and Pull fail before git runs, so no request reaches the remote.
func TestCredentialsOnlyForAllowedOrigins(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	defer srv.Close()
	creds := InstallationCredentials("ghs_SHOULD_NOT_LEAVE")
	r := &Runner{Protocols: []string{"http", "https"}}
	if err := r.Clone(ctx, srv.URL+"/a/b", filepath.Join(t.TempDir(), "c"), CloneOptions{Credentials: creds}); !errors.Is(err, ErrCredentialsOrigin) {
		t.Fatalf("clone to a non-GitHub origin with credentials = %v", err)
	}
	if hits != 0 {
		t.Fatalf("remote was contacted %d times", hits)
	}
	if !(&Runner{}).credentialsAllowed("https://github.com/a/b") {
		t.Error("https://github.com must be a credential origin")
	}
	for _, bad := range []string{"http://github.com/a/b", "https://www.github.com/a/b", "https://github.com.evil.test/a/b", "https://x:y@github.com/a/b", srv.URL + "/a/b"} {
		if (&Runner{}).credentialsAllowed(bad) {
			t.Errorf("credentials allowed for %q", bad)
		}
	}

	// Pull: an origin that is not the expected one (or not allowed) is refused.
	u := newUpstream(t)
	dest := filepath.Join(t.TempDir(), "proj")
	fr := fileRunner()
	if err := fr.Clone(ctx, u.url, dest, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := fr.SetOriginURL(ctx, dest, srv.URL+"/a/b"); err != nil {
		t.Fatal(err)
	}
	pr := &Runner{Protocols: []string{"http"}, CredentialOrigins: []string{srv.URL}}
	err := pr.Pull(ctx, dest, nil, PullOptions{Ref: "main", Credentials: creds, ExpectedOrigin: "https://github.com/a/b"})
	if !errors.Is(err, ErrCredentialsOrigin) {
		t.Fatalf("pull with mismatched origin = %v", err)
	}
	if err := (&Runner{Protocols: []string{"http"}}).Pull(ctx, dest, nil, PullOptions{Ref: "main", Credentials: creds}); !errors.Is(err, ErrCredentialsOrigin) {
		t.Fatalf("pull to a non-allowed origin = %v", err)
	}
	if hits != 0 {
		t.Fatalf("remote was contacted %d times", hits)
	}
}

// TestCredentialsNeverStored serves the upstream over smart HTTP (git
// http-backend through CGI) behind Basic auth: the clone and pull succeed
// with the token, which never reaches .git/config, and a wrong token's
// failure is redacted.
func TestCredentialsNeverStored(t *testing.T) {
	gitBin := requireGit(t)
	u := newUpstream(t)
	git(t, u.dir, "config", "http.receivepack", "false")
	const token = "ghs_TOPSECRET123"
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
	backend := &cgi.Handler{
		Path: gitBin,
		Args: []string{"http-backend"},
		Env:  []string{"GIT_PROJECT_ROOT=" + filepath.Dir(u.dir), "GIT_HTTP_EXPORT_ALL=1"},
	}
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != want {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		sawAuth = true
		backend.ServeHTTP(w, r)
	}))
	defer srv.Close()

	r := &Runner{Protocols: []string{"http"}, CredentialOrigins: []string{srv.URL}}
	remote := srv.URL + "/upstream"
	dest := filepath.Join(t.TempDir(), "proj")
	if err := r.Clone(ctx, remote, dest, CloneOptions{Credentials: InstallationCredentials(token)}); err != nil {
		if strings.Contains(err.Error(), "http-backend") || strings.Contains(err.Error(), "500") {
			t.Skipf("git http-backend unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if !sawAuth {
		t.Fatal("server never saw the Authorization header")
	}
	cfg := readFile(t, filepath.Join(dest, ".git", "config"))
	if strings.Contains(cfg, token) || strings.Contains(cfg, "x-access-token") || strings.Contains(cfg, "extraheader") {
		t.Fatalf(".git/config holds credentials:\n%s", cfg)
	}
	commit(t, u.dir, "app.txt", "http\n", "over http")
	var out bytes.Buffer
	if err := r.Pull(ctx, dest, &out, PullOptions{Ref: "main", Credentials: InstallationCredentials(token)}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if readFile(t, filepath.Join(dest, "app.txt")) != "http\n" {
		t.Error("pull over http did not update")
	}
	if strings.Contains(readFile(t, filepath.Join(dest, ".git", "config")), token) {
		t.Fatal("token stored by pull")
	}

	// Without credentials the prompt is disabled, so git fails instead of hanging.
	if err := r.Pull(ctx, dest, nil, PullOptions{Ref: "main"}); err == nil {
		t.Error("pull without credentials must fail")
	}
	// A wrong token embedded in the URL is redacted from the error.
	bad := strings.Replace(srv.URL, "http://", "http://x-access-token:ghs_WRONG@", 1) + "/upstream"
	err := r.Clone(ctx, bad, filepath.Join(t.TempDir(), "c"), CloneOptions{})
	if err == nil || strings.Contains(err.Error(), "ghs_WRONG") {
		t.Errorf("clone with bad token err = %v", err)
	}
	var gerr *Error
	if errors.As(err, &gerr) {
		for _, a := range gerr.Args {
			if strings.Contains(a, "ghs_WRONG") {
				t.Errorf("args not redacted: %v", gerr.Args)
			}
		}
	}
}

func TestTimeout(t *testing.T) {
	u := newUpstream(t)
	r := &Runner{Protocols: []string{"file"}, NetworkTimeout: 1}
	err := r.Clone(ctx, u.url, filepath.Join(t.TempDir(), "c"), CloneOptions{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
}
