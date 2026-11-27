package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"docklift/internal/deploy"
	"docklift/internal/execx"
	"docklift/internal/store"
)

// deletable creates an uploaded project with a service, a volume, a runtime
// compose file and a service nginx conf.
func deletable(t *testing.T, f *fixture) (*store.Project, *store.Service, *store.PersistentVolume, string) {
	t.Helper()
	ctx := context.Background()
	id := f.createUpload("Del Me")["id"].(string)
	p, _ := f.d.Store.GetProject(ctx, id)
	svc := f.service(id, "app", store.Ptr("dl_del_me_app"))
	vol, err := f.d.Store.CreateVolume(ctx, store.VolumeCreate{ProjectID: id, ServiceName: "app", Name: "dl-vol", DisplayName: "data", MountPath: "/data"})
	if err != nil {
		t.Fatal(err)
	}
	compose := f.d.Deploy.ComposePath(id)
	if err := os.MkdirAll(filepath.Dir(compose), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conf, err := f.d.Edge.Nginx.ServiceConfPath(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("server {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p, svc, vol, conf
}

func TestDeleteProject(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	p, _, _, conf := deletable(t, f)
	f.d.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: p.ID, Key: "A", Value: "1"})

	f.do("DELETE", "/"+p.ID, nil).expect(t, 200)

	if _, err := f.d.Store.GetProject(ctx, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("row must be deleted")
	}
	for _, path := range []string{f.d.Deploy.ProjectPath(p.ID), f.d.Deploy.StatePath(p.ID), conf} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s must be removed", path)
		}
	}
	primary := deploy.ComposeProjectName(p.Name, p.ID)
	args := f.run.Args()
	want := []string{
		"compose -f " + f.d.Deploy.ComposePath(p.ID) + " -p " + primary + " down --remove-orphans --rmi all",
		"compose -p " + p.ID + " down --remove-orphans --rmi all",
		"volume rm -f dl-vol",
	}
	for _, w := range want {
		if !slices.Contains(args, w) {
			t.Errorf("missing docker call %q in %v", w, args)
		}
	}
	for _, c := range f.run.Cmds() {
		if c.Args[0] == "compose" && c.Dir != f.d.Deploy.ProjectPath(p.ID) {
			t.Errorf("compose ran in %s", c.Dir)
		}
	}
	net := deploy.ProjectNetworkName(p.ID)
	calls := strings.Join(f.engine.Calls(), "\n")
	for _, w := range []string{"network disconnect " + net + " " + f.d.Config.NginxProxyContainer, "network rm " + net} {
		if !strings.Contains(calls, w) {
			t.Errorf("missing engine call %q in\n%s", w, calls)
		}
	}
	// The lock is released.
	if f.d.Deploy.Tracker.IsDeploying(p.ID) {
		t.Fatal("deploy lock still held")
	}
	f.do("DELETE", "/"+p.ID, nil).expectError(t, 404, msgProjectNotFound)
}

func TestDeleteTeardownFailureKeepsProject(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	p, _, _, _ := deletable(t, f)
	f.run.answer = func(args string) (execx.Result, error) {
		switch {
		case strings.HasPrefix(args, "compose"):
			return execx.Result{Stderr: []byte("Cannot connect\n"), ExitCode: 1}, exitErr(1)
		case strings.HasPrefix(args, "ps -aq"):
			return execx.Result{Stdout: []byte("abc\n")}, nil
		}
		return execx.Result{}, nil
	}
	r := f.do("DELETE", "/"+p.ID, nil)
	primary := deploy.ComposeProjectName(p.Name, p.ID)
	r.expectError(t, 409, `Cannot delete project: Docker teardown failed for "`+primary+`" (owned containers/networks still present). Cannot connect`)
	if _, err := f.d.Store.GetProject(ctx, p.ID); err != nil {
		t.Fatal("row must survive")
	}
	if _, err := os.Stat(f.d.Deploy.ProjectPath(p.ID)); err != nil {
		t.Fatal("files must survive")
	}
	if vols, _ := f.d.Store.ListVolumes(ctx, p.ID, store.OrderNone); len(vols) != 1 {
		t.Fatal("volume row must survive")
	}
	if !strings.Contains(strings.Join(f.engine.Calls(), "\n"), "network connect "+deploy.ProjectNetworkName(p.ID)+" "+f.d.Config.NginxProxyContainer) {
		t.Fatalf("proxy must be re-attached: %v", f.engine.Calls())
	}
	if f.d.Deploy.Tracker.IsDeploying(p.ID) {
		t.Fatal("deploy lock still held")
	}
}

