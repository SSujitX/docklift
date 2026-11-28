package contract

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// httpsPanelConf is a dashboard panel conf with an active certificate, as
// BuildPanelConf writes it (the certificate itself is not needed here).
func httpsPanelConf(host string) string {
	return `# docklift-panel-domain
# docklift-panel-port: 8080

# Panel domain ` + host + `
# docklift-ssl: active primary=` + host + ` cert=` + host + `
server {
    listen 80;
    server_name ` + host + `;

    include /etc/nginx/snippets/acme.conf;

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ` + host + `;

    ssl_certificate /etc/letsencrypt/live/` + host + `/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/` + host + `/privkey.pem;

    resolver 127.0.0.11 valid=30s ipv6=off;

    location / {
        set $dashboard docklift-backend.docklift_network;
        proxy_pass http://$dashboard:8000;
    }
}
`
}

// TestPanelAccess covers "Only allow access through the domain"
// (PUT /api/domains/:domain/access and the guard in front of every route).
// The instance trusts only 127.0.0.1 as its proxy, so a client of [::1]
// plays a direct client of the published dashboard port.
func TestPanelAccess(t *testing.T) {
	const host = "panel-access.contract-test.dev"
	inst := dedicatedInstance(t, BootOptions{
		Name:     "panel-access",
		ExtraEnv: map[string]string{"TRUSTED_PROXIES": "127.0.0.1"},
		Prepare: func(d InstanceDirs) error {
			return os.WriteFile(filepath.Join(d.Nginx, host+".conf"), []byte(httpsPanelConf(host)), 0o644)
		},
	})
	s, err := ensureAdmin(inst)
	if err != nil {
		t.Fatal(err)
	}
	proxied := s.Client()
	viaDomain := proxied.With("X-Forwarded-Proto", "https").With("X-Forwarded-Host", host)
	openFirst := "Open the dashboard at https://" + host + " first, then turn this on"
	refused := "Dashboard access by IP is disabled. Use https://" + host
	on := map[string]any{"domainOnly": true, "password": s.Password}
	listed := func(c *Client) any { return findBy(c.Get(t, "/api/domains").Arr(t), "domain", host)["access"] }

	match(t, "access by IP", Obj{"domainOnly": false, "canEnable": false, "reason": openFirst}, listed(proxied))
	match(t, "access through the domain", Obj{"domainOnly": false, "canEnable": true}, listed(viaDomain))

	t.Run("enable refused when not through the domain", func(t *testing.T) {
		proxied.Put(t, "/api/domains/"+host+"/access", on).ExpectError(t, 409, openFirst)
		viaDomain.With("X-Forwarded-Proto", "http").Put(t, "/api/domains/"+host+"/access", on).ExpectError(t, 409, openFirst)
		viaDomain.With("X-Forwarded-Host", "other.contract-test.dev").Put(t, "/api/domains/"+host+"/access", on).ExpectError(t, 409, openFirst)
		viaDomain.Put(t, "/api/domains/"+host+"/access", map[string]any{"domainOnly": true}).
			Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
		viaDomain.Put(t, "/api/domains/"+host+"/access", map[string]any{"domainOnly": true, "password": "wrong-password"}).
			Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
		match(t, "still open", Obj{"domainOnly": false, "canEnable": true}, listed(viaDomain))
	})

	// [::1] is a direct client; skip what needs it when the host has no IPv6 loopback.
	u, err := url.Parse(inst.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var direct *Client
	if conn, err := net.Dial("tcp", net.JoinHostPort("::1", u.Port())); err == nil {
		conn.Close()
		direct = (&Instance{Name: inst.Name + "-ipv6", BaseURL: "http://" + net.JoinHostPort("::1", u.Port())}).As(s.Token)
	} else {
		t.Logf("no IPv6 loopback (%v): direct-client checks skipped", err)
	}

	t.Run("enable, enforce, disable", func(t *testing.T) {
		viaDomain.Put(t, "/api/domains/"+host+"/access", on).
			Expect(t, 200, Obj{"success": true, "domain": host, "access": Obj{"domainOnly": true, "canEnable": true}})
		match(t, "listed while on", Obj{"domainOnly": true, "canEnable": false, "reason": openFirst}, listed(proxied))
		proxied.Get(t, "/api/projects").ExpectStatus(t, 200)

		if direct != nil {
			direct.Get(t, "/api/projects").Expect(t, 403, Obj{"error": refused})
			direct.Get(t, "/api/projects", "X-Forwarded-Proto", "https", "X-Forwarded-Host", host, "X-Forwarded-For", "127.0.0.1").
				Expect(t, 403, Obj{"error": refused})
			direct.Post(t, "/api/auth/login", map[string]any{"email": s.Email, "password": s.Password}).Expect(t, 403, Obj{"error": refused})
			direct.Get(t, "/ws/terminal?token=x", "Upgrade", "websocket", "Connection", "Upgrade",
				"Sec-WebSocket-Version", "13", "Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==").Expect(t, 403, Obj{"error": refused})
			nav := direct.Get(t, "/settings?tab=domain", "Accept", "text/html,application/xhtml+xml")
			nav.ExpectStatus(t, 302)
			if got := nav.Header.Get("Location"); got != "https://"+host+"/settings?tab=domain" {
				t.Errorf("redirect Location = %q", got)
			}
			direct.Get(t, "/api/health").Expect(t, 200, Obj{"status": "ok"})
			if r := direct.Post(t, "/api/github/webhook", map[string]any{"zen": "hi"}, "X-GitHub-Event", "ping"); r.Status == 403 {
				t.Errorf("webhook refused: %s", r)
			}
		}

		// Turning it off needs only a session (no password, not the domain).
		proxied.Put(t, "/api/domains/"+host+"/access", map[string]any{"domainOnly": false}).
			Expect(t, 200, Obj{"success": true, "domain": host, "access": Obj{"domainOnly": false, "canEnable": false, "reason": openFirst}})
		if direct != nil {
			direct.Get(t, "/api/projects").ExpectStatus(t, 200)
		}
	})

	t.Run("removing the domain turns it off", func(t *testing.T) {
		viaDomain.Put(t, "/api/domains/"+host+"/access", on).ExpectStatus(t, 200)
		if r := proxied.Delete(t, "/api/domains/"+host); r.Status != 200 && r.Status != 500 {
			t.Fatalf("delete: %s", r)
		}
		if findBy(proxied.Get(t, "/api/domains").Arr(t), "domain", host) != nil {
			t.Fatal("domain still listed")
		}
		if direct != nil {
			direct.Get(t, "/api/projects").ExpectStatus(t, 200)
		}
	})
}
