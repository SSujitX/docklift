package domains

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/auth"
	"docklift/internal/certs"
	"docklift/internal/dnscheck"
	"docklift/internal/httpx"
	"docklift/internal/nginx"
	"docklift/internal/panelaccess"
)

// fakeDocker answers every `docker exec` the nginx and certs services run.
type fakeDocker struct {
	mu          sync.Mutex
	calls       [][]string
	reloadFails bool
	certbotFn   func()
}

func (f *fakeDocker) Run(_ context.Context, name string, args ...string) (certs.ExecResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	reloadFails := f.reloadFails
	fn := f.certbotFn
	f.mu.Unlock()
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "nginx -s reload") && reloadFails:
		return certs.ExecResult{ExitCode: 1, Stderr: "nginx: boom"}, nil
	case strings.Contains(joined, "certbot") && fn != nil:
		fn()
	case strings.Contains(joined, "certbot"):
		return certs.ExecResult{ExitCode: 1, Stderr: "certbot refused"}, nil
	}
	return certs.ExecResult{}, nil
}

func (f *fakeDocker) setReloadFails(v bool) {
	f.mu.Lock()
	f.reloadFails = v
	f.mu.Unlock()
}

func (f *fakeDocker) setCertbot(fn func()) {
	f.mu.Lock()
	f.certbotFn = fn
	f.mu.Unlock()
}

func (f *fakeDocker) count(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), substr) {
			n++
		}
	}
	return n
}

type fakeResolver struct{ ips map[string]string }

func (r fakeResolver) LookupNetIP(_ context.Context, network, host string) ([]netip.Addr, error) {
	if network == "ip6" {
		return nil, nil
	}
	if ip, ok := r.ips[host]; ok {
		return []netip.Addr{netip.MustParseAddr(ip)}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

type fixture struct {
	d      *app.Deps
	h      http.Handler
	docker *fakeDocker
	nginx  string
	le     string
}

const serverIP = "203.0.113.9"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := apptest.NewDeps(t)
	docker := &fakeDocker{}
	ipSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ip":"`+serverIP+`"}`)
	}))
	t.Cleanup(ipSrv.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dns := dnscheck.New(
		dnscheck.WithResolver(fakeResolver{ips: map[string]string{"ok.example.com": serverIP, "other.example.com": "198.51.100.1", "retry.example.org": serverIP}}),
		dnscheck.WithIPSource(ipSrv.URL, ipSrv.Client()),
	)
	c := certs.New(certs.SettingsFrom(d.Config), d.Store, dns, log, certs.WithRunner(docker))
	n := nginx.New(nginx.Options{
		ConfDir:        d.Config.NginxConfPath,
		ProxyContainer: d.Config.NginxProxyContainer,
		BackendPort:    d.Config.Port,
		Runner:         docker,
		Log:            log,
	}, c, d.Store)
	d.Edge = &nginx.Stack{DNS: dns, Certs: c, Nginx: n}
	r := chi.NewRouter()
	r.Use(httpx.ParseBody)
	Mount(r, d)
	return &fixture{d: d, h: r, docker: docker, nginx: d.Config.NginxConfPath, le: d.Config.LetsEncryptPath}
}

// setEmail configures the ACME account email certbot runs need.
func (f *fixture) setEmail(t *testing.T) {
	t.Helper()
	if err := f.d.Edge.Certs.SetAcmeEmail(context.Background(), "acme@example.org"); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) do(t *testing.T, method, path string, body any) (int, map[string]any, []any) {
	t.Helper()
	var rd io.Reader
	if s, ok := body.(string); ok {
		rd = strings.NewReader(s)
	} else if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, path, rd)
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	var obj map[string]any
	var arr []any
	raw := rec.Body.Bytes()
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &arr); err != nil {
			t.Fatalf("%s %s: bad array %q", method, path, raw)
		}
	} else if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("%s %s: bad body %q", method, path, raw)
	}
	return rec.Code, obj, arr
}

func (f *fixture) expectError(t *testing.T, method, path string, body any, status int, msg string) {
	t.Helper()
	code, obj, _ := f.do(t, method, path, body)
	if code != status || obj["error"] != msg {
		t.Errorf("%s %s = %d %v, want %d %q", method, path, code, obj, status, msg)
	}
}

