package nginx

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"docklift/internal/certs"
	"docklift/internal/store"
)

// These tests run nginx -t and reloads against a real nginx container with
// the repository's nginx-proxy config and generated confs. They need a Linux
// Docker host and DOCKLIFT_DOCKER=1. No certificate is requested from Let's
// Encrypt: certbot is never run (SkipIssue), certificates are self-signed.

func requireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("DOCKLIFT_DOCKER") != "1" {
		t.Skip("set DOCKLIFT_DOCKER=1 to run against a real nginx container")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	raw := readFile(t, src)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSelfSigned installs a real key pair as live/<name>/.
func writeSelfSigned(t *testing.T, le, name string, hosts []string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: hosts[0]},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour), DNSNames: hosts,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(le, "live", name)
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(live, "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	_ = os.WriteFile(filepath.Join(live, "privkey.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o644)
}

// startNginx runs the nginx image pinned in docker-compose.yml, like the
// docklift-nginx-proxy service.
func startNginx(t *testing.T, e *env) string {
	t.Helper()
	repo := filepath.Join("..", "..", "..", "nginx-proxy")
	copyFile(t, filepath.Join(repo, "nginx.conf"), filepath.Join(e.dir, "nginx.conf"))
	copyFile(t, filepath.Join(repo, "snippets", "acme.conf"), filepath.Join(e.dir, "snippets", "acme.conf"))
	copyFile(t, filepath.Join(repo, "conf.d", "default.conf"), filepath.Join(e.confDir, "default.conf"))
	_ = os.MkdirAll(filepath.Join(e.dir, "www"), 0o755)
	_ = os.MkdirAll(e.le, 0o755)

	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	name := "docklift-it-nginx-" + hex.EncodeToString(suffix)
	abs := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	docker(t, "run", "-d", "--name", name, "--add-host", "host.docker.internal:host-gateway",
		"-v", abs(e.confDir)+":/etc/nginx/conf.d:ro",
		"-v", abs(filepath.Join(e.dir, "nginx.conf"))+":/etc/nginx/nginx.conf:ro",
		"-v", abs(filepath.Join(e.dir, "snippets"))+":/etc/nginx/snippets:ro",
		"-v", abs(filepath.Join(e.dir, "www"))+":/var/www/certbot:ro",
		"-v", abs(e.le)+":/etc/letsencrypt:ro",
		"nginx:1.30.8-alpine")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() })

	deadline := time.Now().Add(30 * time.Second)
	for {
		if exec.Command("docker", "exec", name, "nginx", "-t").Run() == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("nginx did not start:\n%s", docker(t, "logs", name))
		}
		time.Sleep(200 * time.Millisecond)
	}
	return name
}

func TestDockerNginxAcceptsGeneratedConfs(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()
	e := newEnv(t)
	writeSelfSigned(t, e.le, "a.example.com", []string{"a.example.com", "b.example.com"})
	name := startNginx(t, e)
	e.nginx = New(Options{ConfDir: e.confDir, ProxyContainer: name, BackendPort: 8000, Runner: certs.ExecRunner{}}, e.certs, e.services)

	services := []store.Service{
		{ID: svcID, Name: "active", ContainerName: ptr("docklift-shop-web"), InternalPort: ptr(3000), Domain: ptr("a.example.com, b.example.com"), Status: ptr("running")},
		{ID: "22222222-2222-2222-2222-222222222222", Name: "partial", ContainerName: ptr("docklift-api"), InternalPort: ptr(8080), Domain: ptr("a.example.com, c.example.com"), Status: ptr("running")},
		{ID: "33333333-3333-3333-3333-333333333333", Name: "http only", ContainerName: ptr("docklift-blog"), InternalPort: ptr(80), Domain: ptr("blog.example.com"), Status: ptr("starting")},
	}
	for _, svc := range services {
		if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{SkipIssue: true}); err != nil {
			t.Fatalf("%s: %v", svc.Name, err)
		}
	}
	for _, p := range []struct {
		domain string
		port   int
		https  bool
	}{{"panel.example.com", 8080, false}, {"a.example.com", 8080, true}, {"x.example.com", 3000, false}} {
		if err := e.nginx.WritePanelDomainConfig(ctx, p.domain, p.port, p.https); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.nginx.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	dump := docker(t, "exec", name, "nginx", "-T")
	for _, want := range []string{"# docklift-ssl: active primary=a.example.com", "# docklift-ssl: partial primary=a.example.com", "set $dashboard docklift-backend.docklift_network;"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("nginx -T lacks %q", want)
		}
	}

	// A rejected conf is rolled back and nginx keeps reloading.
	bad := filepath.Join(e.confDir, "broken.example.com.conf")
	err := e.nginx.reloader.ApplyConf(ctx, bad, "server { bogus_directive on; }\n")
	if !IsReloadError(err) {
		t.Fatalf("invalid conf accepted: %v", err)
	}
	if _, statErr := os.Stat(bad); !os.IsNotExist(statErr) {
		t.Fatal("rejected conf left on disk")
	}
	if err := e.nginx.Reload(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDockerPanelMigration(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()
	e := newEnv(t)
	writeSelfSigned(t, e.le, "a.example.com", []string{"a.example.com"})
	_ = os.MkdirAll(e.confDir, 0o755)
	copyFile(t, goldenPath("panel-dashboard-active"), filepath.Join(e.confDir, "a.example.com.conf"))
	copyFile(t, goldenPath("panel-dashboard-http-only"), filepath.Join(e.confDir, "panel.example.com.conf"))
	name := startNginx(t, e)
	e.nginx = New(Options{ConfDir: e.confDir, ProxyContainer: name, BackendPort: 8000, Runner: certs.ExecRunner{}}, e.certs, e.services)

	changed, err := e.nginx.MigratePanelUpstreams(ctx)
	if err != nil || len(changed) != 2 {
		t.Fatal(changed, err)
	}
	if dump := docker(t, "exec", name, "nginx", "-T"); strings.Contains(dump, "docklift-nginx;") {
		t.Fatal("old upstream still loaded")
	}
	if changed, err := e.nginx.MigratePanelUpstreams(ctx); err != nil || len(changed) != 0 {
		t.Fatal(changed, err)
	}
}
