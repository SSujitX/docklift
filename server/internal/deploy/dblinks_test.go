package deploy

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"docklift/internal/store"
)

type linkFixture struct {
	r      *Runtime
	fd     *fakeDocker
	dbID   string
	dbHost string
	appID  string
	app2ID string
}

func newLinkFixture(t *testing.T) *linkFixture {
	t.Helper()
	r, fd := newTestRuntime(t)
	ctx := context.Background()
	dbp := mustProject(t, r, store.ProjectCreate{Name: "Main DB", ProjectType: strp("database"), DBEngine: strp(EnginePostgres), Status: strp("running")})
	host := ServiceContainerName("Main DB", dbp.ID, "db")
	mustService(t, r, store.ServiceCreate{ProjectID: dbp.ID, Name: "db", ContainerName: &host})
	for _, kv := range EngineRuntimeEnv(mustEngine(t, EnginePostgres), Credentials{Username: "docklift", Password: "p@ss/w", Database: "main_db"}) {
		if _, err := r.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: dbp.ID, Key: kv.Key, Value: kv.Value}); err != nil {
			t.Fatal(err)
		}
	}
	app := mustProject(t, r, store.ProjectCreate{Name: "web"})
	app2 := mustProject(t, r, store.ProjectCreate{Name: "web2"})
	return &linkFixture{r: r, fd: fd, dbID: dbp.ID, dbHost: host, appID: app.ID, app2ID: app2.ID}
}

