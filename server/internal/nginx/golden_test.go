package nginx

import (
	"path/filepath"
	"strings"
	"testing"
)

// goldenCase is one committed rendering (Name is the golden file).
type goldenCase struct {
	Name        string
	Kind        string
	ID          string
	ServiceName string
	Container   string
	Port        int
	Hosts       []string
	Domain      string
	EnableHTTPS bool
}

const svcID = "3f2c9a4e-1b7d-4c2e-9f0a-6d5e4c3b2a10"

var goldenCases = []goldenCase{
	{Name: "service-http-only", Kind: "service", ID: svcID, ServiceName: "web", Container: "docklift-shop-web", Port: 3000, Hosts: []string{"app.example.com"}, EnableHTTPS: true},
	{Name: "service-http-only-https-disabled", Kind: "service", ID: svcID, ServiceName: "web", Container: "docklift-shop-web", Port: 3000, Hosts: []string{"a.example.com", "b.example.com"}},
	{Name: "service-active", Kind: "service", ID: svcID, ServiceName: "web", Container: "docklift-shop-web", Port: 8080, Hosts: []string{"a.example.com", "b.example.com"}, EnableHTTPS: true},
	{Name: "service-active-reordered", Kind: "service", ID: svcID, ServiceName: "My App (prod)", Container: "docklift-my-app-prod", Port: 80, Hosts: []string{"b.example.com", "a.example.com"}, EnableHTTPS: true},
	{Name: "service-active-single-san", Kind: "service", ID: "svc1", ServiceName: "api", Container: "api_1", Port: 65535, Hosts: []string{"b.example.com"}, EnableHTTPS: true},
	{Name: "service-partial", Kind: "service", ID: svcID, ServiceName: "web", Container: "docklift-shop-web", Port: 3000, Hosts: []string{"a.example.com", "c.example.com"}, EnableHTTPS: true},
	{Name: "service-partial-orphan", Kind: "service", ID: svcID, ServiceName: "web", Container: "docklift-shop-web", Port: 3000, Hosts: []string{"b.example.com", "c.example.com"}, EnableHTTPS: true},
	{Name: "service-partial-staging", Kind: "service", ID: svcID, ServiceName: "web", Container: "docklift-shop-web", Port: 3000, Hosts: []string{"prod.example.com", "staging.example.com"}, EnableHTTPS: true},
	{Name: "panel-dashboard-http-only", Kind: "panel", Domain: "panel.example.com", Port: 8080},
	{Name: "panel-dashboard-pending", Kind: "panel", Domain: "panel.example.com", Port: 8080, EnableHTTPS: true},
	{Name: "panel-dashboard-active", Kind: "panel", Domain: "a.example.com", Port: 8080, EnableHTTPS: true},
	{Name: "panel-host-port-http-only", Kind: "panel", Domain: "x.example.com", Port: 3000},
	{Name: "panel-host-port-active", Kind: "panel", Domain: "b.example.com", Port: 8443, EnableHTTPS: true},
}

func goldenCerts(t testing.TB, le string) {
	coveringCerts(t, le)
	writeLive(t, le, "prod.example.com", fixturePEM(t, "prod-only.pem"))
	writeLive(t, le, "staging.example.com", fixturePEM(t, "staging-only.pem"))
}

// renderGo renders a case with the Go templates.
func renderGo(t *testing.T, e *env, c goldenCase) string {
	t.Helper()
	if c.Kind == "panel" {
		out, err := BuildPanelConf(e.certs, c.Domain, c.Port, 8000, c.EnableHTTPS)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	loc, err := BuildServiceProxyLocation(c.ID, c.Container, c.Port)
	if err != nil {
		t.Fatal(err)
	}
	out, err := BuildHTTPHTTPSServers(e.certs, ServerBlocks{
		Comment: "Main server block for " + c.ServiceName, ServerNames: strings.Join(c.Hosts, " "),
		PrimaryDomain: c.Hosts[0], ProxyLocation: loc, EnableHTTPS: c.EnableHTTPS,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// expectedFromTS maps a golden file (the 2.x rendering) to what Go must
// produce: equal, except dashboard panel confs, whose upstream moved to
// docklift-backend (exactly the boot migration's rewrite).
func expectedFromTS(c goldenCase, ts string) string {
	if c.Kind == "panel" && c.Port == DashboardPort {
		return rewritePanelUpstream(ts, 8000)
	}
	return ts
}

func goldenPath(name string) string { return filepath.Join("testdata", "golden", name+".conf") }

// TestGoldenFiles compares Go with the committed renderings produced by the
// 2.1.x backend, so existing nginx confs stay byte-identical.
func TestGoldenFiles(t *testing.T) {
	e := newEnv(t)
	goldenCerts(t, e.le)
	for _, c := range goldenCases {
		t.Run(c.Name, func(t *testing.T) {
			ts := readFile(t, goldenPath(c.Name))
			if got, want := renderGo(t, e, c), expectedFromTS(c, ts); got != want {
				t.Fatalf("--- go\n%s\n--- want\n%s", got, want)
			}
			if c.Kind == "panel" && c.Port == DashboardPort {
				if !strings.Contains(ts, "set $dashboard docklift-nginx;\n        proxy_pass http://$dashboard:80;") {
					t.Fatal("TS golden lacks the old upstream")
				}
				got := renderGo(t, e, c)
				if !strings.Contains(got, "set $dashboard docklift-backend.docklift_network;\n        proxy_pass http://$dashboard:8000;") || strings.Contains(got, "docklift-nginx") {
					t.Fatalf("new upstream missing:\n%s", got)
				}
				if p, ok := ParsePanelPort(got); !ok || p != 8080 {
					t.Fatal("panel port not parseable from the new conf")
				}
			}
		})
	}
	// The markers the code parses back must be present.
	for name, marker := range map[string]string{
		"service-http-only":      "# docklift-ssl: pending|http-only primary=app.example.com\n",
		"service-partial":        "# docklift-ssl: partial primary=a.example.com cert=a.example.com\n",
		"service-active":         "# docklift-ssl: active primary=a.example.com cert=a.example.com\n",
		"panel-dashboard-active": "# docklift-panel-domain\n# docklift-panel-port: 8080\n\n# Panel domain a.example.com\n",
	} {
		if !strings.Contains(readFile(t, goldenPath(name)), marker) {
			t.Errorf("%s lacks %q", name, marker)
		}
	}
}
