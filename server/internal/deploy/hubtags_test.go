package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func versionTags(vs []EngineVersion) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Tag
	}
	return out
}

func recommendedOf(vs []EngineVersion) string {
	for _, v := range vs {
		if v.Recommended {
			return v.Tag
		}
	}
	return ""
}

func TestFilterHubTagsToVersions(t *testing.T) {
	// dockerHubTags.test.ts
	versions := FilterHubTagsToVersions("postgres", []string{
		"latest", "18.4-trixie", "18.4", "18", "18-alpine", "18.4-alpine", "17", "17-alpine",
		"16-bookworm", "16-alpine", "19beta2-alpine", "15-alpine",
	}, "16-alpine")
	tags := versionTags(versions)
	for _, want := range []string{"18-alpine", "18", "17-alpine", "latest"} {
		if !slices.Contains(tags, want) {
			t.Errorf("missing %s in %v", want, tags)
		}
	}
	for _, not := range []string{"18.4-trixie", "16-bookworm", "19beta2-alpine", "18.4", "18.4-alpine"} {
		if slices.Contains(tags, not) {
			t.Errorf("unexpected %s in %v", not, tags)
		}
	}
	if recommendedOf(versions) != "16-alpine" {
		t.Errorf("recommended %s", recommendedOf(versions))
	}
	if !reflect.DeepEqual(tags, []string{"18-alpine", "18", "17-alpine", "17", "16-alpine", "15-alpine", "latest"}) {
		t.Errorf("order %v", tags)
	}

	versions = FilterHubTagsToVersions("postgres", []string{"18-alpine", "17-alpine", "18", "17"}, "99-alpine")
	if versions[0].Tag != "18-alpine" || recommendedOf(versions) != "18-alpine" || versions[0].Label != "18 · Alpine" {
		t.Errorf("newest alpine recommended: %+v", versions)
	}
	if v := FilterHubTagsToVersions("mysql", []string{"latest", "8.4-oracle", "8.4", "8.0"}, ""); recommendedOf(v) != "8.4" || v[1].Label != "8.4 · Oracle Linux" {
		t.Errorf("mysql %+v", v)
	}
	if v := FilterHubTagsToVersions("mongodb", []string{"latest"}, ""); recommendedOf(v) != "latest" {
		t.Errorf("latest only %+v", v)
	}
	if v := FilterHubTagsToVersions("mongodb", nil, ""); len(v) != 0 {
		t.Errorf("empty %+v", v)
	}
	many := make([]string, 50)
	for i := range many {
		many[i] = fmt.Sprintf("%d.%d", i/5, i%5)
	}
	if v := FilterHubTagsToVersions("mariadb", many, ""); len(v) != 36 || v[0].Tag != "9.4" {
		t.Errorf("cap %d first %+v", len(v), v[0])
	}
}

func TestMatchesEngineTagPattern(t *testing.T) {
	for _, c := range []struct {
		engine, tag string
		want        bool
	}{
		{"postgres", "18-alpine", true}, {"postgres", "18.4-bookworm", false}, {"mysql", "8.4", true}, {"mongodb", "8", true},
		{"mysql", "8", false}, {"mysql", "8.4-oracle", true}, {"redis", "7.2-alpine", true}, {"mariadb", "11.4", true},
		{"postgres", "", false}, {"postgres", "latest", true}, {"postgres", "16rc1", false}, {"postgres", "dev", false},
		{"redis", "7-alpine3.20", false}, {"mongodb", "windowsservercore", false}, {"sqlite", "3", false},
	} {
		if got := MatchesEngineTagPattern(c.engine, c.tag); got != c.want {
			t.Errorf("%s %q -> %v", c.engine, c.tag, got)
		}
	}
}

const hubScript = nodePrelude + `
const h = await load('lib/dockerHubTags.js');
done(input.map((c) => ({
  versions: h.filterHubTagsToVersions(c.engine, c.tags, c.rec || undefined),
  matches: c.tags.map((t) => h.matchesEngineTagPattern(c.engine, t)),
})));
`

// TestHubTagsMatchTypeScript feeds a realistic tag soup through the TS and
// Go filters.
func TestHubTagsMatchTypeScript(t *testing.T) {
	soup := []string{"latest", "18", "18-alpine", "18.4", "18.4-alpine", "18-alpine3.22", "18.4-bookworm", "18-trixie", "17", "17.6",
		"17-alpine", "17.6-alpine", "16", "16.10", "16-alpine", "15", "15-alpine", "14", "13", "12", "11", "10", "9.6", "9.6-alpine",
		"19beta3", "19rc1-alpine", "8.4", "8.4.6", "8.4-oracle", "8.0", "8.0-oracle", "8.0.43", "9.4", "9", "11.4", "10.11", "10.6", "11.8.3",
		"8-alpine", "7-alpine", "7.4", "7.4-alpine", "7", "6-alpine", "6.2", "8.0-rc", "8.2-windowsservercore", "7.0-nanoserver",
		"7.0-jammy", "8.0", "7.0", "6.0", "5.0", "4.4", "8", "6", "5", "4", "08", "8.04", "nightly", "dev-alpine", "18.0", "18.00"}
	type in struct {
		Engine string   `json:"engine"`
		Tags   []string `json:"tags"`
		Rec    string   `json:"rec"`
	}
	type out struct {
		Versions []EngineVersion `json:"versions"`
		Matches  []bool          `json:"matches"`
	}
	var cases []in
	for _, id := range EngineIDs {
		cases = append(cases, in{Engine: id, Tags: soup}, in{Engine: id, Tags: soup, Rec: "7-alpine"}, in{Engine: id, Tags: soup[30:], Rec: "nope"})
	}
	var want []out
	decodeJSON(t, runNode(t, hubScript, cases), &want)
	for i, c := range cases {
		got := out{Versions: FilterHubTagsToVersions(c.Engine, c.Tags, c.Rec)}
		for _, tag := range c.Tags {
			got.Matches = append(got.Matches, MatchesEngineTagPattern(c.Engine, tag))
		}
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("%s rec=%q\n got  %v\n want %v", c.Engine, c.Rec, got, want[i])
		}
	}
}