func TestDeleteVolumeFailure(t *testing.T) {
	f := newFixture(t, Options{})
	p, _, _, _ := deletable(t, f)
	f.run.answer = func(args string) (execx.Result, error) {
		if strings.HasPrefix(args, "volume rm") {
			return execx.Result{Stderr: []byte("volume is in use"), ExitCode: 1}, exitErr(1)
		}
		return execx.Result{}, nil
	}
	f.do("DELETE", "/"+p.ID, nil).expectError(t, 409, `Could not remove persistent volume "data". Detach any containers using it, then retry project deletion.`)
	if _, err := f.d.Store.GetProject(context.Background(), p.ID); err != nil {
		t.Fatal("row must survive")
	}

	// A volume that is already gone is fine.
	f.run.answer = func(args string) (execx.Result, error) {
		if strings.HasPrefix(args, "volume rm") {
			return execx.Result{Stderr: []byte("Error: No such volume: dl-vol"), ExitCode: 1}, exitErr(1)
		}
		return execx.Result{}, nil
	}
	f.do("DELETE", "/"+p.ID, nil).expect(t, 200)
}

func TestDeleteBusy(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	p := f.createUpload("busy")
	id := p["id"].(string)
	release, ok := f.d.Deploy.Tracker.Acquire(id, "deploy-1")
	if !ok {
		t.Fatal("acquire")
	}
	f.do("DELETE", "/"+id, nil).expectError(t, 409, msgDeployInProgress)
	if owner, _ := f.d.Deploy.Tracker.Owner(id); owner != "deploy-1" {
		t.Fatalf("delete must not steal the lock: %q", owner)
	}
	release()
	d, err := f.d.Store.CreateDeployment(ctx, store.DeploymentCreate{ProjectID: id, Status: store.Ptr(deploy.StatusInProgress)})
	if err != nil {
		t.Fatal(err)
	}
	f.do("DELETE", "/"+id, nil).expectError(t, 409, msgDeployInProgress)
	if _, err := f.d.Store.UpdateDeployment(ctx, d.ID, store.DeploymentUpdate{Status: store.Some(store.Ptr(deploy.StatusFailed))}); err != nil {
		t.Fatal(err)
	}
	if len(f.run.Args()) != 0 {
		t.Fatalf("no docker call while busy: %v", f.run.Args())
	}
	f.do("DELETE", "/"+id, nil).expect(t, 200)
}

func TestDeleteHoldsDeployLock(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.createUpload("locked")
	id := p["id"].(string)
	var during bool
	f.run.answer = func(args string) (execx.Result, error) {
		if strings.HasPrefix(args, "compose") {
			_, ok := f.d.Deploy.Tracker.Acquire(id, "late-deploy")
			during = !ok
		}
		return execx.Result{}, nil
	}
	f.do("DELETE", "/"+id, nil).expect(t, 200)
	if !during {
		t.Fatal("a deploy could start while the project was being deleted")
	}
}

func TestDeleteDockerUnavailable(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.createUpload("nodocker")
	f.engine.pingErr = unavailable()
	f.do("DELETE", "/"+p["id"].(string), nil).expectError(t, 503, msgDockerDown)
	if len(f.run.Args()) != 0 {
		t.Fatalf("no teardown without Docker: %v", f.run.Args())
	}
}

func TestDeleteMissingCheckout(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.project(store.ProjectCreate{Name: "nofiles"})
	f.do("DELETE", "/"+p.ID, nil).expect(t, 200)
	for _, c := range f.run.Cmds() {
		if c.Args[0] != "compose" {
			continue
		}
		if c.Dir != f.d.Config.DeploymentsPath {
			t.Fatalf("compose must run in the deployments dir, ran in %s", c.Dir)
		}
		if slices.Contains(c.Args, "-f") {
			t.Fatal("no runtime compose file exists")
		}
	}
}

func TestDeleteDatabaseProjectCleansLinks(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	dbp := f.project(store.ProjectCreate{Name: "pg", ProjectType: store.Ptr("database"), DBEngine: store.Ptr("postgres")})
	appP := f.project(store.ProjectCreate{Name: "app"})
	if _, err := f.d.Store.UpsertLink(ctx, store.LinkCreate{DatabaseProjectID: dbp.ID, AppProjectID: appP.ID, EnvKey: "DATABASE_URL"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: appP.ID, Key: "DATABASE_URL", Value: "postgres://x"}); err != nil {
		t.Fatal(err)
	}
	f.do("DELETE", "/"+dbp.ID, nil).expect(t, 200)
	if n, _ := f.d.Store.CountLinks(ctx, store.LinkFilter{AppProjectID: &appP.ID}); n != 0 {
		t.Fatal("links must be removed")
	}
	key := "DATABASE_URL"
	if _, err := f.d.Store.FindEnvVariable(ctx, store.EnvFilter{ProjectID: &appP.ID, Key: &key}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("injected env must be removed from the app")
	}
}

func TestServiceDeleteProjectErrors(t *testing.T) {
	f := newFixture(t, Options{})
	s := NewService(f.d)
	if err := s.DeleteProject(context.Background(), missingID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err %v", err)
	}
	p := f.project(store.ProjectCreate{})
	f.engine.pingErr = unavailable()
	if err := s.DeleteProject(context.Background(), p.ID); !errors.Is(err, ErrDockerUnavailable) {
		t.Fatalf("err %v", err)
	}
}
