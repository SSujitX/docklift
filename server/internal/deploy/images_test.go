package deploy

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"docklift/internal/db"
	"docklift/internal/dockerx"
	"docklift/internal/store"
)

func tags(kv ...string) ImageTags {
	var m ImageTags
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i], kv[i+1])
	}
	return m
}

func rawJSON(s string) *db.JSON {
	if s == "" {
		return nil
	}
	j := db.JSON(s)
	return &j
}

func TestPickTaggedImageMaps(t *testing.T) {
	// imageCleanup.test.ts
	a, b, c := `{"api":"docklift-abcdef12-api:aaaaaaaa"}`, `{"api":"docklift-abcdef12-api:bbbbbbbb"}`, `{"api":"docklift-abcdef12-api:cccccccc"}`
	rows := []*db.JSON{rawJSON(b), nil, rawJSON("null"), rawJSON(a)}
	maps := PickTaggedImageMaps(rows, 2)
	if len(maps) != 2 || maps[0].Values()[0] != "docklift-abcdef12-api:bbbbbbbb" || maps[1].Values()[0] != "docklift-abcdef12-api:aaaaaaaa" {
		t.Fatalf("stop/restart rows must not use a keep slot: %+v", maps)
	}
	keep := BuildKeepImageSet(maps, nil)
	if !keep["docklift-abcdef12-api:aaaaaaaa"] || !keep["docklift-abcdef12-api:bbbbbbbb"] {
		t.Fatal("rollback target evicted")
	}
	maps = PickTaggedImageMaps([]*db.JSON{rawJSON(c), rawJSON(b), rawJSON(a)}, 2)
	if len(maps) != 2 || maps[0].Values()[0] != "docklift-abcdef12-api:cccccccc" || maps[1].Values()[0] != "docklift-abcdef12-api:bbbbbbbb" {
		t.Fatalf("newest two: %+v", maps)
	}
	if len(PickTaggedImageMaps(nil, 2)) != 0 || len(PickTaggedImageMaps([]*db.JSON{nil, rawJSON("{}")}, 2)) != 0 {
		t.Fatal("empty and untagged-only history")
	}
}

func TestIsDockliftAppImage(t *testing.T) {
	for ref, want := range map[string]bool{
		"docklift-abcdef12-api:deadbeef":     true,
		"docklift-abcdef12-web-app:12345678": true,
		"docklift-ABCDEF12-api":              true,
		"postgres:16":                        false,
		"nginx:alpine":                       false,
		"docklift-backend:latest":            false,
		"docklift-abcdef1-api:tag":           false,
		"docklift-abcdef12--x:1":             false,
	} {
		if got := IsDockliftAppImage(ref); got != want {
			t.Errorf("IsDockliftAppImage(%q) = %v", ref, got)
		}
	}
}

func TestBuildKeepImageSet(t *testing.T) {
	keep := BuildKeepImageSet([]ImageTags{
		tags("api", "docklift-abcdef12-api:11111111", "web", "docklift-abcdef12-web:11111111"),
		tags("api", "docklift-abcdef12-api:22222222", "web", "docklift-abcdef12-web:22222222"),
	}, nil)
	if len(keep) != 4 || !keep["docklift-abcdef12-api:22222222"] {
		t.Fatalf("keep %v", keep)
	}
	keep = BuildKeepImageSet([]ImageTags{tags("api", "docklift-abcdef12-api:aaaaaaaa", "db", "postgres:16")}, nil)
	if !reflect.DeepEqual(sortedKeys(keep), []string{"docklift-abcdef12-api:aaaaaaaa"}) {
		t.Fatalf("managed images must be ignored: %v", keep)
	}
	fallback := tags("api", "docklift-abcdef12-api:cccccccc")
	if keep = BuildKeepImageSet(nil, &fallback); !keep["docklift-abcdef12-api:cccccccc"] {
		t.Fatal("fallback")
	}
	if keep = BuildKeepImageSet([]ImageTags{{}, tags("api", "")}, nil); len(keep) != 0 {
		t.Fatal("empty maps")
	}
}

