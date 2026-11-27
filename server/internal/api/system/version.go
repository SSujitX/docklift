package system

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"docklift/internal/sysinfo"
)

// GitHubLatestReleaseURL is the release probe of GET /version.
const GitHubLatestReleaseURL = "https://api.github.com/repos/SSujitX/docklift/releases/latest"

// Cache lifetimes of the version probe: fresh releases must show up quickly,
// so the TTL is short when no update is known or GitHub failed.
const (
	versionTTLUpdate          = 15 * time.Minute
	versionTTLCurrent         = 2 * time.Minute
	versionTTLGitHubFail      = 30 * time.Second
	versionRefreshMinInterval = 30 * time.Second
	versionProbeTimeout       = 15 * time.Second
)

// VersionInfo is the /api/system/version response.
type VersionInfo struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	// GitHubOK is false when GitHub could not be reached: not "already latest".
	GitHubOK  bool   `json:"githubOk"`
	CheckedAt string `json:"checkedAt"`
}

// versionChecker caches the GitHub probe; concurrent callers share one
// request in flight.
type versionChecker struct {
	current string
	url     string
	http    *http.Client
	log     *slog.Logger
	now     func() time.Time

	mu        sync.Mutex
	cached    *VersionInfo
	lastCheck time.Time
	ttl       time.Duration
	inFlight  chan struct{}
}

// IsNewerVersion compares the first three dot-separated numbers (parseInt
// semantics; missing or invalid parts count as 0).
func IsNewerVersion(latest, current string) bool {
	parts := func(v string) [3]float64 {
		var out [3]float64
		for i, p := range strings.Split(v, ".") {
			if i == 3 {
				break
			}
			if n := sysinfo.ParseInt(p); !math.IsNaN(n) {
				out[i] = n
			}
		}
		return out
	}
	l, c := parts(latest), parts(current)
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

// Get returns the cached info while fresh (a forced refresh is honoured at
// most every 30 seconds), else probes GitHub.
func (v *versionChecker) Get(ctx context.Context, forceRefresh bool) VersionInfo {
	v.mu.Lock()
	now := v.now()
	fresh := v.cached != nil && v.ttl > 0 && now.Sub(v.lastCheck) < v.ttl
	cooldownOK := now.Sub(v.lastCheck) >= versionRefreshMinInterval
	if fresh && !(forceRefresh && cooldownOK) {
		info := *v.cached
		v.mu.Unlock()
		return info
	}
	done := v.inFlight
	if done == nil {
		done = make(chan struct{})
		v.inFlight = done
		go v.probe(done)
	}
	v.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cached == nil {
		return VersionInfo{Current: v.current, Latest: v.current, CheckedAt: sysinfo.ISOString(now)}
	}
	return *v.cached
}

func (v *versionChecker) probe(done chan struct{}) {
	now := v.now()
	latest, ok := v.fetchLatest()
	update := ok && IsNewerVersion(latest, v.current)
	info := &VersionInfo{Current: v.current, Latest: latest, UpdateAvailable: update, GitHubOK: ok, CheckedAt: sysinfo.ISOString(now)}
	ttl := versionTTLCurrent
	switch {
	case !ok:
		ttl = versionTTLGitHubFail
	case update:
		ttl = versionTTLUpdate
	}
	v.mu.Lock()
	v.cached, v.lastCheck, v.ttl, v.inFlight = info, now, ttl, nil
	v.mu.Unlock()
	close(done)
}

// fetchLatest returns the latest release tag without its "v" prefix, or the
// current version and false when GitHub did not answer usefully.
func (v *versionChecker) fetchLatest() (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return v.current, false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Docklift")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := v.http.Do(req)
	if err != nil {
		v.log.Warn("[version] GitHub releases/latest error", "error", err)
		return v.current, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		v.log.Warn("[version] GitHub releases/latest failed: " + resp.Status)
		return v.current, false
	}
	var body struct {
		TagName any `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		v.log.Warn("[version] GitHub releases/latest error", "error", err)
		return v.current, false
	}
	switch tag := body.TagName.(type) {
	case nil:
		return v.current, true
	case string:
		if latest := strings.TrimPrefix(tag, "v"); latest != "" {
			return latest, true
		}
		return v.current, true
	default:
		// tag_name?.replace threw on a non-string in TypeScript.
		v.log.Warn("[version] GitHub releases/latest error", "error", "tag_name is not a string")
		return v.current, false
	}
}