// installCert places a live/<name> lineage valid for the given days.
func (f *fixture) installCert(t *testing.T, name string, days int) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Duration(days) * 24 * time.Hour),
		DNSNames:     []string{name},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.le, "live", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) readConf(t *testing.T, domain string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.nginx, domain+".conf"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInvalidDomainsRejected(t *testing.T) {
	f := newFixture(t)
	for _, d := range []string{"localhost", "bad_domain.test", "-a.test", "a.test;b", "a..test", "a.test%0Ab", "..%2f..%2fetc.passwd"} {
		f.expectError(t, "GET", "/"+d+"/ssl", nil, 400, msgInvalidFormat)
		f.expectError(t, "POST", "/"+d+"/ssl/retry", nil, 400, msgInvalidFormat)
		f.expectError(t, "DELETE", "/"+d, nil, 400, msgInvalidFormat)
		if !strings.Contains(d, "%") {
			f.expectError(t, "POST", "/", map[string]any{"domain": d, "port": 8080}, 400, msgInvalidFormat)
		}
	}
	if n := f.docker.count(""); n != 0 {
		t.Errorf("docker was called %d times for invalid input", n)
	}
}

func TestAddValidation(t *testing.T) {
	f := newFixture(t)
	for _, body := range []any{
		map[string]any{}, map[string]any{"domain": "a.test"}, map[string]any{"port": 8080},
		map[string]any{"domain": "a.test", "port": "0"}, map[string]any{"domain": "a.test", "port": 65536},
		map[string]any{"domain": "a.test", "port": "80a"}, map[string]any{"domain": "a.test", "port": 80.5},
		map[string]any{"domain": "a.test", "port": -1}, map[string]any{"domain": "a.test", "port": true},
		map[string]any{"domain": "a.test", "port": []int{80}}, map[string]any{"domain": "   ", "port": 8080},
		map[string]any{"domain": nil, "port": 8080}, nil,
	} {
		f.expectError(t, "POST", "/", body, 400, "Invalid domain or port")
	}
	f.expectError(t, "POST", "/", map[string]any{"domain": 5, "port": 8080}, 400, msgInvalidFormat)
	f.expectError(t, "POST", "/", map[string]any{"domain": []string{"a.test"}, "port": 8080}, 400, msgInvalidFormat)
	f.expectError(t, "POST", "/", "{not json", 400, "Invalid JSON body")
}

func TestPortParsing(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{`8080`, 8080, true}, {`"8443"`, 8443, true}, {`" 9000 "`, 9000, true}, {`"08080"`, 8080, true},
		{`1`, 1, true}, {`65535`, 65535, true}, {`1e3`, 1000, true},
		{`0`, 0, false}, {`65536`, 0, false}, {`"0"`, 0, false}, {`80.5`, 0, false}, {`"80.5"`, 0, false},
		{`""`, 0, false}, {`"+80"`, 0, false}, {`"\u0663\u0663"`, 0, false}, {`null`, 0, false}, {`true`, 0, false},
		{`"99999999999999999999"`, 0, false},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"port":`+c.in+`}`))
		req.Header.Set("Content-Type", "application/json")
		var got int
		var ok bool
		httpx.ParseBody(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got, ok = parsePort(httpx.BodyFrom(r).Get("port"))
		})).ServeHTTP(httptest.NewRecorder(), req)
		if ok != c.ok || got != c.want {
			t.Errorf("parsePort(%s) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLifecycleReloadFailure(t *testing.T) {
	f := newFixture(t)
	f.docker.setReloadFails(true)
	host := "panel-abc.example.org"

	code, obj, _ := f.do(t, "POST", "/", map[string]any{"domain": "  PANEL-ABC.Example.org ", "port": "8443"})
	if code != 500 || obj["error"] != "Nginx reload failed: nginx: boom" {
		t.Fatalf("add = %d %v", code, obj)
	}
	conf := f.readConf(t, host)
	for _, want := range []string{"# docklift-panel-domain\n# docklift-panel-port: 8443\n", "server_name " + host + ";"} {
		if !strings.Contains(conf, want) {
			t.Errorf("conf missing %q:\n%s", want, conf)
		}
	}

	code, _, list := f.do(t, "GET", "/", nil)
	if code != 200 || len(list) != 1 {
		t.Fatalf("list = %d %v", code, list)
	}
	item := list[0].(map[string]any)
	ssl := item["ssl"].(map[string]any)
	if item["domain"] != host || item["port"] != float64(8443) || ssl["status"] != "missing" || ssl["expired"] != false || ssl["daysUntilExpiry"] != nil {
		t.Errorf("listed %v", item)
	}

	for _, path := range []string{"/" + host + "/ssl", "/PANEL-ABC.example.org/ssl"} {
		code, obj, _ = f.do(t, "GET", path, nil)
		if code != 200 || obj["ssl"].(map[string]any)["domain"] != host || len(obj["events"].([]any)) != 0 {
			t.Errorf("%s = %d %v", path, code, obj)
		}
	}

	f.expectError(t, "POST", "/", map[string]any{"domain": host, "port": 8080}, 409, `Hostname "`+host+`" is already used by a panel domain`)

	code, obj, _ = f.do(t, "POST", "/"+host+"/ssl/retry", nil)
	if code != 500 || !strings.HasPrefix(obj["error"].(string), "Nginx reload failed: ") {
		t.Errorf("retry = %d %v", code, obj)
	}

	f.expectError(t, "DELETE", "/"+host, nil, 500, "Failed to delete domain")
	f.expectError(t, "DELETE", "/"+host, nil, 404, msgNotFound)
	if _, _, list = f.do(t, "GET", "/", nil); len(list) != 0 {
		t.Errorf("deleted domain still listed: %v", list)
	}
}

func TestAddWithoutCertificateStaysHTTP(t *testing.T) {
	f := newFixture(t)
	host := "plain.example.org"
	f.setEmail(t)
	f.docker.setCertbot(nil)
	code, obj, _ := f.do(t, "POST", "/", map[string]any{"domain": host, "port": 8443})
	if code != 200 || obj["success"] != true || obj["domain"] != host || obj["port"] != float64(8443) {
		t.Fatalf("add = %d %v", code, obj)
	}
	ssl := obj["ssl"].(map[string]any)
	if ssl["status"] != "missing" || ssl["expired"] != false {
		t.Errorf("ssl = %v", ssl)
	}
	if conf := f.readConf(t, host); strings.Contains(conf, "listen 443") {
		t.Errorf("HTTP-only expected without a certificate:\n%s", conf)
	}
	if len(obj["events"].([]any)) == 0 {
		t.Error("events should narrate the attempt")
	}
	_, status, _ := f.do(t, "GET", "/"+host+"/ssl", nil)
	if len(status["events"].([]any)) == 0 || status["ssl"].(map[string]any)["status"] != "failed" {
		t.Errorf("status = %v", status)
	}
}

func TestAddWithCoveringCertificateEnablesHTTPS(t *testing.T) {
	f := newFixture(t)
	host := "secure.example.org"
	f.installCert(t, host, 60)
	code, obj, _ := f.do(t, "POST", "/", map[string]any{"domain": host, "port": 8080})
	if code != 200 {
		t.Fatalf("add = %d %v", code, obj)
	}
	ssl := obj["ssl"].(map[string]any)
	if ssl["status"] != "active" || ssl["expired"] != false || ssl["renewalOverdue"] != false {
		t.Errorf("ssl = %v", ssl)
	}
	if days, _ := ssl["daysUntilExpiry"].(float64); days < 58 || days > 60 {
		t.Errorf("daysUntilExpiry = %v", ssl["daysUntilExpiry"])
	}
	if conf := f.readConf(t, host); !strings.Contains(conf, "443") || !strings.Contains(conf, "live/"+host) {
		t.Errorf("HTTPS conf expected:\n%s", conf)
	}
	if f.docker.count("certbot") != 0 {
		t.Error("an existing covering certificate must be reused, not re-requested")
	}
}

func TestRetryForcesIssueAndKeepsPort(t *testing.T) {
	f := newFixture(t)
	host := "retry.example.org"
	f.setEmail(t)
	f.expectError(t, "POST", "/"+host+"/ssl/retry", nil, 404, msgNotFound)
	f.do(t, "POST", "/", map[string]any{"domain": host, "port": 9191})
	f.docker.setCertbot(func() { f.installCert(t, host, 80) })
	code, obj, _ := f.do(t, "POST", "/"+host+"/ssl/retry", nil)
	if code != 200 || obj["success"] != true || obj["domain"] != host {
		t.Fatalf("retry = %d %v", code, obj)
	}
	if _, has := obj["port"]; has {
		t.Error("retry response has no port key")
	}
	if obj["ssl"].(map[string]any)["status"] != "active" {
		t.Errorf("ssl = %v", obj["ssl"])
	}
	if conf := f.readConf(t, host); !strings.Contains(conf, "# docklift-panel-port: 9191\n") {
		t.Errorf("port lost:\n%s", conf)
	}
	if f.docker.count("--force-renewal") != 1 {
		t.Errorf("retry must force renewal; calls=%v", f.docker.calls)
	}
}

func TestStatusAndDeleteMissing(t *testing.T) {
	f := newFixture(t)
	f.expectError(t, "GET", "/nothere.example.org/ssl", nil, 404, msgNotFound)
	f.expectError(t, "DELETE", "/nothere.example.org", nil, 404, msgNotFound)
}

func TestDeleteSuccess(t *testing.T) {
	f := newFixture(t)
	host := "gone.example.org"
	f.do(t, "POST", "/", map[string]any{"domain": host, "port": 8080})
	code, obj, _ := f.do(t, "DELETE", "/"+host, nil)
	if code != 200 || obj["success"] != true || len(obj) != 1 {
		t.Fatalf("delete = %d %v", code, obj)
	}
	if _, err := os.Stat(filepath.Join(f.nginx, host+".conf")); !os.IsNotExist(err) {
		t.Errorf("conf still there: %v", err)
	}
	f.expectError(t, "GET", "/"+host+"/ssl", nil, 404, msgNotFound)
}

func TestListSkipsNonPanelConfs(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(f.nginx, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"default.conf":            "server_name default.example.org; # docklift-panel-domain",
		"service-abc.conf":        "server_name svc.example.org; # docklift-panel-domain",
		"notes.txt":               "docklift-panel-domain",
		"unrelated.conf":          "server_name unrelated.example.org;",
		"legacy.example.org.conf": "server_name legacy.example.org www.legacy.example.org;\nproxy_pass http://host.docker.internal:3001;",
		"noport.conf":             "# docklift-panel-domain\nserver_name noport.example.org;",
	} {
		if err := os.WriteFile(filepath.Join(f.nginx, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, _, list := f.do(t, "GET", "/", nil)
	if code != 200 || len(list) != 1 {
		t.Fatalf("list = %d %v", code, list)
	}
	item := list[0].(map[string]any)
	if item["domain"] != "legacy.example.org" || item["port"] != float64(3001) {
		t.Errorf("item = %v", item)
	}
}

func TestListEmpty(t *testing.T) {
	f := newFixture(t)
	code, _, list := f.do(t, "GET", "/", nil)
	if code != 200 || list == nil || len(list) != 0 {
		t.Errorf("list = %d %v", code, list)
	}
}

func TestDNSCheck(t *testing.T) {
	f := newFixture(t)
	for _, body := range []any{
		map[string]any{}, map[string]any{"domains": "a.test"}, map[string]any{"domains": []any{"bad domain", 5, nil, true, map[string]any{}}},
		map[string]any{"domains": []any{}}, nil,
	} {
		f.expectError(t, "POST", "/dns-check", body, 400, "Provide at least one domain")
	}
	code, obj, _ := f.do(t, "POST", "/dns-check", map[string]any{"domains": []any{"bad domain", " OK.Example.com ", "other.example.com", "gone.example.com", 7}})
	if code != 200 || obj["serverIp"] != serverIP {
		t.Fatalf("dns-check = %d %v", code, obj)
	}
	checks := obj["checks"].([]any)
	want := map[string]string{"ok.example.com": "ok", "other.example.com": "mismatch", "gone.example.com": "missing"}
	if len(checks) != 3 {
		t.Fatalf("checks = %v", checks)
	}
	for _, c := range checks {
		m := c.(map[string]any)
		if want[m["domain"].(string)] != m["status"] {
			t.Errorf("check %v", m)
		}
	}
}

func TestDNSCheckCapsAtTenHosts(t *testing.T) {
	f := newFixture(t)
	var hosts []any
	for i := range 15 {
		hosts = append(hosts, "h"+string(rune('a'+i))+".example.com")
	}
	_, obj, _ := f.do(t, "POST", "/dns-check", map[string]any{"domains": hosts})
	if n := len(obj["checks"].([]any)); n != 10 {
		t.Errorf("checks = %d, want 10", n)
	}
}

func TestACMEEmail(t *testing.T) {
	f := newFixture(t)
	code, obj, _ := f.do(t, "GET", "/ssl/email", nil)
	if code != 200 || obj["email"] != "" {
		t.Fatalf("initial = %d %v", code, obj)
	}
	for _, body := range []any{map[string]any{}, map[string]any{"email": nil}, map[string]any{"email": ""}, map[string]any{"email": "not-an-email"},
		map[string]any{"email": "   "}, map[string]any{"email": "a@b.test\nBcc: x@y.test"}, map[string]any{"email": strings.Repeat("a", 250) + "@b.test"}, nil} {
		f.expectError(t, "PUT", "/ssl/email", body, 400, "Invalid email")
	}
	for _, v := range []any{12, true, []string{"a@b.test"}, map[string]any{"x": 1}} {
		f.expectError(t, "PUT", "/ssl/email", map[string]any{"email": v}, 400, "Email must be a string")
	}
	code, obj, _ = f.do(t, "PUT", "/ssl/email", map[string]any{"email": "  Ops@Contract.Test "})
	if code != 200 || obj["success"] != true || obj["email"] != "ops@contract.test" {
		t.Fatalf("put = %d %v", code, obj)
	}
	if _, obj, _ = f.do(t, "GET", "/ssl/email", nil); obj["email"] != "ops@contract.test" {
		t.Errorf("get = %v", obj)
	}
}

func TestACMEEmailFallsBackToOldestUser(t *testing.T) {
	f := newFixture(t)
	apptest.CreateAdmin(t, f.d, "root@example.org", "correct horse battery")
	if _, obj, _ := f.do(t, "GET", "/ssl/email", nil); obj["email"] != "root@example.org" {
		t.Errorf("get = %v", obj)
	}
}

func TestLockReleasesEntries(t *testing.T) {
	f := newFixture(t)
	h := &handlers{d: f.d, locks: map[string]*domainLock{}}
	unlock := h.lock("a.example.org")
	unlock()
	if len(h.locks) != 0 {
		t.Errorf("locks leaked: %v", h.locks)
	}
}

// accessRequest says where a request comes from: the proxy (trusted peer
// with X-Forwarded-Proto/-Host) or directly.
type accessRequest struct {
	viaProxy bool
	proto    string // X-Forwarded-Proto when viaProxy
	host     string // X-Forwarded-Host when viaProxy
}

// doAs is do for the signed-in admin userID, from the given place.
func (f *fixture) doAs(t *testing.T, userID string, from accessRequest, method, path string, body any) (int, map[string]any, []any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = serverIP + ":8080"
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if from.viaProxy {
		req.Header.Set("X-Forwarded-Proto", from.proto)
		req.Header.Set("X-Forwarded-Host", from.host)
	} else {
		// A direct client may send anything; it is not believed.
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Host", "panel.example.org")
	}
	ctx := httpx.WithPeerTrusted(req.Context(), from.viaProxy)
	req = req.WithContext(auth.WithUser(ctx, auth.User{UserID: userID, Role: "admin"}))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	var obj map[string]any
	var arr []any
	raw := rec.Body.Bytes()
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &arr); err != nil {
			t.Fatalf("%s %s: bad array %q", method, path, raw)
		}
	} else if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("%s %s: bad body %q", method, path, raw)
	}
	return rec.Code, obj, arr
}

// listedAccess is the access object of host in GET /.
func (f *fixture) listedAccess(t *testing.T, userID string, from accessRequest, host string) map[string]any {
	t.Helper()
	code, _, list := f.doAs(t, userID, from, "GET", "/", nil)
	if code != 200 {
		t.Fatalf("list = %d", code)
	}
	for _, item := range list {
		if m := item.(map[string]any); m["domain"] == host {
			return m["access"].(map[string]any)
		}
	}
	t.Fatalf("%s not listed: %v", host, list)
	return nil
}

func (f *fixture) domainOnlyRow(t *testing.T) (string, bool) {
	t.Helper()
	v, ok, err := f.d.Store.GetSetting(context.Background(), panelaccess.SettingKey)
	if err != nil {
		t.Fatal(err)
	}
	return v, ok
}

func TestPanelAccess(t *testing.T) {
	f := newFixture(t)
	u, _ := apptest.CreateAdmin(t, f.d, "ada@example.com", "password123")
	const host = "panel.example.org"
	f.installCert(t, host, 60)
	if code, obj, _ := f.do(t, "POST", "/", map[string]any{"domain": host, "port": 8080}); code != 200 {
		t.Fatalf("add = %d %v", code, obj)
	}
	f.do(t, "POST", "/", map[string]any{"domain": "plain.example.org", "port": 8080})
	f.installCert(t, "app.example.org", 60)
	f.do(t, "POST", "/", map[string]any{"domain": "app.example.org", "port": 3000})

	direct := accessRequest{}
	via := accessRequest{viaProxy: true, proto: "https", host: host}
	open := "Open the dashboard at https://" + host + " first, then turn this on"
	noCert := "https://plain.example.org has no certificate yet. Wait until HTTPS is active"
	otherPort := "This domain points to another port, not the Docklift dashboard"
	on := map[string]any{"domainOnly": true, "password": "password123"}

	t.Run("status in the list", func(t *testing.T) {
		if got := f.listedAccess(t, u.ID, direct, host); got["domainOnly"] != false || got["canEnable"] != false || got["reason"] != open {
			t.Errorf("direct = %v", got)
		}
		if got := f.listedAccess(t, u.ID, via, host); got["domainOnly"] != false || got["canEnable"] != true || len(got) != 2 {
			t.Errorf("through the domain = %v", got)
		}
		if got := f.listedAccess(t, u.ID, via, "plain.example.org"); got["canEnable"] != false || got["reason"] != noCert {
			t.Errorf("no certificate = %v", got)
		}
		if got := f.listedAccess(t, u.ID, via, "app.example.org"); got["canEnable"] != false || got["reason"] != otherPort {
			t.Errorf("other port = %v", got)
		}
	})

	t.Run("validation", func(t *testing.T) {
		for _, body := range []any{map[string]any{}, map[string]any{"domainOnly": "true"}, map[string]any{"domainOnly": 1}, map[string]any{"domainOnly": nil}} {
			if code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/"+host+"/access", body); code != 400 || obj["error"] != "domainOnly must be true or false" {
				t.Errorf("%v = %d %v", body, code, obj)
			}
		}
		if code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/bad_domain.test/access", on); code != 400 || obj["error"] != msgInvalidFormat {
			t.Errorf("invalid domain = %d %v", code, obj)
		}
		if code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/missing.example.org/access", on); code != 404 || obj["error"] != msgNotFound {
			t.Errorf("missing domain = %d %v", code, obj)
		}
	})

	t.Run("enable guards", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			from   accessRequest
			domain string
			msg    string
		}{
			{"not through the proxy", direct, host, open},
			{"through the proxy for another host", accessRequest{viaProxy: true, proto: "https", host: "plain.example.org"}, host, open},
			{"through the proxy over http", accessRequest{viaProxy: true, proto: "http", host: host}, host, open},
			{"no certificate", accessRequest{viaProxy: true, proto: "https", host: "plain.example.org"}, "plain.example.org", noCert},
			{"not the dashboard", accessRequest{viaProxy: true, proto: "https", host: "app.example.org"}, "app.example.org", otherPort},
		} {
			if code, obj, _ := f.doAs(t, u.ID, tc.from, "PUT", "/"+tc.domain+"/access", on); code != 409 || obj["error"] != tc.msg {
				t.Errorf("%s = %d %v, want 409 %q", tc.name, code, obj, tc.msg)
			}
		}
		code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/"+host+"/access", map[string]any{"domainOnly": true})
		if code != 403 || obj["requirePassword"] != true {
			t.Errorf("no password = %d %v", code, obj)
		}
		code, obj, _ = f.doAs(t, u.ID, via, "PUT", "/"+host+"/access", map[string]any{"domainOnly": true, "password": "wrong"})
		if code != 403 || obj["error"] != "Invalid password" {
			t.Errorf("wrong password = %d %v", code, obj)
		}
		if _, ok := f.domainOnlyRow(t); ok || f.d.PanelAccess.Domain() != "" {
			t.Fatal("a refused request changed the setting")
		}
	})

	t.Run("enable and disable", func(t *testing.T) {
		code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/"+host+"/access", on)
		access, _ := obj["access"].(map[string]any)
		if code != 200 || obj["success"] != true || obj["domain"] != host || access["domainOnly"] != true {
			t.Fatalf("enable = %d %v", code, obj)
		}
		if v, _ := f.domainOnlyRow(t); v != host || f.d.PanelAccess.Domain() != host {
			t.Fatalf("setting %q cache %q", v, f.d.PanelAccess.Domain())
		}
		if got := f.listedAccess(t, u.ID, via, host); got["domainOnly"] != true {
			t.Errorf("listed after enable = %v", got)
		}
		// Off for another domain changes nothing.
		if code, obj, _ := f.doAs(t, u.ID, direct, "PUT", "/plain.example.org/access", map[string]any{"domainOnly": false}); code != 200 || f.d.PanelAccess.Domain() != host {
			t.Errorf("off for another domain = %d %v", code, obj)
		}
		// Off needs no password and works from any session.
		code, obj, _ = f.doAs(t, u.ID, direct, "PUT", "/"+host+"/access", map[string]any{"domainOnly": false})
		if code != 200 || obj["access"].(map[string]any)["domainOnly"] != false || f.d.PanelAccess.Domain() != "" {
			t.Fatalf("disable = %d %v", code, obj)
		}
		if v, ok := f.domainOnlyRow(t); ok {
			t.Errorf("setting left: %q", v)
		}
	})

	t.Run("certificate retry without a certificate turns it off", func(t *testing.T) {
		if code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/"+host+"/access", on); code != 200 {
			t.Fatalf("enable = %d %v", code, obj)
		}
		f.setEmail(t)
		f.docker.setCertbot(nil)
		// The retry keeps HTTPS while a covering certificate exists.
		f.do(t, "POST", "/"+host+"/ssl/retry", nil)
		if f.d.PanelAccess.Domain() != host {
			t.Fatal("a retry that kept HTTPS turned it off")
		}
		if err := os.RemoveAll(filepath.Join(f.le, "live", host)); err != nil {
			t.Fatal(err)
		}
		f.do(t, "POST", "/"+host+"/ssl/retry", nil)
		if strings.Contains(f.readConf(t, host), "listen 443") {
			t.Fatal("retry kept HTTPS without a certificate")
		}
		if _, ok := f.domainOnlyRow(t); ok || f.d.PanelAccess.Domain() != "" {
			t.Error("retry ended without HTTPS but the restriction stayed on")
		}
	})

	t.Run("deleting the domain turns it off", func(t *testing.T) {
		f.installCert(t, host, 60)
		f.do(t, "POST", "/"+host+"/ssl/retry", nil)
		if code, obj, _ := f.doAs(t, u.ID, via, "PUT", "/"+host+"/access", on); code != 200 {
			t.Fatalf("enable = %d %v", code, obj)
		}
		f.docker.setReloadFails(true)
		if code, _, _ := f.do(t, "DELETE", "/"+host, nil); code != 500 {
			t.Fatalf("delete with a failing reload = %d", code)
		}
		if _, ok := f.domainOnlyRow(t); ok || f.d.PanelAccess.Domain() != "" {
			t.Error("the conf is gone but the restriction stayed on")
		}
	})
}