func TestParseImageTags(t *testing.T) {
	m, ok := ParseImageTags(rawJSON(`{"api":"docklift-abcdef12-api:1","bad":3,"empty":"","web":"w:1"}`))
	if !ok || !reflect.DeepEqual(m.Services(), []string{"api", "web"}) {
		t.Fatalf("parsed %+v", m)
	}
	for _, raw := range []string{`[]`, `{}`, `null`, `"x"`, `{"a":1}`, `not json`} {
		if _, ok := ParseImageTags(rawJSON(raw)); ok {
			t.Errorf("%s parsed as tags", raw)
		}
	}
	if _, ok := ParseImageTags(nil); ok {
		t.Error("nil")
	}
	m, _ = ParseImageTags(rawJSON(`{"a":"x:1","a":"x:2"}`))
	if v, _ := m.Get("a"); v != "x:2" || m.Len() != 1 {
		t.Errorf("duplicate key: %+v", m)
	}
}

func TestImageTagsJSOrder(t *testing.T) {
	m := tags("web", "w:1", "2", "two:1", "api", "a:1", "10", "ten:1", "01", "zero-one:1")
	if got := m.Services(); !reflect.DeepEqual(got, []string{"2", "10", "web", "api", "01"}) {
		t.Fatalf("order %v", got)
	}
	b, _ := m.MarshalJSON()
	if string(b) != `{"2":"two:1","10":"ten:1","web":"w:1","api":"a:1","01":"zero-one:1"}` {
		t.Fatalf("json %s", b)
	}
	m.Set("web", "w:2")
	if v, _ := m.Get("web"); v != "w:2" || m.Len() != 5 {
		t.Fatal("replace keeps one entry")
	}
	if got := string(*tags("a", "<&>").JSON()); got != `{"a":"<&>"}` {
		t.Fatalf("json column %s", got)
	}
}

func TestRollbackTargetGuard(t *testing.T) {
	tg := tags("api", "docklift-abcdef12-api:oldold01")
	cases := []struct {
		target, latest, status string
		tags                   ImageTags
		want                   RollbackGuard
	}{
		{"dep-a", "dep-a", "success", tg, RollbackGuard{Status: 400, Error: "That deployment is already the current success"}},
		{"dep-b", "dep-a", "success", ImageTags{}, RollbackGuard{Status: 409, Error: "This deployment has no stored image tags (pre-keep-2). Redeploy from git instead."}},
		{"dep-b", "dep-a", "failed", tg, RollbackGuard{Status: 404, Error: "Target successful deployment not found"}},
		{"dep-b", "dep-a", "success", tg, RollbackGuard{OK: true}},
		{"dep-b", "", "success", tg, RollbackGuard{OK: true}},
	}
	for _, c := range cases {
		if got := RollbackTargetGuard(c.target, c.latest, c.status, c.tags); got != c.want {
			t.Errorf("%+v -> %+v", c, got)
		}
	}
}

func TestRemoveUnusedDockliftImages(t *testing.T) {
	r, fd := newTestRuntime(t)
	ctx := context.Background()
	fd.images = []dockerx.Image{
		{ID: "sha256:keep", Tags: []string{"docklift-abcdef12-api:11111111"}},
		{ID: "sha256:old", Tags: []string{"docklift-abcdef12-api:00000000", "docklift-abcdef12-api:00000001"}},
		{ID: "sha256:inuse", Tags: []string{"docklift-abcdef12-web:00000000"}},
		{ID: "sha256:other", Tags: []string{"docklift-99999999-api:00000000"}},
		{ID: "sha256:pg", Tags: []string{"postgres:16"}},
		{ID: "sha256:fail", Tags: []string{"docklift-abcdef12-job:00000000"}},
	}
	fd.removeErr["docklift-abcdef12-job:00000000"] = errors.New("conflict")
	keep := map[string]bool{"docklift-abcdef12-api:11111111": true}
	inUse := map[string]bool{"inuse": true}
	res, err := r.RemoveUnusedDockliftImages(ctx, "abcdef12-0000-4000-8000-000000000000", keep, inUse)
	if err != nil {
		t.Fatal(err)
	}
	want := CleanupResult{
		Kept:         []string{"docklift-abcdef12-api:11111111"},
		Removed:      []string{"docklift-abcdef12-api:00000000", "docklift-abcdef12-api:00000001"},
		SkippedInUse: []string{"docklift-abcdef12-web:00000000"},
		Errors:       []string{"docklift-abcdef12-job:00000000: conflict"},
	}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("got %+v", res)
	}
	for _, c := range fd.Calls() {
		if strings.Contains(c, "postgres") || strings.Contains(c, "99999999") {
			t.Fatalf("touched a foreign image: %s", c)
		}
	}
	// Without a project every Docklift app image is a candidate.
	fd.calls = nil
	res, _ = r.RemoveUnusedDockliftImages(ctx, "", keep, map[string]bool{"sha256:in": true})
	if !slices.Contains(res.Removed, "docklift-99999999-api:00000000") || !slices.Contains(res.SkippedInUse, "docklift-abcdef12-web:00000000") {
		t.Fatalf("global %+v", res)
	}
}

