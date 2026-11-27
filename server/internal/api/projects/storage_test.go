package projects

import (
	"context"
	"strings"
	"testing"

	"docklift/internal/deploy"
	"docklift/internal/execx"
	"docklift/internal/store"
)

func TestStorage(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	p := f.project(store.ProjectCreate{Name: "Store Me"})
	f.service(p.ID, "app", nil)
	base := "/" + p.ID + "/storage"

	if got := f.do("GET", base, nil).arr(t); len(got) != 0 {
		t.Fatalf("list %v", got)
	}
	if got := f.do("GET", "/"+missingID+"/storage", nil).arr(t); len(got) != 0 {
		t.Fatalf("missing list %v", got)
	}
	f.do("POST", "/"+missingID+"/storage", map[string]any{"name": "data"}).expectError(t, 404, msgProjectNotFound)
	for _, bad := range []any{nil, "", "-data", "da ta", strings.Repeat("a", 33), "dätä"} {
		f.do("POST", base, map[string]any{"name": bad, "service_name": "app", "mount_path": "/data"}).
			expectError(t, 400, "Storage name must use 1-32 letters, numbers, _ or -")
	}
	f.do("POST", base, map[string]any{"name": "data", "service_name": "web", "mount_path": "/data"}).expectError(t, 400, "Select a valid project service")
	f.do("POST", base, map[string]any{"name": "data", "service_name": "app", "mount_path": "/etc/x"}).expectError(t, 400, "This system mount path is not allowed")
	f.do("POST", base, map[string]any{"name": "data", "service_name": "app", "mount_path": "rel"}).expectError(t, 400, "Mount path must be an absolute container path")

	created := f.do("POST", base, map[string]any{"name": " data ", "service_name": " app ", "mount_path": "/data/"}).expect(t, 201)
	volName := deploy.PersistentVolumeName(p.ID, "data")
	if created["name"] != volName || created["display_name"] != "data" || created["mount_path"] != "/data" || created["service_name"] != "app" {
		t.Fatalf("created %v", created)
	}
	want := "volume create --label com.docker.compose.project=" + deploy.ComposeProjectName(p.Name, p.ID) + " --label com.docklift.project=" + p.ID + " " + volName
	if args := f.run.Args(); len(args) != 1 || args[0] != want {
		t.Fatalf("docker calls %v\nwant %q", args, want)
	}
	f.do("POST", base, map[string]any{"name": "data", "service_name": "app", "mount_path": "/other"}).
		expectError(t, 409, "That storage name or mount path already exists")
	f.do("POST", base, map[string]any{"name": "data2", "service_name": "app", "mount_path": "/data"}).
		expectError(t, 409, "That storage name or mount path already exists")

	// docker volume create fails: its stderr is returned and the row removed.
	f.run.answer = func(args string) (execx.Result, error) {
		return execx.Result{Stderr: []byte("  disk full \n"), ExitCode: 1}, exitErr(1)
	}
	f.do("POST", base, map[string]any{"name": "cache", "service_name": "app", "mount_path": "/cache"}).expectError(t, 500, "disk full")
	f.run.answer = func(args string) (execx.Result, error) { return execx.Result{ExitCode: 1}, exitErr(1) }
	f.do("POST", base, map[string]any{"name": "cache", "service_name": "app", "mount_path": "/cache"}).expectError(t, 500, "Failed to create Docker volume")
	if vols, _ := f.d.Store.ListVolumes(ctx, p.ID, store.OrderNone); len(vols) != 1 {
		t.Fatalf("failed creates must not keep rows: %d", len(vols))
	}

	// Delete.
	vid := created["id"].(string)
	f.do("DELETE", base+"/"+vid, nil).expectError(t, 400, "Confirm permanent data deletion with removeVolume=true")
	f.do("DELETE", base+"/"+vid+"?removeVolume=1", nil).expectError(t, 400, "Confirm permanent data deletion with removeVolume=true")
	f.do("DELETE", base+"/"+vid+"?removeVolume=true&removeVolume=true", nil).expectError(t, 400, "Confirm permanent data deletion with removeVolume=true")
	f.do("DELETE", base+"/nope?removeVolume=true", nil).expectError(t, 404, "Persistent storage not found")
	f.do("DELETE", "/"+missingID+"/storage/"+vid+"?removeVolume=true", nil).expectError(t, 404, "Persistent storage not found")
	f.run.answer = func(args string) (execx.Result, error) {
		return execx.Result{Stderr: []byte("volume is in use\n"), ExitCode: 1}, exitErr(1)
	}
	f.do("DELETE", base+"/"+vid+"?removeVolume=true", nil).expectError(t, 409, "volume is in use")
	f.run.answer = func(args string) (execx.Result, error) { return execx.Result{ExitCode: 1}, exitErr(1) }
	f.do("DELETE", base+"/"+vid+"?removeVolume=true", nil).expectError(t, 409, "Stop the project before deleting storage")
	f.run.answer = nil
	if got := f.do("DELETE", base+"/"+vid+"?removeVolume=true", nil).expect(t, 200); got["status"] != "deleted" {
		t.Fatalf("delete %v", got)
	}
	if vols, _ := f.d.Store.ListVolumes(ctx, p.ID, store.OrderNone); len(vols) != 0 {
		t.Fatal("row must be deleted")
	}
}

func TestStorageDockerUnavailable(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	p := f.project(store.ProjectCreate{})
	f.service(p.ID, "app", nil)
	vol, err := f.d.Store.CreateVolume(ctx, store.VolumeCreate{ProjectID: p.ID, ServiceName: "app", Name: "v", DisplayName: "v", MountPath: "/v"})
	if err != nil {
		t.Fatal(err)
	}
	f.engine.pingErr = unavailable()
	f.do("POST", "/"+p.ID+"/storage", map[string]any{"name": "data", "service_name": "app", "mount_path": "/data"}).expectError(t, 503, msgDockerDown)
	f.do("DELETE", "/"+p.ID+"/storage/"+vol.ID+"?removeVolume=true", nil).expectError(t, 503, msgDockerDown)
	if vols, _ := f.d.Store.ListVolumes(ctx, p.ID, store.OrderNone); len(vols) != 1 {
		t.Fatal("rows must be untouched")
	}
	if len(f.run.Args()) != 0 {
		t.Fatalf("no docker CLI call: %v", f.run.Args())
	}
}