// fakeHub serves /v2/repositories/<repo>/tags pages.
func fakeHub(t *testing.T, pages map[string][][]string, status *atomic.Int32, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if code := status.Load(); code != 0 {
			w.WriteHeader(int(code))
			return
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/repositories/"), "/tags")
		if r.URL.Query().Get("page_size") != "100" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("request %s %v", r.URL, r.Header)
		}
		var page int
		fmt.Sscan(r.URL.Query().Get("page"), &page)
		ps := pages[repo]
		var body struct {
			Results []map[string]any `json:"results"`
			Next    any              `json:"next"`
		}
		if page-1 < len(ps) {
			for _, n := range ps[page-1] {
				body.Results = append(body.Results, map[string]any{"name": n})
			}
		}
		if page < len(ps) {
			body.Next = "more"
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHubLiveVersions(t *testing.T) {
	var status, hits atomic.Int32
	srv := fakeHub(t, map[string][][]string{
		"library/postgres": {{"18-alpine", "18"}, {"17-alpine", "17"}},
		"library/redis":    {{"8-alpine"}, {"7-alpine"}, {"7"}, {"6-alpine"}},
		"library/mysql":    {{"weird-tag"}},
	}, &status, &hits)
	h := NewHubTags(srv.Client(), nil)
	h.BaseURL = srv.URL
	now := time.Unix(1_700_000_000, 0)
	h.now = func() time.Time { return now }
	ctx := context.Background()
	pg := mustEngine(t, EnginePostgres)

	got := h.LiveVersions(ctx, EnginePostgres, "postgres", pg.Versions)
	if !reflect.DeepEqual(versionTags(got), []string{"18-alpine", "18", "17-alpine", "17"}) || recommendedOf(got) != "18-alpine" || hits.Load() != 2 {
		t.Fatalf("versions %v hits %d", versionTags(got), hits.Load())
	}
	// Cached for 6 h.
	now = now.Add(5 * time.Hour)
	h.LiveVersions(ctx, EnginePostgres, "postgres", pg.Versions)
	if hits.Load() != 2 {
		t.Fatal("complete catalog must be cached")
	}
	now = now.Add(2 * time.Hour)
	h.LiveVersions(ctx, EnginePostgres, "postgres", pg.Versions)
	if hits.Load() != 4 {
		t.Fatalf("expired cache must refetch, hits %d", hits.Load())
	}

	// More than 3 pages: usable but incomplete, cached only 5 minutes.
	redis := mustEngine(t, EngineRedis)
	hits.Store(0)
	got = h.LiveVersions(ctx, EngineRedis, "redis", redis.Versions)
	if !reflect.DeepEqual(versionTags(got), []string{"8-alpine", "7-alpine", "7"}) || hits.Load() != 3 {
		t.Fatalf("redis %v hits %d", versionTags(got), hits.Load())
	}
	now = now.Add(6 * time.Minute)
	h.LiveVersions(ctx, EngineRedis, "redis", redis.Versions)
	if hits.Load() != 6 {
		t.Fatalf("incomplete catalog TTL, hits %d", hits.Load())
	}

	// Nothing usable: static fallback, not cached.
	mysql := mustEngine(t, EngineMySQL)
	if got = h.LiveVersions(ctx, EngineMySQL, "mysql", mysql.Versions); !reflect.DeepEqual(got, mysql.Versions) {
		t.Fatalf("fallback %v", got)
	}
	status.Store(503)
	h.ClearCache()
	if got = h.LiveVersions(ctx, EnginePostgres, "postgres", pg.Versions); !reflect.DeepEqual(got, pg.Versions) {
		t.Fatalf("Hub down must fall back: %v", got)
	}

	list := h.ListEnginesWithLiveVersions(ctx)
	if len(list) != 5 || list[0].ID != EnginePostgres || list[0].Image != "postgres:18-alpine" {
		t.Fatalf("list %+v", list[0])
	}
	status.Store(0)
	h.ClearCache()
	list = h.ListEnginesWithLiveVersions(ctx)
	if list[3].ID != EngineRedis || len(list[3].Versions) != 3 || list[3].Image != "redis:8-alpine" {
		t.Fatalf("live list %+v", list[3])
	}
}
