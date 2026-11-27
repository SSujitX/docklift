// Package gitops runs the git CLI for project checkouts: clone, pull a
// branch or tag with fetch + hard reset, read HEAD, roll back to a commit and
// manage the origin URL (the port of backend/src/services/git.ts).
//
// Every invocation is hardened: git is executed directly (never through a
// shell) with a timeout; terminal prompts, credential helpers, hooks and
// fsmonitor are disabled; only the allowed transports may be used
// (GIT_ALLOW_PROTOCOL, https and http by default, so `ext::`, `file://`,
// local paths and ssh are refused); repository discovery cannot climb above
// the project directory; inherited GIT_* variables are dropped; user values
// are validated and passed after `--` or `--end-of-options`.
//
// Credentials (the GitHub App installation token) are sent as an HTTP
// Authorization header through GIT_CONFIG_* environment variables scoped to
// the remote's origin, so the token is never written to .git/config and
// never appears in the process arguments. Every error message and every line
// written to a log is passed through Redact, which removes URL userinfo and
// the token itself.
package gitops

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"docklift/internal/execx"
)

// Default timeouts of one git invocation.
const (
	DefaultNetworkTimeout = 10 * time.Minute
	DefaultLocalTimeout   = 2 * time.Minute
)

// DefaultProtocols are the transports git may use.
var DefaultProtocols = []string{"https", "http"}

// Credentials authenticate HTTP(S) fetches. They are sent as a Basic
// Authorization header and never stored in the repository.
type Credentials struct {
	Username string
	Password string
}

// InstallationCredentials are the credentials of a GitHub App installation
// token (username x-access-token).
func InstallationCredentials(token string) *Credentials {
	return &Credentials{Username: "x-access-token", Password: token}
}

// Runner runs git. The zero value is ready to use.
type Runner struct {
	// Bin is the git executable (default "git", looked up in PATH).
	Bin string
	// NetworkTimeout bounds clone/fetch/ls-remote (default 10 minutes).
	NetworkTimeout time.Duration
	// LocalTimeout bounds every other command (default 2 minutes).
	LocalTimeout time.Duration
	// Protocols is the transport allow-list (default DefaultProtocols).
	// Tests add "file" to work with local repositories.
	Protocols []string
}

func (r *Runner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return "git"
}

func (r *Runner) protocols() []string {
	if len(r.Protocols) > 0 {
		return r.Protocols
	}
	return DefaultProtocols
}

// Error is a failed git invocation. Its message is git's stderr (or the
// process error when stderr is empty), already redacted, as simple-git
// reported it.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// ErrInvalidURL, ErrInvalidRef, ErrInvalidCommit and ErrNotRepository are
// returned (wrapped) for rejected inputs.
var (
	ErrInvalidURL    = errors.New("invalid repository URL")
	ErrInvalidRef    = errors.New("invalid git ref")
	ErrInvalidCommit = errors.New("Invalid commit SHA")
	ErrNotRepository = errors.New("Project path is not a git repository")
)

// ErrCredentialsRemain is VerifyNoCredentials' failure.
var ErrCredentialsRemain = errors.New("Origin remote still contains credentials after scrub")

type call struct {
	dir     string
	args    []string
	network bool
	creds   *Credentials
	// remote is the URL the credentials are scoped to.
	remote string
}

