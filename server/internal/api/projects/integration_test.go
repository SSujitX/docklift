package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"docklift/internal/app/apptest"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/httpx"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// Integration tests against a real Docker daemon (with compose). They run
// only with DOCKLIFT_DOCKER=1 and remove everything they create.

const itImage = "alpine:3.20"

func dockerFixture(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("DOCKLIFT_DOCKER") != "1" {
		t.Skip("set DOCKLIFT_DOCKER=1 to run Docker integration tests")
	}
	engine, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	d := apptest.NewDeps(t)
	d.DockerX.Engine = engine
	d.Edge.Nginx = nginx.New(nginx.Options{ConfDir: d.Config.NginxConfPath, ProxyContainer: d.Config.NginxProxyContainer,
		BackendPort: d.Config.Port, Runner: nginxRunner{}, Log: d.Log}, d.Edge.Certs, d.Store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	if err := d.DockerX.Ping(ctx); err != nil {
		t.Fatalf("docker not reachable: %v", err)
	}
	if err := d.DockerX.PullImage(ctx, itImage, dockerx.PullOptions{}); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	MountWith(r, d, Options{})
	return &fixture{t: t, d: d, h: httpx.ParseBody(r)}
}

func TestIntegrationDeleteTearsDownCompose(t *testing.T) {
	f := dockerFixture(t)
	ctx := context.Background()
	id := f.createUpload("it-delete")["id"].(string)
	p, err := f.d.Store.GetProject(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	primary := deploy.ComposeProjectName(p.Name, p.ID)
	compose := f.d.Deploy.ComposePath(id)
	if err := os.MkdirAll(filepath.Dir(compose), 0o755); err != nil {
		t.Fatal(err)
	}
	// /anon is an anonymous volume, like an image VOLUME nothing is mounted on.
	spec := "services:\n  app:\n    image: " + itImage + "\n    command: [\"sleep\", \"300\"]\n    volumes: [\"/anon\"]\n"
	if err := os.WriteFile(compose, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = execx.Run(context.Background(), execx.Cmd{Name: "docker", Args: []string{"compose", "-p", primary, "down", "--remove-orphans"}, Dir: os.TempDir(), Timeout: time.Minute})
	})
	res, err := f.d.DockerX.Compose(ctx, dockerx.ComposeCmd{Project: primary, Files: []string{compose}, Dir: f.d.Deploy.ProjectPath(id), Args: []string{"up", "-d"}, Timeout: 2 * time.Minute})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("compose up: %v %s", err, res.Stderr)
	}
	f.service(id, "app", nil)
	vol := f.do("POST", "/"+id+"/storage", map[string]any{"name": "data", "service_name": "app", "mount_path": "/data"}).expect(t, 201)
	volName := vol["name"].(string)
	t.Cleanup(func() { _, _ = f.d.DockerX.VolumeRemove(context.Background(), volName) })
	cid, _, _, err := execx.Output(ctx, execx.Cmd{Name: "docker", Args: []string{"ps", "-q", "--filter", "label=com.docker.compose.project=" + primary}, Timeout: 30 * time.Second})
	if err != nil || strings.TrimSpace(cid) == "" {
		t.Fatalf("project container: %q %v", cid, err)
	}
	anon, _, _, err := execx.Output(ctx, execx.Cmd{Name: "docker", Args: []string{"inspect", "-f", `{{range .Mounts}}{{if eq .Destination "/anon"}}{{.Name}}{{end}}{{end}}`, strings.TrimSpace(cid)}, Timeout: 30 * time.Second})
	anon = strings.TrimSpace(anon)
	if err != nil || anon == "" {
		t.Fatalf("anonymous volume of the container: %q %v", anon, err)
	}
	t.Cleanup(func() { _, _ = f.d.DockerX.VolumeRemove(context.Background(), anon) })

	f.do("DELETE", "/"+id, nil).expect(t, 200)

	// The container's anonymous volume goes with it (2.x leaked these).
	if _, err := execx.Run(ctx, execx.Cmd{Name: "docker", Args: []string{"volume", "inspect", anon}, Timeout: 30 * time.Second}); err == nil {
		t.Fatalf("anonymous volume %s must be removed", anon)
	}
	if gone := f.d.DockerX.ProbeComposeProject(ctx, primary); !gone.Gone() {
		t.Fatalf("compose resources left: %+v", gone)
	}
	if _, err := execx.Run(ctx, execx.Cmd{Name: "docker", Args: []string{"volume", "inspect", volName}, Timeout: 30 * time.Second}); err == nil {
		t.Fatalf("volume %s must be removed", volName)
	}
	if _, err := f.d.Store.GetProject(ctx, id); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("row must be deleted")
	}
}

func TestIntegrationStorageVolume(t *testing.T) {
	f := dockerFixture(t)
	ctx := context.Background()
	p := f.project(store.ProjectCreate{Name: "it-storage"})
	f.service(p.ID, "app", nil)
	vol := f.do("POST", "/"+p.ID+"/storage", map[string]any{"name": "cache", "service_name": "app", "mount_path": "/cache"}).expect(t, 201)
	volName := vol["name"].(string)
	t.Cleanup(func() { _, _ = f.d.DockerX.VolumeRemove(context.Background(), volName) })
	out, _, _, err := execx.Output(ctx, execx.Cmd{Name: "docker", Args: []string{"volume", "inspect", "--format", "{{index .Labels \"com.docklift.project\"}}", volName}, Timeout: 30 * time.Second})
	if err != nil || strings.TrimSpace(out) != p.ID {
		t.Fatalf("volume inspect: %q %v", out, err)
	}
	f.do("DELETE", "/"+p.ID+"/storage/"+vol["id"].(string)+"?removeVolume=true", nil).expect(t, 200)
	if _, err := execx.Run(ctx, execx.Cmd{Name: "docker", Args: []string{"volume", "inspect", volName}, Timeout: 30 * time.Second}); err == nil {
		t.Fatal("volume must be removed")
	}
}
