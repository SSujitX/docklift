package backup

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/db"
	"docklift/internal/store"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// fakeDocker records the restore's Docker calls.
type fakeDocker struct {
	mu         sync.Mutex
	running    []string
	stopped    []string
	volumes    []string
	ups        []string
	proxies    []string
	reloads    int
	composeErr error
	reloadErr  error
}

func (f *fakeDocker) RunningUserContainers(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.running...), nil
}

func (f *fakeDocker) StopContainers(_ context.Context, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, ids...)
	return nil
}

func (f *fakeDocker) VolumeCreate(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.volumes = append(f.volumes, name)
	return nil
}

func (f *fakeDocker) ComposeUp(_ context.Context, file, project, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.composeErr != nil {
		return f.composeErr
	}
	f.ups = append(f.ups, project+"|"+filepath.Base(filepath.Dir(file)))
	return nil
}

func (f *fakeDocker) ConnectProxy(_ context.Context, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proxies = append(f.proxies, projectID)
	return nil
}

func (f *fakeDocker) ReloadProxy(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reloads++
	return f.reloadErr
}

type fakeReloader struct{ err error }

func (f *fakeReloader) Reload(context.Context) error { return f.err }

// env is one Docklift installation on a temporary directory.
type env struct {
	t      *testing.T
	d      *app.Deps
	paths  Paths
	docker *fakeDocker
	nginx  *fakeReloader
	r      *Restorer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	d.Config.GitHubPrivateKeyPath = filepath.Join(filepath.Dir(d.Config.DataPath), "github-app.pem")
	paths := PathsFrom(d.Config)
	e := &env{t: t, d: d, paths: paths, docker: &fakeDocker{}, nginx: &fakeReloader{}}
	e.r = &Restorer{
		Paths: paths, DB: d.DB, Store: d.Store, State: d.Maintenance,
		Docker: e.docker, Nginx: e.nginx, Setup: d.Auth.Bootstrap(), Log: quiet(),
	}
	return e
}

func (e *env) write(rel, body string) {
	e.t.Helper()
	p := filepath.Join(filepath.Dir(e.paths.DataPath), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// symlinksSupported reports whether this process can create symlinks
// (Windows needs developer mode or privileges).
func symlinksSupported(t *testing.T) bool {
	dir := t.TempDir()
	return os.Symlink("x", filepath.Join(dir, "l")) == nil
}

// seed fills the installation: an admin, a project with a runtime compose
// file and a volume, nginx confs, certificates (with certbot's relative
// live/ links when the OS allows) and a GitHub key.
func (e *env) seed(tag string) *store.Project {
	e.t.Helper()
	ctx := context.Background()
	apptest.CreateAdmin(e.t, e.d, tag+"@example.test", "Passw0rd!"+tag)
	p, err := e.d.Store.CreateProject(ctx, store.ProjectCreate{Name: tag + "-app"})
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.d.Store.CreateVolume(ctx, store.VolumeCreate{ProjectID: p.ID, ServiceName: "app", Name: "dl-vol-" + tag, DisplayName: "data", MountPath: "/data"}); err != nil {
		e.t.Fatal(err)
	}
	e.write("deployments/"+p.ID+"/Dockerfile", "FROM alpine # "+tag+"\n")
	e.write("deployments/"+p.ID+"/src/main.txt", "main "+tag+"\n")
	e.write("deployments/.docklift/"+p.ID+"/compose.yml", "services: {}\n# "+tag+"\n")
	e.write("deployments/.gitkeep", "")
	if err := os.MkdirAll(filepath.Join(e.paths.DeploymentsPath, p.ID, "empty-dir"), 0o755); err != nil {
		e.t.Fatal(err)
	}
	e.write("nginx/service-"+tag+".conf", "# conf "+tag+"\n")
	e.write("letsencrypt/archive/"+tag+".test/cert1.pem", "CERT "+tag+"\n")
	if symlinksSupported(e.t) {
		live := filepath.Join(e.paths.LetsEncryptPath, "live", tag+".test")
		if err := os.MkdirAll(live, 0o755); err != nil {
			e.t.Fatal(err)
		}
		if err := os.Symlink(filepath.FromSlash("../../archive/"+tag+".test/cert1.pem"), filepath.Join(live, "cert.pem")); err != nil {
			e.t.Fatal(err)
		}
	}
	e.write("github-app.pem", "KEY "+tag+"\n")
	return p
}

// backup creates an archive of the installation and returns its path.
func (e *env) backup() string {
	e.t.Helper()
	var log strings.Builder
	c := &Creator{Paths: e.paths, DB: e.d.DB, Log: quiet()}
	name := c.Create(context.Background(), "test", func(s string) { log.WriteString(s) })
	if name == "" {
		e.t.Fatalf("backup failed:\n%s", log.String())
	}
	return filepath.Join(e.paths.BackupDir, name)
}

// restore runs a restore of archive and returns the outcome and log.
func (e *env) restore(archive string, req Request) (Outcome, string) {
	e.t.Helper()
	req.ArchivePath = archive
	if req.Filename == "" {
		req.Filename = filepath.Base(archive)
	}
	var log strings.Builder
	out := e.r.Run(context.Background(), req, func(s string) { log.WriteString(s) })
	return out, log.String()
}

// admins returns the emails of the admins in the live database.
func (e *env) admins() []string {
	e.t.Helper()
	rows, err := e.d.DB.Reader().Query(`SELECT "email" FROM "users" WHERE "role" = 'admin' ORDER BY "email"`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// tree lists every entry below dir (relative, forward slashes): files with
// their content, directories with a trailing slash, links as "-> target".
// Restore work directories are ignored.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipAll
			}
			return err
		}
		if p == dir {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, dir+string(filepath.Separator)))
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			out[rel] = "-> " + filepath.ToSlash(target)
		case d.IsDir():
			out[rel+"/"] = ""
		default:
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[rel] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	var diffs []string
	for k, v := range want {
		if g, ok := got[k]; !ok {
			diffs = append(diffs, "missing "+k)
		} else if g != v {
			diffs = append(diffs, "differs "+k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			diffs = append(diffs, "extra "+k)
		}
	}
	sort.Strings(diffs)
	if len(diffs) > 0 {
		t.Fatalf("%s trees differ:\n  %s", label, strings.Join(diffs, "\n  "))
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustContain(t *testing.T, log string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(log, p) {
			t.Fatalf("log lacks %q:\n%s", p, log)
		}
	}
}

func noFile(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s must not exist (err=%v)", p, err)
	}
}

// openDB opens a database file read-only for assertions.
func openDB(t *testing.T, p string) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