// run executes git and returns its trimmed stdout.
func (r *Runner) run(ctx context.Context, c call) (string, error) {
	timeout := r.LocalTimeout
	if timeout <= 0 {
		timeout = DefaultLocalTimeout
	}
	if c.network {
		timeout = r.NetworkTimeout
		if timeout <= 0 {
			timeout = DefaultNetworkTimeout
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hardening := []string{
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "core.fsmonitor=false",
		"-c", "credential.helper=",
		"-c", "core.askPass=",
		"-c", "protocol.allow=never",
	}
	for _, p := range r.protocols() {
		hardening = append(hardening, "-c", "protocol."+p+".allow=always")
	}
	args := append(hardening, c.args...)
	res, err := execx.Run(ctx, execx.Cmd{Name: r.bin(), Args: args, Dir: c.dir, Env: r.env(c)})
	if err != nil {
		secrets := secretsOf(c.creds)
		if ctx.Err() != nil {
			err = fmt.Errorf("%w (timeout %s)", ctx.Err(), timeout)
		}
		return "", &Error{
			Args:   redactAll(c.args, secrets),
			Stderr: strings.TrimSpace(Redact(string(res.Stderr), secrets...)),
			Err:    err,
		}
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

func secretsOf(c *Credentials) []string {
	if c == nil || c.Password == "" {
		return nil
	}
	return []string{c.Password, base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))}
}

func redactAll(args []string, secrets []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = Redact(a, secrets...)
	}
	return out
}

// env is the parent environment without any GIT_* variable, plus the
// hardening variables and, when credentials are set, the scoped
// Authorization header.
func (r *Runner) env(c call) []string {
	var env []string
	for _, kv := range os.Environ() {
		if u := strings.ToUpper(kv); !strings.HasPrefix(u, "GIT_") && !strings.HasPrefix(u, "SSH_ASKPASS=") {
			env = append(env, kv)
		}
	}
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ALLOW_PROTOCOL="+strings.Join(r.protocols(), ":"),
		"GIT_PROTOCOL_FROM_USER=0",
		"GCM_INTERACTIVE=never",
	)
	if c.dir != "" {
		// Discovery must not climb above the project into a parent repository.
		env = append(env, "GIT_CEILING_DIRECTORIES="+filepath.Dir(c.dir))
	}
	if c.creds != nil {
		if scope, ok := credentialScope(c.remote); ok {
			header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(c.creds.Username+":"+c.creds.Password))
			env = append(env,
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=http."+scope+".extraHeader",
				"GIT_CONFIG_VALUE_0="+header,
			)
		}
	}
	return env
}

// credentialScope is scheme://host[:port]/ of an http(s) remote.
func credentialScope(remote string) (string, bool) {
	u, err := url.Parse(remote)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	return u.Scheme + "://" + u.Host + "/", true
}

// ValidateURL accepts a remote URL whose scheme is in the allow-list. It
// rejects option-like values (leading `-`), transport syntax (`ext::...`),
// scp-style and local paths, whitespace and control characters, and
// http(s) URLs without a host.
func (r *Runner) ValidateURL(raw string) error {
	if raw == "" || strings.HasPrefix(raw, "-") || strings.Contains(raw, "::") {
		return fmt.Errorf("%w: %s", ErrInvalidURL, RedactURL(raw))
	}
	for _, c := range raw {
		if c <= ' ' || c == 0x7f {
			return fmt.Errorf("%w: contains whitespace or control characters", ErrInvalidURL)
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Opaque != "" {
		return fmt.Errorf("%w: %s", ErrInvalidURL, RedactURL(raw))
	}
	allowed := false
	for _, p := range r.protocols() {
		if strings.EqualFold(u.Scheme, p) {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("%w: scheme %q is not allowed", ErrInvalidURL, u.Scheme)
	}
	if (u.Scheme == "https" || u.Scheme == "http") && u.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	return nil
}

// ValidateRef accepts a branch or tag name (git check-ref-format rules) or a
// commit SHA. It rejects names starting with `-`, `..`, control characters,
// spaces and the characters `~^:?*[\`, `@{`, a lone `@`, `.lock` suffixes,
// and leading, trailing or doubled slashes and dots.
func ValidateRef(ref string) error {
	bad := func(why string) error { return fmt.Errorf("%w %q: %s", ErrInvalidRef, ref, why) }
	switch {
	case ref == "" || ref == "@":
		return bad("empty")
	case strings.HasPrefix(ref, "-"):
		return bad("starts with '-'")
	case strings.Contains(ref, ".."), strings.Contains(ref, "@{"), strings.Contains(ref, "//"):
		return bad("forbidden sequence")
	case strings.HasPrefix(ref, "/"), strings.HasSuffix(ref, "/"), strings.HasSuffix(ref, "."), strings.HasSuffix(ref, ".lock"):
		return bad("forbidden start or end")
	}
	for _, c := range ref {
		if c < 0x20 || c == 0x7f || strings.ContainsRune(" ~^:?*[\\", c) {
			return bad("forbidden character")
		}
	}
	for _, seg := range strings.Split(ref, "/") {
		if strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".lock") {
			return bad("forbidden component")
		}
	}
	return nil
}

var (
	userinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]*@`)
	shaRe      = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
	// credsRe is verifyOriginHasNoCredentials' user:password@ check.
	credsRe = regexp.MustCompile(`https?://[^/@]+:[^/@]+@`)
)

// Redact removes URL userinfo (`https://user:token@host` becomes
// `https://***@host`) and every occurrence of the given secrets from s.
func Redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "***")
		}
	}
	return userinfoRe.ReplaceAllString(s, "${1}***@")
}

