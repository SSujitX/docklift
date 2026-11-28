package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"docklift/internal/js"
)

// Docker Hub catalog limits.
const (
	HubTTL           = 6 * time.Hour
	HubIncompleteTTL = 5 * time.Minute
	hubTimeout       = 10 * time.Second
	hubMaxPages      = 3
	hubPageSize      = 100
	hubMaxVersions   = 36
)

// HubTags fetches live Docker Hub tags for managed database images, with a
// per-engine TTL cache; it falls back to the static versions when Hub is
// unreachable. It is safe for concurrent use.
type HubTags struct {
	// BaseURL is the Docker Hub API origin (tests point it at a fake).
	BaseURL string
	Client  *http.Client
	Log     *slog.Logger
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]hubCacheEntry
}

type hubCacheEntry struct {
	at       time.Time
	ttl      time.Duration
	versions []EngineVersion
}

// NewHubTags returns a catalog using client (nil = http.DefaultClient).
func NewHubTags(client *http.Client, log *slog.Logger) *HubTags {
	if client == nil {
		client = http.DefaultClient
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &HubTags{BaseURL: "https://hub.docker.com", Client: client, Log: log, now: time.Now, cache: map[string]hubCacheEntry{}}
}

// ClearCache drops every cached catalog.
func (h *HubTags) ClearCache() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache = map[string]hubCacheEntry{}
}

func libraryRepo(imageRepo string) string {
	if strings.Contains(imageRepo, "/") {
		return imageRepo
	}
	return "library/" + imageRepo
}

// fetchTagNames reads up to hubMaxPages pages of tag names; complete is
// false when a page failed or more pages remained.
func (h *HubTags) fetchTagNames(ctx context.Context, imageRepo string) (names []string, complete bool) {
	repo := libraryRepo(imageRepo)
	for page := 1; page <= hubMaxPages; page++ {
		url := fmt.Sprintf("%s/v2/repositories/%s/tags?page_size=%d&page=%d", h.BaseURL, repo, hubPageSize, page)
		batch, next, ok := h.fetchPage(ctx, url)
		if !ok {
			return names, false
		}
		if len(batch) == 0 {
			return names, true
		}
		names = append(names, batch...)
		if !next {
			return names, true
		}
	}
	return names, false
}

