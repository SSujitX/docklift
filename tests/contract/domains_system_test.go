package contract

import (
	"runtime"
	"testing"
)

func sslMissing(domain string) Obj {
	o := Obj{"status": "missing", "domain": domain, "expiresAt": nil, "error": nil, "diagnosticCommand": nil, "certPath": nil}
	if targetIsGo() {
		o["expired"], o["renewalOverdue"], o["daysUntilExpiry"] = false, false, nil
	}
	return o
}

func TestPanelDomains(t *testing.T) {
	c := admin.Client()
	t.Run("validation", func(t *testing.T) {
		for _, body := range []map[string]any{{}, {"domain": "a.test"}, {"port": 8080}, {"domain": "a.test", "port": "0"}, {"domain": "a.test", "port": 65536},
			{"domain": "a.test", "port": "80a"}, {"domain": "a.test", "port": 80.5}, {"domain": "   ", "port": 8080}} {
			c.Post(t, "/api/domains", body).ExpectError(t, 400, "Invalid domain or port")
		}
		for _, d := range []string{"localhost", "bad_domain.test", "-a.test", "a.test;b", "a..test"} {
			c.Post(t, "/api/domains", map[string]any{"domain": d, "port": 8080}).ExpectError(t, 400, "Invalid domain format")
			c.Get(t, "/api/domains/"+d+"/ssl").ExpectError(t, 400, "Invalid domain format")
			c.Post(t, "/api/domains/"+d+"/ssl/retry", nil).ExpectError(t, 400, "Invalid domain format")
			c.Delete(t, "/api/domains/"+d).ExpectError(t, 400, "Invalid domain format")
		}
		missing := "missing-" + randHex(3) + ".contract-test.dev"
		c.Get(t, "/api/domains/"+missing+"/ssl").ExpectError(t, 404, "Domain configuration not found")
		c.Post(t, "/api/domains/"+missing+"/ssl/retry", nil).ExpectError(t, 404, "Domain configuration not found")
		c.Delete(t, "/api/domains/"+missing).ExpectError(t, 404, "Domain configuration not found")
		c.Post(t, "/api/domains/dns-check", map[string]any{}).ExpectError(t, 400, "Provide at least one domain")
		c.Post(t, "/api/domains/dns-check", map[string]any{"domains": "a.test"}).ExpectError(t, 400, "Provide at least one domain")
		c.Post(t, "/api/domains/dns-check", map[string]any{"domains": []any{"bad domain", 5, nil}}).ExpectError(t, 400, "Provide at least one domain")
		mainInst.Anon().Get(t, "/api/domains").ExpectError(t, 401, "Authentication required")
	})
	t.Run("lifecycle without docker", func(t *testing.T) {
		requireNoDocker(t)
		host := "panel-" + randHex(4) + ".contract-test.dev"
		// The conf is written, then the nginx reload fails: 500, but the mapping exists.
		c.Post(t, "/api/domains", map[string]any{"domain": "  PANEL" + host[5:] + "  ", "port": "8443"}).Expect(t, 500, Obj{"error": Prefix("Nginx reload failed: ")})
		list := c.Get(t, "/api/domains").Arr(t)
		match(t, "listed panel domain", Obj{"domain": host, "port": 8443, "ssl": sslMissing(host)}, findBy(list, "domain", host))
		c.Get(t, "/api/domains/"+host+"/ssl").Expect(t, 200, Obj{"ssl": sslMissing(host), "events": Arr{}})
		c.Get(t, "/api/domains/"+"PANEL"+host[5:]+"/ssl").Expect(t, 200, Obj{"ssl": sslMissing(host), "events": Arr{}})
		if mainInst.Booted() {
			conf := string(readFileBytes(t, mainInst.Dirs.Nginx+"/"+host+".conf"))
			expectContains(t, "panel conf", conf, "# docklift-panel-domain\n# docklift-panel-port: 8443\n", "server_name "+host+";")
		}
		// A hostname belongs to one mapping.
		c.Post(t, "/api/domains", map[string]any{"domain": host, "port": 8080}).ExpectError(t, 409, `Hostname "`+host+`" is already used by a panel domain`)
		p, s := deployedProject(t, c, "paneldup")
		c.Put(t, "/api/deployments/"+p["id"].(string)+"/services/"+s["id"].(string), map[string]any{"domain": host}).
			ExpectError(t, 409, `Hostname "`+host+`" is already used by a panel domain`)
		c.Post(t, "/api/domains/"+host+"/ssl/retry", nil).Expect(t, 500, Obj{"error": Prefix("Nginx reload failed: ")})
		// Delete removes the conf before the reload fails.
		c.Delete(t, "/api/domains/"+host).ExpectError(t, 500, "Failed to delete domain")
		c.Delete(t, "/api/domains/"+host).ExpectError(t, 404, "Domain configuration not found")
		if findBy(c.Get(t, "/api/domains").Arr(t), "domain", host) != nil {
			t.Fatal("deleted panel domain still listed")
		}
	})
	t.Run("dns-check lookups", func(t *testing.T) {
		requireNetwork(t)
		c.Post(t, "/api/domains/dns-check", map[string]any{"domains": []any{"example.com"}}).Expect(t, 200, Obj{
			"checks":   Arr{Obj{"domain": "example.com", "status": Either("ok", "mismatch", "missing", "unknown"), "a": AnyArr, "aaaa": AnyArr, "serverIp": OrNull(Str), "message": Str}},
			"serverIp": OrNull(Str)})
	})
}