// RedactURL is Redact for a single URL.
func RedactURL(u string) string { return Redact(u) }

// HasCredentials reports whether a remote URL embeds credentials: an
// x-access-token or user:password userinfo (verifyOriginHasNoCredentials).
func HasCredentials(remote string) bool {
	return strings.Contains(remote, "x-access-token") || credsRe.MatchString(remote)
}

var githubRepoRe = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/]+)`)

// GitHubOwnerRepo extracts owner and repository (without .git) from a
// GitHub URL, as the TS token-injection code did.
func GitHubOwnerRepo(u string) (owner, repo string, ok bool) {
	m := githubRepoRe.FindStringSubmatch(u)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSuffix(m[2], ".git"), true
}

func absDir(dir string) (string, error) {
	a, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	return a, nil
}

// IsRepository reports whether dir has a .git entry (fs.existsSync(dir/.git)).
func IsRepository(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// CloneOptions configure Clone.
type CloneOptions struct {
	// Ref is a branch or tag to check out (`-b`); empty clones the default branch.
	Ref string
	// Credentials, if set, authenticate the fetch without being stored.
	Credentials *Credentials
	// Out receives the progress lines of cloneRepo(…, res); nil discards them.
	Out io.Writer
}

// Clone clones remote into dest, removing dest first if it exists.
func (r *Runner) Clone(ctx context.Context, remote, dest string, opts CloneOptions) error {
	if err := r.ValidateURL(remote); err != nil {
		return err
	}
	if opts.Ref != "" {
		if err := ValidateRef(opts.Ref); err != nil {
			return err
		}
	}
	dest, err := absDir(dest)
	if err != nil {
		return err
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	secrets := secretsOf(opts.Credentials)
	fmt.Fprintf(out, "📥 Cloning repository...\n")
	fmt.Fprintf(out, "   URL: %s\n", Redact(remote, secrets...))
	if opts.Ref != "" {
		fmt.Fprintf(out, "   Ref: %s\n", opts.Ref)
	}
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("remove %s: %w", dest, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
	}
	args := []string{"clone"}
	if opts.Ref != "" {
		args = append(args, "-b", opts.Ref)
	}
	args = append(args, "--", remote, dest)
	if _, err := r.run(ctx, call{dir: filepath.Dir(dest), args: args, network: true, creds: opts.Credentials, remote: remote}); err != nil {
		return err
	}
	fmt.Fprintf(out, "✅ Repository cloned successfully\n\n")
	return nil
}

// PullOptions configure Pull.
type PullOptions struct {
	// Ref is the branch or tag to sync to; empty uses the checked-out branch
	// (or the detached commit), falling back to "main".
	Ref string
	// Credentials, if set, authenticate the fetch without being stored.
	Credentials *Credentials
}

// Pull syncs dir to the remote state of a branch or tag with fetch + hard
// reset + clean, writing the same progress lines as pullRepo to out. A ref
// that is neither a remote branch nor a tag on the remote fails closed.
// Errors are "Git sync failed: <reason>" (reason redacted).
func (r *Runner) Pull(ctx context.Context, dir string, out io.Writer, opts PullOptions) error {
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "📥 Fetching latest changes...\n")
	err := r.pull(ctx, dir, out, opts)
	if err != nil {
		msg := Redact(err.Error(), secretsOf(opts.Credentials)...)
		fmt.Fprintf(out, "   ❌ Git sync failed: %s\n\n", msg)
		return &syncError{msg: "Git sync failed: " + msg, err: err}
	}
	return nil
}

type syncError struct {
	msg string
	err error
}

func (e *syncError) Error() string { return e.msg }
func (e *syncError) Unwrap() error { return e.err }

func (r *Runner) pull(ctx context.Context, dir string, out io.Writer, opts PullOptions) error {
	dir, err := absDir(dir)
	if err != nil {
		return err
	}
	local := func(args ...string) (string, error) {
		return r.run(ctx, call{dir: dir, args: args})
	}
	remote, _ := local("remote", "get-url", "--", "origin")
	network := func(args ...string) (string, error) {
		return r.run(ctx, call{dir: dir, args: args, network: true, creds: opts.Credentials, remote: remote})
	}

	target := opts.Ref
	if target == "" {
		if b, err := local("rev-parse", "--abbrev-ref", "HEAD"); err != nil {
			target = "main"
		} else if b == "HEAD" {
			if target, err = local("rev-parse", "HEAD"); err != nil {
				target = "main"
			}
		} else {
			target = b
		}
	}
	fmt.Fprintf(out, "   Ref: %s\n", target)
	if err := ValidateRef(target); err != nil {
		return err
	}

	before, err := local("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if _, err := network("fetch", "--tags", "--prune", "--prune-tags", "--force", "--", "origin"); err != nil {
		return err
	}
	fmt.Fprintf(out, "   ✅ Fetched from origin (branches + tags)\n")

	remoteBranch := "origin/" + target
	if _, err := local("show-ref", "--verify", "--quiet", "--", "refs/remotes/"+remoteBranch); err == nil {
		if _, err := local("reset", "--hard", "refs/remotes/"+remoteBranch, "--"); err != nil {
			return err
		}
		fmt.Fprintf(out, "   ✅ Reset to %s\n", remoteBranch)
	} else {
		tagRef := "refs/tags/" + target
		listed, err := network("ls-remote", "--tags", "--refs", "--", "origin", tagRef)
		if err != nil {
			return err
		}
		if listed == "" {
			return fmt.Errorf("Ref %q not found as remote branch (%s) or tag (%s)", target, remoteBranch, tagRef)
		}
		// fetch --tags above usually suffices; the local verify below decides.
		_, _ = network("fetch", "--force", "--", "origin", "+"+tagRef+":"+tagRef)
		if _, err := local("rev-parse", "--verify", "--end-of-options", tagRef+"^{commit}"); err != nil {
			return fmt.Errorf("Ref %q exists on remote but could not be resolved locally", target)
		}
		if _, err := local("reset", "--hard", tagRef, "--"); err != nil {
			return err
		}
		fmt.Fprintf(out, "   ✅ Reset to tag %s\n", target)
	}

	if _, err := local("clean", "-f", "-d"); err != nil {
		return err
	}
	fmt.Fprintf(out, "   ✅ Cleaned untracked files\n")

	after, err := local("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if short(before) != short(after) {
		fmt.Fprintf(out, "   📝 Updated: %s → %s\n", short(before), short(after))
	} else {
		fmt.Fprintf(out, "   Already up to date (%s)\n", short(after))
	}
	fmt.Fprintf(out, "\n")
	return nil
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// CurrentBranch is `git rev-parse --abbrev-ref HEAD` ("HEAD" when
// detached); ok is false when dir is not a repository.
func (r *Runner) CurrentBranch(ctx context.Context, dir string) (string, bool) {
	dir, err := absDir(dir)
	if err != nil {
		return "", false
	}
	b, err := r.run(ctx, call{dir: dir, args: []string{"rev-parse", "--abbrev-ref", "HEAD"}})
	if err != nil {
		return "", false
	}
	return b, true
}

// LastCommitMessage is the subject of the HEAD commit; ok is false without
// a checkout or commit, or when the subject is empty.
func (r *Runner) LastCommitMessage(ctx context.Context, dir string) (string, bool) {
	if !IsRepository(dir) {
		return "", false
	}
	dir, err := absDir(dir)
	if err != nil {
		return "", false
	}
	msg, err := r.run(ctx, call{dir: dir, args: []string{"log", "-1", "--format=%s"}})
	if err != nil || msg == "" {
		return "", false
	}
	return msg, true
}

// CommitSHA is the full SHA of HEAD; ok is false without a checkout.
func (r *Runner) CommitSHA(ctx context.Context, dir string) (string, bool) {
	if !IsRepository(dir) {
		return "", false
	}
	dir, err := absDir(dir)
	if err != nil {
		return "", false
	}
	sha, err := r.run(ctx, call{dir: dir, args: []string{"rev-parse", "HEAD"}})
	if err != nil || sha == "" {
		return "", false
	}
	return sha, true
}

// ResetToCommit hard-resets the working tree to a known commit (rollback).
// sha must be 7–40 hex digits.
func (r *Runner) ResetToCommit(ctx context.Context, dir, sha string) error {
	if !shaRe.MatchString(sha) {
		return ErrInvalidCommit
	}
	if !IsRepository(dir) {
		return ErrNotRepository
	}
	dir, err := absDir(dir)
	if err != nil {
		return err
	}
	_, err = r.run(ctx, call{dir: dir, args: []string{"reset", "--hard", sha, "--"}})
	return err
}

// OriginURL is the fetch URL of the origin remote. Callers must Redact it
// before logging.
func (r *Runner) OriginURL(ctx context.Context, dir string) (string, error) {
	dir, err := absDir(dir)
	if err != nil {
		return "", err
	}
	return r.run(ctx, call{dir: dir, args: []string{"remote", "get-url", "--", "origin"}})
}

// SetOriginURL sets the origin URL (`git remote set-url -- origin <u>`).
func (r *Runner) SetOriginURL(ctx context.Context, dir, u string) error {
	dir, err := absDir(dir)
	if err != nil {
		return err
	}
	_, err = r.run(ctx, call{dir: dir, args: []string{"remote", "set-url", "--", "origin", u}})
	return err
}

// ScrubOrigin removes credentials from the origin remote by setting it to
// cleanURL (scrubOriginRemote). It does nothing when cleanURL is empty or
// dir is not a checkout.
func (r *Runner) ScrubOrigin(ctx context.Context, dir, cleanURL string) error {
	if cleanURL == "" || !IsRepository(dir) {
		return nil
	}
	return r.SetOriginURL(ctx, dir, cleanURL)
}

// VerifyNoCredentials fails with ErrCredentialsRemain when the origin URL
// still embeds credentials (verifyOriginHasNoCredentials). A missing origin
// counts as clean, as in TS.
func (r *Runner) VerifyNoCredentials(ctx context.Context, dir string) error {
	u, err := r.OriginURL(ctx, dir)
	if err != nil {
		var gerr *Error
		if errors.As(err, &gerr) && gerr.Stderr != "" && strings.Contains(gerr.Stderr, "No such remote") {
			return nil
		}
		return err
	}
	if HasCredentials(u) {
		return ErrCredentialsRemain
	}
	return nil
}
