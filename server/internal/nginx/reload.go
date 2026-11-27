package nginx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"docklift/internal/certs"
)

// execTimeout bounds one `docker exec ... nginx` call.
const execTimeout = 30 * time.Second

// ReloadError is a failed nginx reload or configuration test. Its message
// keeps the TypeScript prefix "Nginx reload failed: " (the API returns it).
type ReloadError struct {
	// Detail is the spawn error, nginx's stderr, or "exit code N".
	Detail string
	// RolledBack is true when a conf write failed `nginx -t` and the
	// previous file content was restored.
	RolledBack bool
}

func (e *ReloadError) Error() string { return "Nginx reload failed: " + e.Detail }

// IsReloadError reports whether err is (or wraps) a ReloadError.
func IsReloadError(err error) bool {
	var re *ReloadError
	return errors.As(err, &re)
}

// selfHealPattern finds a service conf nginx refuses because its upstream
// host does not resolve (old-style confs without a resolver variable).
var selfHealPattern = regexp.MustCompile(`host not found in upstream .* in (.*/service-[a-zA-Z0-9-]+\.conf):`)

// Reloader runs nginx inside the nginx-proxy container.
type Reloader struct {
	runner    certs.Runner
	container string
	confDir   string
	log       *slog.Logger
}

func (r *Reloader) exec(ctx context.Context, args ...string) (certs.ExecResult, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	return r.runner.Run(ctx, "docker", append([]string{"exec", r.container, "nginx"}, args...)...)
}

// attemptSelfHeal removes the local service conf named in a "host not found
// in upstream" error. It returns the removed file's name.
func (r *Reloader) attemptSelfHeal(stderr string) string {
	m := selfHealPattern.FindStringSubmatch(stderr)
	if m == nil || m[1] == "" {
		return ""
	}
	name := m[1][strings.LastIndex(m[1], "/")+1:]
	local := filepath.Join(r.confDir, name)
	if _, err := os.Stat(local); err != nil {
		return ""
	}
	r.log.Info("Self-Healing: Removing bad Nginx config causing crash: " + name)
	if err := os.Remove(local); err != nil {
		r.log.Error("Failed to remove bad config", "error", err)
		return ""
	}
	return name
}

type reloadAttempt struct {
	ok     bool
	detail string
	stderr string
}

func (r *Reloader) reloadOnce(ctx context.Context) reloadAttempt {
	r.log.Info("Reloading Nginx proxy...")
	res, err := r.exec(ctx, "-s", "reload")
	if err != nil {
		r.log.Error("Failed to spawn docker exec", "error", err)
		return reloadAttempt{detail: err.Error(), stderr: res.Stderr}
	}
	if res.ExitCode == 0 {
		r.log.Info("Nginx proxy reloaded successfully")
		return reloadAttempt{ok: true}
	}
	r.log.Error(fmt.Sprintf("Nginx reload failed with code %d", res.ExitCode), "stderr", res.Stderr)
	detail := strings.TrimSpace(res.Stderr)
	if detail == "" {
		detail = "exit code " + strconv.Itoa(res.ExitCode)
	}
	return reloadAttempt{detail: detail, stderr: res.Stderr}
}

// Reload runs `nginx -s reload`. On failure it removes a service conf whose
// upstream host is unknown (self-heal) and tries once more; a second failure
// is returned as *ReloadError.
func (r *Reloader) Reload(ctx context.Context) error {
	first := r.reloadOnce(ctx)
	if first.ok {
		return nil
	}
	r.attemptSelfHeal(first.stderr)
	second := r.reloadOnce(ctx)
	if second.ok {
		return nil
	}
	return &ReloadError{Detail: second.detail}
}

// testVerdict is the outcome of `nginx -t`.
type testVerdict int

const (
	// testPassed: nginx accepted the configuration.
	testPassed testVerdict = iota
	// testInvalid: nginx ran and rejected the configuration.
	testInvalid
	// testUnavailable: nginx could not be run (no Docker, container down);
	// nothing is known about the configuration.
	testUnavailable
)

// Test runs `nginx -t` and classifies the result. Only output that nginx
// itself produced ("... test failed") counts as an invalid configuration; a
// docker error (daemon down, no such container) is testUnavailable.
func (r *Reloader) Test(ctx context.Context) (testVerdict, string) {
	res, err := r.exec(ctx, "-t")
	if err != nil {
		return testUnavailable, err.Error()
	}
	if res.ExitCode == 0 {
		return testPassed, ""
	}
	out := res.Stderr + "\n" + res.Stdout
	if strings.Contains(out, "test failed") || strings.Contains(out, "[emerg]") {
		return testInvalid, strings.TrimSpace(res.Stderr)
	}
	return testUnavailable, strings.TrimSpace(res.Stderr)
}

// ApplyConf writes content to path atomically and runs `nginx -t`. If nginx
// rejects the configuration — after one self-heal attempt for an unrelated
// broken service conf — the previous content is restored (or the new file
// removed) and a *ReloadError with RolledBack is returned. When nginx cannot
// be run at all the new file is kept, as the TypeScript backend did; the
// following Reload reports that failure. Reloading is left to the caller.
func (r *Reloader) ApplyConf(ctx context.Context, path, content string) error {
	prev, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if err := certs.WriteFileAtomic(path, []byte(content), 0o644); err != nil {
		return err
	}
	verdict, detail := r.Test(ctx)
	if verdict == testInvalid {
		if healed := r.attemptSelfHeal(detail); healed != "" && healed != filepath.Base(path) {
			verdict, detail = r.Test(ctx)
		}
	}
	if verdict != testInvalid {
		return nil
	}
	var restoreErr error
	if existed {
		restoreErr = certs.WriteFileAtomic(path, prev, 0o644)
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		restoreErr = err
	}
	if restoreErr != nil {
		return errors.Join(&ReloadError{Detail: detail}, fmt.Errorf("restore %s: %w", filepath.Base(path), restoreErr))
	}
	r.log.Warn("nginx rejected the generated config — previous version restored", "file", filepath.Base(path), "detail", detail)
	return &ReloadError{Detail: detail, RolledBack: true}
}