func TestACMEEmail(t *testing.T) {
	c := admin.Client()
	c.Get(t, "/api/domains/ssl/email").Expect(t, 200, Obj{"email": Str})
	c.Put(t, "/api/domains/ssl/email", map[string]any{}).ExpectError(t, 400, "Invalid email")
	c.Put(t, "/api/domains/ssl/email", map[string]any{"email": "not-an-email"}).ExpectError(t, 400, "Invalid email")
	// A non-string reaches String.prototype methods in TS and leaks the JS TypeError; Go answers clearly.
	nonString := "email.includes is not a function"
	if targetIsGo() {
		nonString = "Email must be a string"
	}
	c.Put(t, "/api/domains/ssl/email", map[string]any{"email": 12}).ExpectError(t, 400, nonString)
	c.Put(t, "/api/domains/ssl/email", map[string]any{"email": "  Ops@Contract.Test "}).Expect(t, 200, Obj{"success": true, "email": "ops@contract.test"})
	c.Get(t, "/api/domains/ssl/email").Expect(t, 200, Obj{"email": "ops@contract.test"})
}

func TestACMEEmailDefaultsToAdmin(t *testing.T) {
	inst := seededInstance(t)
	s := mustLogin(t, inst, seedAdminEmail, seedAdminPassword)
	s.Client().Get(t, "/api/domains/ssl/email").Expect(t, 200, Obj{"email": seedAdminEmail})
}

func TestSystemStats(t *testing.T) {
	c := admin.Client()
	c.Get(t, "/api/system/stats").Expect(t, 200, Obj{
		"cpu":    Obj{"usage": Num, "cores": Int, "model": Str, "speed": Num, "temperature": OrNull(Num)},
		"memory": Obj{"total": Num, "used": Num, "free": Num, "usedPercent": Num},
		"gpu": Obj{"available": Bool, "model": OrNull(Str), "memoryTotal": OrNull(Num), "memoryUsed": OrNull(Num),
			"temperature": OrNull(Num), "utilization": OrNull(Num)},
		"disk":    ArrayOf(Obj{"mount": Str, "type": Str, "total": Num, "used": Num, "usedPercent": Num}, 0),
		"network": Obj{"bytesReceived": Num, "bytesSent": Num, "rxSpeed": Num, "txSpeed": Num},
		"server": Obj{"hostname": Str, "platform": Str, "distro": Str, "kernel": Str, "arch": Str, "uptime": Num,
			"uptimeFormatted": Str, "serverTime": Str, "cpuModel": Str, "cpuCores": Re(`^\d+ Cores @ [\d.]+ GHz$`),
			"loadAvg": Obj{"load1": Num, "load5": Num, "load15": Num}, "swap": Obj{"total": Num, "used": Num},
			"ipAddress": Str, "location": Str, "activeConnections": Int},
		"processes": ArrayOf(Obj{"pid": Int, "name": Str, "cpu": Num, "mem": Num, "user": Str}, 0),
		"timestamp": ISOms,
	})
	c.Get(t, "/api/system/quick").Expect(t, 200, Obj{"cpu": Num, "memory": Num, "timestamp": ISOms})
	c.Get(t, "/api/system/ip").Expect(t, 200, Obj{"ip": Str})
	c.Get(t, "/api/system/version?refresh=1").Expect(t, 200, Obj{"current": NonEmpty, "latest": NonEmpty, "updateAvailable": Bool, "githubOk": Bool, "checkedAt": ISOms})
	for _, p := range []string{"/api/system/stats", "/api/system/quick", "/api/system/ip", "/api/system/version"} {
		mainInst.Anon().Get(t, p).ExpectError(t, 401, "Authentication required")
	}
}

