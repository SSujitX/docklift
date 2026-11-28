package nginx

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"docklift/internal/certs"
	"docklift/internal/dnscheck"
	"docklift/internal/store"
)

// fakeDocker scripts `docker exec` results: nginx -t, nginx -s reload and
// certbot.
type fakeDocker struct {
	mu      sync.Mutex
	calls   []string
	test    func() (certs.ExecResult, error)
	reload  func() (certs.ExecResult, error)
	certbot func(args []string) (certs.ExecResult, error)
}

func (f *fakeDocker) Run(_ context.Context, name string, args ...string) (certs.ExecResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	test, reload, certbot := f.test, f.reload, f.certbot
	f.mu.Unlock()
	switch {
	case slices.Contains(args, "-t"):
		if test != nil {
			return test()
		}
	case slices.Contains(args, "reload"):
		if reload != nil {
			return reload()
		}
	case slices.Contains(args, "certbot"):
		if certbot != nil {
			return certbot(args)
		}
	}
	return certs.ExecResult{}, nil
}

func (f *fakeDocker) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeDocker) count(sub string) int {
	n := 0
	for _, c := range f.Calls() {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}

// noDocker behaves like a host without Docker: every exec fails to start.
func noDocker() (certs.ExecResult, error) {
	return certs.ExecResult{ExitCode: -1}, &os.PathError{Op: "exec", Path: "docker", Err: os.ErrNotExist}
}

func invalidConf(msg string) func() (certs.ExecResult, error) {
	return func() (certs.ExecResult, error) {
		return certs.ExecResult{ExitCode: 1, Stderr: "nginx: [emerg] " + msg + "\nnginx: configuration file /etc/nginx/nginx.conf test failed\n"}, nil
	}
}

type memServices struct {
	mu   sync.Mutex
	rows []*store.Service
}

func (m *memServices) ListServices(_ context.Context, f store.ServiceFilter, _ store.Order) ([]*store.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*store.Service
	for _, s := range m.rows {
		if f.HasDomain != nil && (s.Domain != nil) != *f.HasDomain {
			continue
		}
		if f.HasContainerName != nil && (s.ContainerName != nil) != *f.HasContainerName {
			continue
		}
		if f.StatusIn != nil && (s.Status == nil || !slices.Contains(f.StatusIn, *s.Status)) {
			continue
		}
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}

type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSettings) GetSetting(_ context.Context, k string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	return v, ok, nil
}
func (s *memSettings) SetSetting(_ context.Context, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}
func (s *memSettings) DeleteSetting(_ context.Context, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}
func (s *memSettings) OldestUser(context.Context) (*store.User, error) {
	return &store.User{Email: "admin@example.com"}, nil
}

type okDNS struct{}

func (okDNS) CheckDomain(_ context.Context, h string) dnscheck.Check {
	return dnscheck.Check{Domain: h, Status: dnscheck.StatusOK, A: []string{}, AAAA: []string{}, Message: h + " ok"}
}

type env struct {
	dir      string
	confDir  string
	le       string
	docker   *fakeDocker
	services *memServices
	settings *memSettings
	certs    *certs.Service
	nginx    *Service
}

func newEnv(t testing.TB) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{
		dir:      dir,
		confDir:  filepath.Join(dir, "conf.d"),
		le:       filepath.Join(dir, "letsencrypt"),
		docker:   &fakeDocker{},
		services: &memServices{},
		settings: &memSettings{m: map[string]string{}},
	}
	e.certs = certs.New(certs.Settings{LetsEncryptPath: e.le, DataPath: filepath.Join(dir, "data"), CertbotContainer: "docklift-certbot"},
		e.settings, okDNS{}, nil, certs.WithRunner(e.docker))
	e.nginx = New(Options{ConfDir: e.confDir, ProxyContainer: "docklift-nginx-proxy", BackendPort: 8000, Runner: e.docker}, e.certs, e.services)
	return e
}

func fixturePEM(t testing.TB, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "certs", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func writeLive(t testing.TB, root, name, fullchain string) {
	t.Helper()
	live := filepath.Join(root, "live", name)
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "fullchain.pem"), []byte(fullchain), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "privkey.pem"), []byte("-----BEGIN PRIVATE KEY-----\nPLACEHOLDER\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// coveringCerts installs live/a (SANs a+b) and live/b (orphan, b only).
func coveringCerts(t testing.TB, le string) {
	writeLive(t, le, "a.example.com", fixturePEM(t, "covering-a-b.pem"))
	writeLive(t, le, "b.example.com", fixturePEM(t, "orphan-b.pem"))
}

func readFile(t testing.TB, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func ptr[T any](v T) *T { return &v }

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
