package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"docklift/internal/db"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return New(d)
}

func mustProject(t *testing.T, s *Store, name string, in ProjectCreate) *Project {
	t.Helper()
	in.Name = name
	p, err := s.CreateProject(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUsers(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if n, err := s.CountUsers(ctx); err != nil || n != 0 {
		t.Fatalf("CountUsers = %d, %v", n, err)
	}
	changed := db.FromUnixMilli(1795000000123)
	u, err := s.CreateUser(ctx, UserCreate{Name: "Ada", Email: "ada@example.com", Password: "hash", PasswordChangedAt: &changed})
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != "admin" || u.PasswordVersion() != 1795000000123 {
		t.Errorf("defaults: role=%q pwdv=%d", u.Role, u.PasswordVersion())
	}
	if _, err := s.CreateUser(ctx, UserCreate{Name: "B", Email: "ada@example.com", Password: "h"}); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate email err = %v", err)
	}
	got, err := s.GetUserByEmail(ctx, "ada@example.com")
	if err != nil || got.ID != u.ID || got.CreatedAt.UnixMilli() != u.CreatedAt.UnixMilli() {
		t.Errorf("GetUserByEmail = %+v, %v", got, err)
	}
	if _, err := s.GetUser(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing user err = %v", err)
	}
	other, _ := s.CreateUser(ctx, UserCreate{Name: "B", Email: "b@example.com", Password: "h", Role: "viewer"})
	if _, err := s.UpdateUser(ctx, other.ID, UserUpdate{Email: Some("ada@example.com")}); !errors.Is(err, ErrConflict) {
		t.Errorf("update to duplicate email err = %v", err)
	}
	upd, err := s.UpdateUser(ctx, u.ID, UserUpdate{Name: Some("Ada L"), PasswordChangedAt: Some[*db.Time](nil)})
	if err != nil || upd.Name != "Ada L" || upd.PasswordChangedAt != nil || upd.PasswordVersion() != 0 {
		t.Errorf("UpdateUser = %+v, %v", upd, err)
	}
	if _, err := s.UpdateUser(ctx, "missing", UserUpdate{Name: Some("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing err = %v", err)
	}
	if n, _ := s.CountUsersByRole(ctx, "admin"); n != 1 {
		t.Errorf("admins = %d", n)
	}
	if first, err := s.FirstUserByRole(ctx, "viewer"); err != nil || first.ID != other.ID {
		t.Errorf("FirstUserByRole = %v, %v", first, err)
	}
	if oldest, err := s.OldestUser(ctx); err != nil || oldest.ID != u.ID {
		t.Errorf("OldestUser = %v, %v", oldest, err)
	}
	b, _ := json.Marshal(u)
	if strings.Contains(string(b), "hash") || !strings.Contains(string(b), `"passwordChangedAt":"2026-11-18T11:06:40.123Z"`) {
		t.Errorf("user JSON = %s", b)
	}
}

func TestProjectsCRUDAndJSON(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := mustProject(t, s, "app", ProjectCreate{SourceType: Ptr("upload"), ProjectType: Ptr("app"), Status: Ptr("pending")})
	if p.BuildType != "auto" || p.BaseDirectory != "." || p.InternalPort != 3000 || p.PublishHostPort || p.AutoDeploy == nil || *p.AutoDeploy {
		t.Errorf("defaults not applied: %+v", p)
	}
	b, _ := json.Marshal(p)
	wantPrefix := `{"id":"` + p.ID + `","name":"app","description":null,"source_type":"upload","project_type":"app","github_url":null,"github_branch":null,"domain":null,"port":null,"status":"pending","container_name":null,"auto_deploy":false,"webhook_secret":null,"build_type":"auto","base_directory":".","dockerfile_path":null,"internal_port":3000,"publish_host_port":false,"db_engine":null,"created_at":"`
	if !strings.HasPrefix(string(b), wantPrefix) {
		t.Errorf("project JSON (Prisma key order and nulls):\n got %s\nwant prefix %s", b, wantPrefix)
	}

	time.Sleep(2 * time.Millisecond)
	upd, err := s.UpdateProject(ctx, p.ID, ProjectUpdate{Description: Some(Ptr("d")), Domain: Some(Ptr("a.example.com")), InternalPort: Some(8080)})
	if err != nil {
		t.Fatal(err)
	}
	if *upd.Description != "d" || upd.InternalPort != 8080 || upd.UpdatedAt.UnixMilli() <= p.UpdatedAt.UnixMilli() {
		t.Errorf("update = %+v", upd)
	}
	cleared, err := s.UpdateProject(ctx, p.ID, ProjectUpdate{Domain: Null[string]()})
	if err != nil || cleared.Domain != nil {
		t.Errorf("clear domain = %v, %v", cleared.Domain, err)
	}
	if _, err := s.UpdateProject(ctx, "missing", ProjectUpdate{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing = %v", err)
	}

	mustProject(t, s, "untyped", ProjectCreate{})
	time.Sleep(2 * time.Millisecond)
	dbp := mustProject(t, s, "pg", ProjectCreate{ProjectType: Ptr("database"), DBEngine: Ptr("postgres"), Status: Ptr("running")})
	notDB, err := s.ListProjects(ctx, ProjectFilter{ProjectTypeNot: Ptr("database")}, OrderNone)
	if err != nil || len(notDB) != 1 || notDB[0].ID != p.ID {
		t.Errorf("ProjectTypeNot must exclude NULL like Prisma: %v %v", names(notDB), err)
	}
	all, _ := s.ListProjects(ctx, ProjectFilter{}, OrderCreatedDesc)
	if len(all) != 3 || all[0].ID != dbp.ID {
		t.Errorf("order created desc = %v", names(all))
	}
	if none, _ := s.ListProjects(ctx, ProjectFilter{StatusIn: []string{}}, OrderNone); len(none) != 0 {
		t.Errorf("empty IN must match nothing, got %v", names(none))
	}
	gh := mustProject(t, s, "gh", ProjectCreate{SourceType: Ptr("github"), AutoDeploy: Ptr(true), GithubURL: Ptr("https://github.com/a/b.git")})
	hooks, _ := s.ListProjects(ctx, ProjectFilter{SourceType: Ptr("github"), AutoDeploy: Ptr(true),
		GithubURLIn: []string{"https://github.com/a/b", "https://github.com/a/b.git"}}, OrderNone)
	if len(hooks) != 1 || hooks[0].ID != gh.ID {
		t.Errorf("webhook lookup = %v", names(hooks))
	}
	active, err := s.ListActiveProjectsForPorts(ctx)
	if err != nil || len(active) != 2 || active[0].ID != dbp.ID || active[1].ID != p.ID {
		t.Errorf("active projects = %+v, %v", active, err)
	}
	if found, err := s.FindProject(ctx, ProjectFilter{ID: &dbp.ID, ProjectType: Ptr("database")}); err != nil || found.ID != dbp.ID {
		t.Errorf("FindProject = %v, %v", found, err)
	}
	if err := s.DeleteProject(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing = %v", err)
	}
}

func names(ps []*Project) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestCascadeDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	app := mustProject(t, s, "app", ProjectCreate{})
	dbp := mustProject(t, s, "pg", ProjectCreate{ProjectType: Ptr("database")})
	if _, err := s.CreateService(ctx, ServiceCreate{ProjectID: app.ID, Name: "web", DockerfilePath: "Dockerfile"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEnvVariable(ctx, EnvCreate{ProjectID: app.ID, Key: "K", Value: "v"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDeployment(ctx, DeploymentCreate{ProjectID: app.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVolume(ctx, VolumeCreate{ProjectID: app.ID, ServiceName: "web", Name: "data", DisplayName: "Data", MountPath: "/data"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.InsertLockedPort(ctx, 5500, app.ID); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := s.UpsertLink(ctx, LinkCreate{DatabaseProjectID: dbp.ID, AppProjectID: app.ID, EnvKey: "DATABASE_URL"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, app.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"services", "env_variables", "deployments", "persistent_volumes", "ports", "database_links"} {
		var n int
		if err := s.DB().Reader().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s rows after cascade = %d, %v", table, n, err)
		}
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	_, err := newStore(t).CreateService(context.Background(), ServiceCreate{ProjectID: "nope", Name: "x", DockerfilePath: "D"})
	if err == nil {
		t.Fatal("orphan service must be rejected")
	}
}

func TestServices(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := mustProject(t, s, "app", ProjectCreate{})
	web, _ := s.CreateService(ctx, ServiceCreate{ProjectID: p.ID, Name: "web", DockerfilePath: "Dockerfile", ContainerName: Ptr("c-web"), Port: Ptr(5501), Status: Ptr("running"), Domain: Ptr("a.example.com")})
	time.Sleep(2 * time.Millisecond)
	api, _ := s.CreateService(ctx, ServiceCreate{ProjectID: p.ID, Name: "api", DockerfilePath: "api/Dockerfile", Status: Ptr("building")})

	first, err := s.FindService(ctx, ServiceFilter{ProjectID: &p.ID}, OrderCreatedAsc)
	if err != nil || first.ID != web.ID {
		t.Errorf("first service = %v, %v", first, err)
	}
	stale, _ := s.ListServices(ctx, ServiceFilter{ProjectID: &p.ID, NameNotIn: []string{"web"}}, OrderNone)
	if len(stale) != 1 || stale[0].ID != api.ID {
		t.Errorf("NameNotIn = %+v", stale)
	}
	routed, _ := s.ListServices(ctx, ServiceFilter{HasDomain: Ptr(true), HasContainerName: Ptr(true), StatusIn: []string{"running", "starting"}}, OrderNone)
	if len(routed) != 1 || routed[0].ID != web.ID {
		t.Errorf("nginx sync filter = %+v", routed)
	}
	n, err := s.UpdateServices(ctx, ServiceFilter{ProjectID: &p.ID}, ServiceUpdate{Status: Some(Ptr("stopped"))})
	if err != nil || n != 2 {
		t.Errorf("UpdateServices = %d, %v", n, err)
	}
	upd, err := s.UpdateService(ctx, web.ID, ServiceUpdate{Port: Null[int](), Domain: Null[string]()})
	if err != nil || upd.Port != nil || upd.Domain != nil || *upd.Status != "stopped" {
		t.Errorf("UpdateService = %+v, %v", upd, err)
	}
	if err := s.DeleteService(ctx, api.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetService(ctx, api.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted service = %v", err)
	}
}

func TestDeployments(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := mustProject(t, s, "app", ProjectCreate{})
	tags, _ := db.MarshalJSONValue(map[string]string{"web": "img:1"})
	ok, _ := s.CreateDeployment(ctx, DeploymentCreate{ProjectID: p.ID, Status: Ptr("success"), ImageTags: &tags})
	if *ok.Trigger != "manual" {
		t.Errorf("trigger default = %q", *ok.Trigger)
	}
	time.Sleep(2 * time.Millisecond)
	running, _ := s.CreateDeployment(ctx, DeploymentCreate{ProjectID: p.ID, Status: Ptr("in_progress"), Trigger: Ptr("restart"), Logs: Ptr("...")})

	list, err := s.ListDeployments(ctx, DeploymentFilter{ProjectID: &p.ID}, OrderCreatedDesc, Page{Limit: 1})
	if err != nil || len(list) != 1 || list[0].ID != running.ID {
		t.Errorf("paged list = %v, %v", list, err)
	}
	if offset, _ := s.ListDeployments(ctx, DeploymentFilter{ProjectID: &p.ID}, OrderCreatedDesc, Page{Limit: 10, Offset: 1}); len(offset) != 1 || offset[0].ID != ok.ID {
		t.Errorf("offset list = %v", offset)
	}
	if n, _ := s.CountDeployments(ctx, DeploymentFilter{ProjectID: &p.ID, Status: Ptr("in_progress")}); n != 1 {
		t.Errorf("count in_progress = %d", n)
	}
	imgs, err := s.ListDeploymentImageTags(ctx, p.ID, 5)
	if err != nil || len(imgs) != 1 || string(*imgs[0]) != `{"web":"img:1"}` {
		t.Errorf("image tags = %v, %v", imgs, err)
	}
	finished := db.Now()
	n, err := s.UpdateDeployments(ctx, DeploymentFilter{ID: &running.ID, StatusNot: Ptr("cancelled")},
		DeploymentUpdate{Status: Some(Ptr("failed")), FinishedAt: Some(&finished)})
	if err != nil || n != 1 {
		t.Errorf("guarded updateMany = %d, %v", n, err)
	}
	if n, _ := s.UpdateDeployments(ctx, DeploymentFilter{ProjectID: &p.ID, Status: Ptr("in_progress"), Trigger: Ptr("restart")}, DeploymentUpdate{Status: Some(Ptr("failed"))}); n != 0 {
		t.Errorf("no in_progress restart left, updated %d", n)
	}
	got, _ := s.GetDeployment(ctx, running.ID)
	if *got.Status != "failed" || got.FinishedAt == nil || got.FinishedAt.UnixMilli() != finished.UnixMilli() {
		t.Errorf("deployment after update = %+v", got)
	}
	found, err := s.FindDeployment(ctx, DeploymentFilter{ProjectID: &p.ID, Status: Ptr("success")}, OrderCreatedDesc)
	if err != nil || found.ID != ok.ID {
		t.Errorf("FindDeployment = %v, %v", found, err)
	}
	cleared, err := s.UpdateDeployment(ctx, ok.ID, DeploymentUpdate{ImageTags: Some[*db.JSON](nil), CommitSHA: Some(Ptr("abc"))})
	if err != nil || cleared.ImageTags != nil || *cleared.CommitSHA != "abc" {
		t.Errorf("UpdateDeployment = %+v, %v", cleared, err)
	}
}

func TestEnvVariables(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := mustProject(t, s, "app", ProjectCreate{})
	shared, err := s.CreateEnvVariable(ctx, EnvCreate{ProjectID: p.ID, Key: "K", Value: "shared", IsRuntime: Ptr(true), IsBuildArg: Ptr(false)})
	if err != nil || shared.IsSecret == nil || *shared.IsSecret {
		t.Fatalf("is_secret default: %+v %v", shared, err)
	}
	if _, err := s.CreateEnvVariable(ctx, EnvCreate{ProjectID: p.ID, ServiceName: "api", Key: "K", Value: "scoped"}); err != nil {
		t.Errorf("same key in another scope must be allowed: %v", err)
	}
	if _, err := s.CreateEnvVariable(ctx, EnvCreate{ProjectID: p.ID, Key: "K", Value: "dup"}); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate scoped key err = %v", err)
	}
	list, _ := s.ListEnvVariables(ctx, EnvFilter{ProjectID: &p.ID}, OrderServiceKey)
	if len(list) != 2 || list[0].ServiceName != "" || list[1].ServiceName != "api" {
		t.Errorf("ordered list = %+v", list)
	}
	upd, err := s.UpdateEnvVariable(ctx, shared.ID, EnvUpdate{Value: Some("new"), IsSecret: Some(Ptr(true))})
	if err != nil || upd.Value != "new" || !*upd.IsSecret {
		t.Errorf("update = %+v, %v", upd, err)
	}
	if _, err := s.DeleteEnvVariables(ctx, EnvFilter{}); err == nil {
		t.Error("unfiltered deleteMany must be refused")
	}
	n, err := s.DeleteEnvVariables(ctx, EnvFilter{ProjectID: &p.ID, ServiceName: Ptr("api"), Key: Ptr("K")})
	if err != nil || n != 1 {
		t.Errorf("deleteMany = %d, %v", n, err)
	}
	if err := s.DeleteEnvVariable(ctx, shared.ID); err != nil {
		t.Error(err)
	}
	if removed, err := s.DedupeEnvVariables(ctx); err != nil || removed != 0 {
		t.Errorf("dedupe = %d, %v", removed, err)
	}
}

func TestVolumes(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := mustProject(t, s, "app", ProjectCreate{})
	v, err := s.CreateVolume(ctx, VolumeCreate{ProjectID: p.ID, ServiceName: "web", Name: "data", DisplayName: "Data", MountPath: "/data"})
	if err != nil {
		t.Fatal(err)
	}
	for _, dup := range []VolumeCreate{
		{ProjectID: p.ID, ServiceName: "api", Name: "data", DisplayName: "x", MountPath: "/other"},
		{ProjectID: p.ID, ServiceName: "web", Name: "other", DisplayName: "x", MountPath: "/data"},
	} {
		if _, err := s.CreateVolume(ctx, dup); !errors.Is(err, ErrConflict) {
			t.Errorf("duplicate %+v err = %v", dup, err)
		}
	}
	if _, err := s.GetVolume(ctx, "other-project", v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("volume of another project must not be found: %v", err)
	}
	moved, err := s.UpdateVolumeMountPath(ctx, v.ID, "/var/data")
	if err != nil || moved.MountPath != "/var/data" {
		t.Errorf("move = %+v, %v", moved, err)
	}
	if err := s.DeleteVolume(ctx, v.ID); err != nil {
		t.Error(err)
	}
}

func TestPortsAndTransactions(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustProject(t, s, "a", ProjectCreate{})
	b := mustProject(t, s, "b", ProjectCreate{})

	err := s.InTx(ctx, func(tx *Store) error {
		inserted, err := tx.InsertLockedPort(ctx, 5500, a.ID)
		if err != nil || !inserted {
			return errors.Join(errors.New("insert"), err)
		}
		again, err := tx.InsertLockedPort(ctx, 5500, b.ID)
		if err != nil || again {
			return errors.Join(errors.New("conflicting insert must report false"), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	locked, _ := s.ListLockedPorts(ctx)
	if len(locked) != 1 || locked[0] != 5500 {
		t.Errorf("locked = %v", locked)
	}
	if claimed, _ := s.ClaimPort(ctx, 5500, b.ID); claimed {
		t.Error("a locked port must not be claimable")
	}
	if n, _ := s.ReleasePort(ctx, a.ID, 5500); n != 1 {
		t.Errorf("release = %d", n)
	}
	if claimed, err := s.ClaimPort(ctx, 5500, b.ID); err != nil || !claimed {
		t.Errorf("claim released port = %v, %v", claimed, err)
	}
	if _, err := s.DB().Writer().Exec(`INSERT INTO ports (port, project_id, is_locked) VALUES (5600, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	if claimed, _ := s.ClaimPort(ctx, 5600, b.ID); claimed {
		t.Error("Prisma `is_locked: {not: true}` never matches NULL")
	}
	rows, err := s.ListPortsWithProject(ctx)
	if err != nil || len(rows) != 2 || rows[0].Project == nil || rows[0].Project.ID != b.ID || rows[1].Project != nil {
		t.Errorf("ports with project = %+v, %v", rows, err)
	}
	if n, _ := s.DeletePort(ctx, 5600); n != 1 {
		t.Errorf("delete port = %d", n)
	}

	rollback := errors.New("rollback")
	err = s.InTx(ctx, func(tx *Store) error {
		if _, err := tx.InsertLockedPort(ctx, 5501, a.ID); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("InTx err = %v", err)
	}
	if _, err := s.GetPort(ctx, 5501); !errors.Is(err, ErrNotFound) {
		t.Errorf("rolled back insert visible: %v", err)
	}
	func() {
		defer func() { _ = recover() }()
		_ = s.InTx(ctx, func(tx *Store) error {
			_, _ = tx.InsertLockedPort(ctx, 5502, a.ID)
			panic("boom")
		})
	}()
	if _, err := s.GetPort(ctx, 5502); !errors.Is(err, ErrNotFound) {
		t.Errorf("panicking transaction must roll back: %v", err)
	}
	if err := s.InTx(ctx, func(tx *Store) error {
		return tx.InTx(ctx, func(inner *Store) error {
			_, err := inner.InsertLockedPort(ctx, 5503, a.ID)
			return err
		})
	}); err != nil {
		t.Errorf("nested InTx = %v", err)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, ok, err := s.GetSetting(ctx, "github_app_id"); ok || err != nil {
		t.Errorf("missing setting = %v, %v", ok, err)
	}
	if err := s.SetSetting(ctx, "github_app_id", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "github_app_id", "2"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := s.GetSetting(ctx, "github_app_id"); !ok || v != "2" {
		t.Errorf("upsert = %q %v", v, ok)
	}
	var updated *db.Time
	if err := s.DB().Reader().QueryRow(`SELECT updated_at FROM settings WHERE key = 'github_app_id'`).Scan(&updated); err != nil || updated != nil {
		t.Errorf("updated_at must stay NULL like the TypeScript upsert: %v %v", updated, err)
	}
	if err := s.DeleteSetting(ctx, "github_app_id"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetSetting(ctx, "github_app_id"); ok {
		t.Error("deleted setting still present")
	}
}

func TestDatabaseLinks(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	app := mustProject(t, s, "app", ProjectCreate{ProjectType: Ptr("app"), Status: Ptr("running")})
	pg := mustProject(t, s, "pg", ProjectCreate{ProjectType: Ptr("database"), DBEngine: Ptr("postgres")})
	time.Sleep(2 * time.Millisecond)
	other := mustProject(t, s, "pg2", ProjectCreate{ProjectType: Ptr("database")})
	if _, err := s.CreateService(ctx, ServiceCreate{ProjectID: app.ID, Name: "web", DockerfilePath: "D", Status: Ptr("running")}); err != nil {
		t.Fatal(err)
	}
	key := LinkCreate{DatabaseProjectID: pg.ID, AppProjectID: app.ID, EnvKey: "DATABASE_URL"}
	l1, err := s.UpsertLink(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := s.UpsertLink(ctx, key)
	if err != nil || l2.ID != l1.ID {
		t.Errorf("upsert must return the existing link: %v %v", l2, err)
	}
	if _, err := s.UpsertLink(ctx, LinkCreate{DatabaseProjectID: other.ID, AppProjectID: app.ID, EnvKey: "DATABASE_URL"}); !errors.Is(err, ErrConflict) {
		t.Errorf("second owner of an env key err = %v", err)
	}
	conflict, err := s.FindLinkOwningEnvKey(ctx, app.ID, "", "DATABASE_URL", other.ID)
	if err != nil || conflict.DatabaseProjectName != "pg" {
		t.Errorf("conflict = %+v, %v", conflict, err)
	}
	if _, err := s.FindLinkOwningEnvKey(ctx, app.ID, "", "DATABASE_URL", pg.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("own link is not a conflict: %v", err)
	}
	if n, _ := s.CountLinks(ctx, LinkFilter{DatabaseProjectID: &pg.ID, AppProjectID: &app.ID}); n != 1 {
		t.Errorf("count = %d", n)
	}
	forApp, err := s.ListLinksForApp(ctx, app.ID)
	if err != nil || len(forApp) != 1 || forApp[0].DatabaseProject.Name != "pg" || *forApp[0].DatabaseProject.DBEngine != "postgres" {
		t.Errorf("links for app = %+v, %v", forApp, err)
	}
	forDB, err := s.ListLinksForDatabase(ctx, pg.ID)
	if err != nil || len(forDB) != 1 || len(forDB[0].AppProject.Services) != 1 {
		t.Errorf("links for db = %+v, %v", forDB, err)
	}
	b, _ := json.Marshal(forDB[0])
	if !strings.Contains(string(b), `"app_project":{"id":"`+app.ID+`","name":"app","status":"running","services":[{"id":`) {
		t.Errorf("link JSON = %s", b)
	}
	dbs, err := s.ListDatabaseProjects(ctx)
	if err != nil || len(dbs) != 2 || dbs[0].ID != other.ID || len(dbs[1].DatabaseLinksAsDb) != 1 || dbs[1].Services == nil {
		t.Errorf("database projects = %+v, %v", dbs, err)
	}
	withSvc, err := s.GetProjectWithServices(ctx, ProjectFilter{ID: &app.ID, ProjectTypeNot: Ptr("database")}, true)
	if err != nil || len(withSvc.Services) != 1 {
		t.Errorf("project with services = %+v, %v", withSvc, err)
	}
	if err := s.DeleteLink(ctx, l1.ID); err != nil {
		t.Error(err)
	}
	if _, err := s.FindLink(ctx, LinkFilter{ID: &l1.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted link = %v", err)
	}
}

func TestMissingTable(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := New(d).CountUsers(ctx); !errors.Is(err, ErrMissingTable) {
		t.Errorf("unmigrated database err = %v, want ErrMissingTable", err)
	}
}