func TestSystemLogStreams(t *testing.T) {
	c := admin.Client()
	sse := c.Post(t, "/api/auth/sse-token", nil).Obj(t)["token"].(string)
	anon := mainInst.Anon()
	anon.Get(t, "/api/system/logs/backend").ExpectError(t, 401, "SSE token required")
	c.Get(t, "/api/system/logs/backend").ExpectError(t, 401, "SSE token required")
	anon.Get(t, "/api/system/logs/backend?token="+admin.Token).ExpectError(t, 401, "SSE token required")
	anon.Get(t, "/api/system/logs/backend?token=x.y.z").ExpectError(t, 401, "Invalid token")
	anon.Get(t, "/api/system/logs/database?token="+sse).ExpectError(t, 400, invalidLogService())
	// Only GET /logs/* takes the SSE path; other methods use Bearer auth.
	anon.Post(t, "/api/system/logs/backend?token="+sse, nil).ExpectError(t, 401, "Authentication required")
	t.Run("stream without docker", func(t *testing.T) {
		requireNoDocker(t)
		for _, svc := range logServices() {
			r := anon.Get(t, "/api/system/logs/"+svc+"?token="+sse+"&tail=10")
			r.ExpectStatus(t, 200)
			assertSSEHeaders(t, r)
			ev := sseEvents(t, string(r.Body))
			if len(ev) != 1 {
				t.Fatalf("%s: want one SSE event, got %v", svc, ev)
			}
			match(t, svc+" event", Obj{"type": "error", "message": Prefix("Container not found: ")}, ev[0])
		}
	})
}

func TestSystemDangerousEndpointsRequireStepUp(t *testing.T) {
	c := admin.Client()
	_, internal := func() (string, string) {
		if mainInst.DataDir == "" {
			return "", ""
		}
		return readSecrets(t, mainInst)
	}()
	// Success paths are never exercised: these reboot/upgrade the host.
	for _, path := range []string{"/api/system/purge", "/api/system/reboot", "/api/system/reset", "/api/system/update-system", "/api/system/upgrade"} {
		t.Run(path, func(t *testing.T) {
			c.Post(t, path, nil).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
			c.Post(t, path, map[string]any{"password": ""}).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
			c.Post(t, path, map[string]any{"password": true}).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
			c.Post(t, path, map[string]any{"password": "wrong-password"}).Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
			if internal != "" {
				// Go no longer accepts the internal secret at all (deliberate fix).
				msg := "Unauthorized"
				if targetIsGo() {
					msg = "Authentication required"
				}
				mainInst.Anon().With("X-Internal-Secret", internal).Post(t, path, map[string]any{"password": admin.Password}).ExpectError(t, 401, msg)
			}
			mainInst.Anon().Post(t, path, map[string]any{"password": admin.Password}).ExpectError(t, 401, "Authentication required")
			sse := c.Post(t, "/api/auth/sse-token", nil).Obj(t)["token"].(string)
			mainInst.As(sse).Post(t, path, map[string]any{"password": admin.Password}).ExpectError(t, 401, "Invalid token for this endpoint")
		})
	}
	t.Run("reset without docker", func(t *testing.T) {
		// Safe to run: the harness points DOCKER_HOST at a closed port.
		requireBoot(t)
		requireNoDocker(t)
		r := c.Post(t, "/api/system/reset", map[string]any{"password": admin.Password})
		switch {
		case runtime.GOOS == "windows":
			r.Expect(t, 200, Obj{"message": "Dev Mode: Simulated Service Reset complete."})
		case targetIsGo():
			// Deliberate fix 9: TypeScript reported success and the detached
			// `docker restart` failed silently.
			r.ExpectError(t, 503, "Docker is not available")
		default:
			r.Expect(t, 200, Obj{"message": "Services reset command sent successfully (Restarting in 1s)"})
		}
	})
	t.Run("purge without docker", func(t *testing.T) {
		// Safe to run: no daemon, so nothing can be removed.
		requireNoDocker(t)
		r := c.Post(t, "/api/system/purge", map[string]any{"password": admin.Password})
		if targetIsGo() {
			// Deliberate fix 9; the dashboard only shows data.error.
			r.ExpectError(t, 503, "Docker is not available")
			return
		}
		r.ExpectError(t, 500, "Failed to complete purge operation")
	})
}