func (h *HubTags) fetchPage(ctx context.Context, url string) (names []string, hasNext, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, hubTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, false
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, false, false
	}
	var data struct {
		Results []struct {
			Name any `json:"name"`
		} `json:"results"`
		Next any `json:"next"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&data); err != nil {
		return nil, false, false
	}
	for _, r := range data.Results {
		if s, ok := r.Name.(string); ok && s != "" {
			names = append(names, s)
		}
	}
	return names, js.Truthy(data.Next), true
}

var (
	preReleaseRe = regexp.MustCompile(`(?i)alpha|beta|rc|preview|nightly|dev`)
	tagNoiseRe   = regexp.MustCompile(`(?i)(alpine\d|bookworm|bullseye|trixie|slim|window|nanoserver|ubi|oraclelinux)`)
	pgRedisTagRe = regexp.MustCompile(`^\d+(\.\d+)?(-alpine)?$`)
	mysqlTagRe   = regexp.MustCompile(`^\d+\.\d+(-oracle)?$`)
	majorMinorRe = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

// MatchesEngineTagPattern reports whether tag looks like a publishable,
// operator-friendly Hub tag for the engine (no pre-releases, no distro or
// arch variants).
func MatchesEngineTagPattern(engineID, tag string) bool {
	if tag == "" || preReleaseRe.MatchString(tag) {
		return false
	}
	if tag == "latest" {
		return true
	}
	if tagNoiseRe.MatchString(tag) {
		return false
	}
	switch engineID {
	case EnginePostgres, EngineRedis:
		return pgRedisTagRe.MatchString(tag)
	case EngineMySQL:
		return mysqlTagRe.MatchString(tag)
	case EngineMariaDB, EngineMongoDB:
		return majorMinorRe.MatchString(tag)
	}
	return false
}

type versionParts struct {
	major, minor float64
	alpine       bool
	latest       bool
}

// jsNumberOr0 is Number(s) || 0 for the digit strings of a tag.
func jsNumberOr0(s string) float64 {
	f, err := strconv.ParseFloat(js.Trim(s), 64)
	if err != nil || f != f {
		return 0
	}
	return f
}

func parseVersionParts(tag string) versionParts {
	if tag == "latest" {
		return versionParts{major: -1, minor: -1, latest: true}
	}
	alpine := strings.HasSuffix(tag, "-alpine")
	core := strings.TrimSuffix(strings.TrimSuffix(tag, "-alpine"), "-oracle")
	parts := strings.Split(core, ".")
	p := versionParts{major: jsNumberOr0(parts[0]), minor: -1, alpine: alpine}
	if len(parts) > 1 {
		p.minor = jsNumberOr0(parts[1])
	}
	return p
}

func labelForTag(tag string) string {
	switch {
	case tag == "latest":
		return "latest"
	case strings.HasSuffix(tag, "-alpine"):
		return strings.TrimSuffix(tag, "-alpine") + " · Alpine"
	case strings.HasSuffix(tag, "-oracle"):
		return strings.TrimSuffix(tag, "-oracle") + " · Oracle Linux"
	}
	return tag
}

// compareTags orders newest major/minor first, alpine before debian for the
// same version, "latest" last.
func compareTags(a, b string) int {
	pa, pb := parseVersionParts(a), parseVersionParts(b)
	if pa.latest != pb.latest {
		if pa.latest {
			return 1
		}
		return -1
	}
	if pa.major != pb.major {
		return sign(pb.major - pa.major)
	}
	if pa.minor != pb.minor {
		return sign(pb.minor - pa.minor)
	}
	if pa.alpine != pb.alpine {
		if pa.alpine {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func sign(f float64) int {
	switch {
	case f < 0:
		return -1
	case f > 0:
		return 1
	}
	return 0
}

func majorKey(tag string, p versionParts) string {
	suffix := ""
	if strings.Contains(tag, "-oracle") {
		suffix = "-oracle"
	} else if p.alpine {
		suffix = "-alpine"
	}
	return js.NumberString(p.major) + suffix
}

// preferMajorLines drops minor/patch tags (16.14) when the major line tag
// (16, 16-alpine) exists, keeping minors when no bare major is published.
func preferMajorLines(tags []string) []string {
	set := map[string]bool{}
	for _, t := range tags {
		set[t] = true
	}
	majors := map[string]bool{}
	for _, t := range tags {
		if t == "latest" {
			continue
		}
		if k := majorKey(t, parseVersionParts(t)); set[k] {
			majors[k] = true
		}
	}
	var out []string
	for _, t := range tags {
		if t == "latest" {
			out = append(out, t)
			continue
		}
		p := parseVersionParts(t)
		if p.minor < 0 || !majors[majorKey(t, p)] {
			out = append(out, t)
		}
	}
	return out
}

// FilterHubTagsToVersions turns raw Hub tag names into the version list of
// the New Database UI: allowed tags, major lines preferred, sorted, at most
// 36, with recommendedTag (when present) or the newest alpine/non-latest
// tag marked recommended.
func FilterHubTagsToVersions(engineID string, tagNames []string, recommendedTag string) []EngineVersion {
	seen := map[string]bool{}
	var allowed []string
	for _, t := range tagNames {
		if MatchesEngineTagPattern(engineID, t) && !seen[t] {
			seen[t] = true
			allowed = append(allowed, t)
		}
	}
	trimmed := preferMajorLines(allowed)
	slices.SortStableFunc(trimmed, compareTags)
	trimmed = trimmed[:min(hubMaxVersions, len(trimmed))]

	recommended := ""
	if recommendedTag != "" && slices.Contains(trimmed, recommendedTag) {
		recommended = recommendedTag
	}
	if recommended == "" {
		for _, t := range trimmed {
			if strings.HasSuffix(t, "-alpine") {
				recommended = t
				break
			}
		}
	}
	if recommended == "" {
		for _, t := range trimmed {
			if t != "latest" {
				recommended = t
				break
			}
		}
	}
	if recommended == "" && len(trimmed) > 0 {
		recommended = trimmed[0]
	}
	out := make([]EngineVersion, 0, len(trimmed))
	for _, t := range trimmed {
		out = append(out, EngineVersion{Tag: t, Label: labelForTag(t), Recommended: t == recommended})
	}
	return out
}

// LiveVersions returns the engine's versions from Docker Hub (cached 6 h,
// or 5 min when Hub returned an incomplete page set), or fallback when Hub
// gives nothing usable (fetchLiveEngineVersions).
func (h *HubTags) LiveVersions(ctx context.Context, engineID, imageRepo string, fallback []EngineVersion) []EngineVersion {
	h.mu.Lock()
	if c, ok := h.cache[engineID]; ok && h.now().Sub(c.at) < c.ttl {
		h.mu.Unlock()
		return slices.Clone(c.versions)
	}
	h.mu.Unlock()

	recommended := ""
	for _, v := range fallback {
		if v.Recommended {
			recommended = v.Tag
			break
		}
	}
	if recommended == "" && len(fallback) > 0 {
		recommended = fallback[0].Tag
	}
	names, complete := h.fetchTagNames(ctx, imageRepo)
	if len(names) == 0 {
		return fallback
	}
	versions := FilterHubTagsToVersions(engineID, names, recommended)
	if len(versions) == 0 {
		return fallback
	}
	ttl := HubTTL
	if !complete {
		ttl = HubIncompleteTTL
		h.Log.Warn("[dockerHubTags] incomplete Hub page set — short-TTL cache", "engine", engineID)
	}
	h.mu.Lock()
	h.cache[engineID] = hubCacheEntry{at: h.now(), ttl: ttl, versions: slices.Clone(versions)}
	h.mu.Unlock()
	return versions
}

// ListEnginesWithLiveVersions is the catalog with versions refreshed from
// Docker Hub, engines fetched concurrently; each engine's Image follows its
// live recommended version (listDatabaseEnginesWithLiveVersions).
func (h *HubTags) ListEnginesWithLiveVersions(ctx context.Context) []Engine {
	out := ListEngines()
	var wg sync.WaitGroup
	for i := range out {
		wg.Go(func() {
			e := &out[i]
			e.Versions = h.LiveVersions(ctx, e.ID, e.ImageRepo, e.Versions)
			if len(e.Versions) > 0 {
				rec := e.Versions[0]
				for _, v := range e.Versions {
					if v.Recommended {
						rec = v
						break
					}
				}
				e.Image = e.ImageRepo + ":" + rec.Tag
			}
		})
	}
	wg.Wait()
	return out
}