func (f *linkFixture) link(t *testing.T, app, service, key string) *store.DatabaseLink {
	t.Helper()
	l, err := f.r.Store.UpsertLink(context.Background(), store.LinkCreate{DatabaseProjectID: f.dbID, AppProjectID: app, ServiceName: service, EnvKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (f *linkFixture) env(t *testing.T, app, service, key string) *store.EnvVariable {
	t.Helper()
	v, err := f.r.Store.FindEnvVariable(context.Background(), store.EnvFilter{ProjectID: &app, ServiceName: &service, Key: &key})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLoadDatabaseCredentials(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	loaded, ok, err := f.r.LoadDatabaseCredentials(ctx, f.dbID)
	if err != nil || !ok || loaded.Host != f.dbHost || loaded.Creds.Password != "p@ss/w" || loaded.Engine.ID != EnginePostgres {
		t.Fatalf("%+v %v %v", loaded, ok, err)
	}
	if url := ConnectionURLFor(loaded.Engine, loaded.Host, loaded.Creds); url != "postgresql://docklift:p%40ss%2Fw@"+f.dbHost+":5432/main_db" {
		t.Fatal(url)
	}
	for _, id := range []string{f.appID, "missing"} {
		if _, ok, err := f.r.LoadDatabaseCredentials(ctx, id); ok || err != nil {
			t.Fatalf("%s: ok=%v err=%v", id, ok, err)
		}
	}
}

func TestAssertEnvKeyAvailable(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	q := EnvKeyRequest{AppProjectID: f.appID, ServiceName: "", EnvKey: "DATABASE_URL", DatabaseProjectID: f.dbID}
	if err := f.r.AssertEnvKeyAvailable(ctx, q); err != nil {
		t.Fatal(err)
	}
	// An unlinked env value needs overwrite.
	if _, err := f.r.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: f.appID, Key: "DATABASE_URL", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	var conflict *LinkConflictError
	if err := f.r.AssertEnvKeyAvailable(ctx, q); !errors.As(err, &conflict) || conflict.Status != 409 ||
		conflict.Message != `"DATABASE_URL" already exists on this project/service. Pass overwrite=true to replace it with the managed database URL.` {
		t.Fatalf("err %v", err)
	}
	q.Overwrite = true
	if err := f.r.AssertEnvKeyAvailable(ctx, q); err != nil {
		t.Fatal(err)
	}
	// Owned by this database: fine without overwrite.
	q.Overwrite = false
	f.link(t, f.appID, "", "DATABASE_URL")
	if err := f.r.AssertEnvKeyAvailable(ctx, q); err != nil {
		t.Fatal(err)
	}
	// Owned by another database: conflict naming it.
	other := mustProject(t, f.r, store.ProjectCreate{Name: "Other DB", ProjectType: strp("database"), DBEngine: strp("redis")})
	q.DatabaseProjectID = other.ID
	if err := f.r.AssertEnvKeyAvailable(ctx, q); !errors.As(err, &conflict) ||
		conflict.Message != `"DATABASE_URL" on this project/service is already linked to database "Main DB". Unlink that first.` {
		t.Fatalf("err %v", err)
	}
}

func TestLinkLifecycle(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	l1 := f.link(t, f.appID, "", "DATABASE_URL")
	f.link(t, f.appID, "api", "DATABASE_URL")
	f.link(t, f.app2ID, "", "DB")
	if err := f.r.UpsertLinkedEnv(ctx, LinkedEnv{AppProjectID: f.appID, EnvKey: "DATABASE_URL", Value: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.r.Store.UpdateEnvVariable(ctx, f.env(t, f.appID, "", "DATABASE_URL").ID, store.EnvUpdate{IsBuildArg: store.Some(store.Ptr(true))}); err != nil {
		t.Fatal(err)
	}

	// Redeploying the database refreshes every linked URL and re-attaches networks.
	if err := f.r.ReapplyDatabaseLinksForDatabase(ctx, f.dbID); err != nil {
		t.Fatal(err)
	}
	want := "postgresql://docklift:p%40ss%2Fw@" + f.dbHost + ":5432/main_db"
	for _, c := range [][3]string{{f.appID, "", "DATABASE_URL"}, {f.appID, "api", "DATABASE_URL"}, {f.app2ID, "", "DB"}} {
		v := f.env(t, c[0], c[1], c[2])
		if v == nil || v.Value != want || !*v.IsRuntime || *v.IsBuildArg || !*v.IsSecret {
			t.Fatalf("%v: %+v", c, v)
		}
	}
	connects := 0
	for _, c := range f.fd.Calls() {
		if strings.HasPrefix(c, "connect dl-net-") && strings.HasSuffix(c, f.dbHost) {
			connects++
		}
	}
	if connects != 2 {
		t.Fatalf("expected one attach per linked app, calls %v", f.fd.Calls())
	}

	ok, failed, err := f.r.ReapplyDatabaseLinksForApp(ctx, f.appID)
	if err != nil || ok != 1 || failed != 0 {
		t.Fatalf("ok=%d failed=%d err=%v", ok, failed, err)
	}
	f.fd.connectErr[f.dbHost] = errors.New("no such network")
	if ok, failed, _ = f.r.ReapplyDatabaseLinksForApp(ctx, f.appID); ok != 0 || failed != 1 {
		t.Fatalf("ok=%d failed=%d", ok, failed)
	}

	// Unlinking the shared-scope link removes its env; the network stays while
	// the api-scoped link remains.
	f.fd.calls = nil
	if err := f.r.UnlinkDatabaseLink(ctx, l1.ID); err != nil {
		t.Fatal(err)
	}
	if f.env(t, f.appID, "", "DATABASE_URL") != nil || f.env(t, f.appID, "api", "DATABASE_URL") == nil {
		t.Fatal("env cleanup")
	}
	if slices.ContainsFunc(f.fd.Calls(), func(c string) bool { return strings.HasPrefix(c, "disconnect") }) {
		t.Fatalf("network detached while a link remains: %v", f.fd.Calls())
	}
	if err := f.r.UnlinkDatabaseLink(ctx, "missing"); err != nil {
		t.Fatal(err)
	}

	if err := f.r.DisconnectLinkedDatabasesFromApp(ctx, f.appID); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.fd.Calls(), "disconnect "+ProjectNetworkName(f.appID)+" "+f.dbHost) {
		t.Fatalf("calls %v", f.fd.Calls())
	}

	// Deleting the database cleans every peer.
	f.fd.calls = nil
	if err := f.r.CleanupLinksForDatabaseProject(ctx, f.dbID); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.r.Store.CountLinks(ctx, store.LinkFilter{DatabaseProjectID: &f.dbID}); n != 0 {
		t.Fatalf("%d links left", n)
	}
	if f.env(t, f.appID, "api", "DATABASE_URL") != nil || f.env(t, f.app2ID, "", "DB") != nil {
		t.Fatal("peer env left behind")
	}
	if !slices.Contains(f.fd.Calls(), "disconnect "+ProjectNetworkName(f.app2ID)+" "+f.dbHost) {
		t.Fatalf("calls %v", f.fd.Calls())
	}
}

func TestDeleteLinkedEnvKeepsOwnedKeys(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	f.link(t, f.appID, "", "DATABASE_URL")
	_ = f.r.UpsertLinkedEnv(ctx, LinkedEnv{AppProjectID: f.appID, EnvKey: "DATABASE_URL", Value: "x"})
	if deleted, err := f.r.DeleteLinkedEnvIfUnowned(ctx, f.appID, "", "DATABASE_URL"); deleted || err != nil {
		t.Fatal("owned key deleted")
	}
	if deleted, err := f.r.DeleteLinkedEnvIfUnowned(ctx, f.appID, "", "OTHER"); !deleted || err != nil {
		t.Fatal("unowned key")
	}
}

func TestEnsureDBOnAppNetworkRequiresContainer(t *testing.T) {
	r, _ := newTestRuntime(t)
	dbp := mustProject(t, r, store.ProjectCreate{Name: "db", ProjectType: strp("database"), DBEngine: strp("redis")})
	if err := r.EnsureDBOnAppNetwork(context.Background(), dbp.ID, "app"); !errors.Is(err, ErrDatabaseNotReady) {
		t.Fatalf("err %v", err)
	}
}