// TestStepUpPasswordThrottle covers the Go-only brute-force limit on
// step-up passwords (owner-approved security fix; TypeScript allowed
// unlimited guesses to anyone holding a session token): 10 wrong passwords
// per user and client address in 15 minutes, then 429 with Retry-After.
func TestStepUpPasswordThrottle(t *testing.T) {
	if !targetIsGo() {
		t.Skip("Go-only security fix: the TypeScript backend has no step-up throttle")
	}
	// Clients behind one proxy address would share (and exhaust) the counter.
	requireBoot(t)
	c := admin.Client()
	rollback := "/api/deployments/" + "00000000-0000-4000-8000-000000000000" + "/rollback"
	for i := 0; i < 10; i++ {
		c.Post(t, rollback, map[string]any{"password": "wrong-password"}).Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
	}
	throttled := Obj{"error": "Too many failed password attempts. Try again later.", "requirePassword": true}
	// The limit holds even for the right password, and across step-up routes.
	r := c.Post(t, rollback, map[string]any{"password": admin.Password})
	r.Expect(t, 429, throttled)
	if got := r.Header.Get("Retry-After"); got == "" || got == "0" {
		t.Fatalf("Retry-After = %q", got)
	}
	c.Post(t, "/api/system/execute", map[string]any{"command": "true", "password": admin.Password}).Expect(t, 429, throttled)
	// Another client address of the same user is counted separately; the
	// guard passes and the route answers for the missing deploymentId.
	admin.Client().Post(t, rollback, map[string]any{"password": admin.Password}).ExpectError(t, 400, "deploymentId is required")
}

func TestSystemExecute(t *testing.T) {
	c := admin.Client()
	c.Post(t, "/api/system/execute", map[string]any{}).ExpectError(t, 400, "Command is required")
	c.Post(t, "/api/system/execute", map[string]any{"command": 7, "password": admin.Password}).ExpectError(t, 400, "Command is required")
	c.Post(t, "/api/system/execute", map[string]any{"command": "echo hi"}).Expect(t, 401, Obj{"error": "Password required for terminal access", "requirePassword": true})
	c.Post(t, "/api/system/execute", map[string]any{"command": "echo hi", "password": 1}).Expect(t, 401, Obj{"error": "Password required for terminal access", "requirePassword": true})
	c.Post(t, "/api/system/execute", map[string]any{"command": "echo hi", "password": "wrong"}).Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
	mainInst.Anon().Post(t, "/api/system/execute", map[string]any{"command": "echo hi", "password": admin.Password}).ExpectError(t, 401, "Authentication required")
	t.Run("runs through /bin/bash", func(t *testing.T) {
		if !mainInst.Booted() || runtime.GOOS != "linux" {
			t.Skip("success path needs a Linux server booted by the harness")
		}
		c.Post(t, "/api/system/execute", map[string]any{"command": "echo contract-ok", "password": admin.Password}).Expect(t, 200, Obj{"output": "contract-ok\n"})
		c.Post(t, "/api/system/execute", map[string]any{"command": "echo oops >&2", "password": admin.Password}).Expect(t, 200, Obj{"output": "oops\n", "error": "oops\n"})
		c.Post(t, "/api/system/execute", map[string]any{"command": "true", "password": admin.Password}).Expect(t, 200, Obj{"output": "(no output)"})
		c.Post(t, "/api/system/execute", map[string]any{"command": "exit 3", "password": admin.Password}).Expect(t, 200, Obj{"output": "", "error": Prefix("Command failed: exit 3")})
	})
	t.Run("no bash on windows", func(t *testing.T) {
		if !mainInst.Booted() || runtime.GOOS != "windows" {
			t.Skip("windows-only")
		}
		c.Post(t, "/api/system/execute", map[string]any{"command": "echo hi", "password": admin.Password}).Expect(t, 200, Obj{"output": "", "error": NonEmpty})
	})
}