func TestImageInUse(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"t:1", true}, {"sha256:abc", true}, {"abc", true}, {"sha256:ab", true}, {"sha256:zz", false}, {"other:1", false},
	}
	for _, c := range cases {
		if got := imageInUse("t:1", "sha256:abc", map[string]bool{c.ref: true}); got != c.want {
			t.Errorf("ref %q -> %v", c.ref, got)
		}
	}
}

func TestPostDeployCleanup(t *testing.T) {
	r, fd := newTestRuntime(t)
	ctx := context.Background()
	p := mustProject(t, r, store.ProjectCreate{Name: "a"})
	pid8 := ProjectID8(p.ID)
	repo := "docklift-" + pid8 + "-api"
	mustService(t, r, store.ServiceCreate{ProjectID: p.ID, Name: "api", ContainerName: strp("c_api")})
	fd.imageRefs["c_api"] = repo + ":running0"
	for _, tg := range []string{`{"api":"` + repo + `:previous"}`, "", `{"api":"` + repo + `:current0"}`} {
		mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusSuccess), ImageTags: rawJSON(tg)})
	}
	var imgs []dockerx.Image
	for _, s := range []string{"current0", "previous", "running0", "old00001", "old00002", "old00003", "old00004", "old00005", "old00006", "old00007", "old00008", "old00009"} {
		imgs = append(imgs, dockerx.Image{ID: "sha256:" + s, Tags: []string{repo + ":" + s}})
	}
	fd.images = imgs
	var log bytes.Buffer
	res := r.PostDeployCleanup(ctx, p.ID, tags("api", repo+":current0"), &log)
	if !res.Success || res.Message != "✓ Keep-2 cleanup: removed 9, kept 2" {
		t.Fatalf("result %+v\n%s", res, log.String())
	}
	out := log.String()
	for _, want := range []string{"   Kept 2 image tag(s) (current + previous)\n", "   Removed 9 older unused tag(s)\n", "     … and 1 more\n", "   Skipped 1 in-use tag(s)\n", "   BuildKit unused cache pruned\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	if !slices.Contains(fd.Calls(), "prune -f") {
		t.Fatal("BuildKit unused prune not run")
	}

	log.Reset()
	fd.prune = dockerx.PruneResult{OK: false}
	res = r.PostDeployCleanup(ctx, p.ID, ImageTags{}, &log)
	if res.Message != "✓ BuildKit unused cache pruned (no app images)" ||
		log.String() != "   ○ No Docklift app images to clean (image prune skipped)\n   ! BuildKit prune warning: failed\n" {
		t.Fatalf("%+v %q", res, log.String())
	}
}

func TestGlobalKeepSet(t *testing.T) {
	r, _ := newTestRuntime(t)
	a := mustProject(t, r, store.ProjectCreate{Name: "a"})
	b := mustProject(t, r, store.ProjectCreate{Name: "b"})
	for _, tg := range []string{`{"x":"docklift-aaaaaaaa-x:1"}`, `{"x":"docklift-aaaaaaaa-x:2"}`, `{"x":"docklift-aaaaaaaa-x:3"}`} {
		mustDeployment(t, r, store.DeploymentCreate{ProjectID: a.ID, Status: strp(StatusSuccess), ImageTags: rawJSON(tg)})
	}
	mustDeployment(t, r, store.DeploymentCreate{ProjectID: b.ID, Status: strp(StatusFailed), ImageTags: rawJSON(`{"x":"docklift-bbbbbbbb-x:1"}`)})
	keep, err := r.GlobalKeepSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keep) != 2 || keep["docklift-bbbbbbbb-x:1"] {
		t.Fatalf("keep %v", keep)
	}
}
