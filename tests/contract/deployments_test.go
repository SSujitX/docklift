package contract

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func shortID(id string) string { return strings.ReplaceAll(id, "-", "")[:8] }

// containerName mirrors lib/naming.ts serviceContainerName for slug-safe names.
func containerName(projectName, projectID, svc string) string {
	return "dl_" + projectName + "_" + shortID(projectID) + "_" + svc
}

func lastDeployment(t *testing.T, c *Client, projectID string) map[string]any {
	t.Helper()
	list := c.Get(t, "/api/deployments/"+projectID+"?limit=1").Arr(t)
	if len(list) == 0 {
		t.Fatalf("no deployments for %s", projectID)
	}
	return list[0].(map[string]any)
}

func TestDeploymentHistory(t *testing.T) {
	c := admin.Client()
	p := createProject(t, c, uniq("history"), dockerfileApp())
	id := p["id"].(string)
	c.Get(t, "/api/deployments/"+id).Expect(t, 200, Arr{})
	c.Get(t, "/api/deployments/"+id+"?meta=1").Expect(t, 200, Obj{"items": Arr{}, "total": 0})
	c.Get(t, "/api/deployments/"+missingProjectID+"?meta=true").Expect(t, 200, Obj{"items": Arr{}, "total": 0})

	for i := 0; i < 3; i++ {
		deployAndWait(t, c, id)
	}
	c.Post(t, "/api/deployments/"+id+"/cancel", nil).ExpectStatus(t, 200)
	all := c.Get(t, "/api/deployments/"+id).Arr(t)
	if len(all) != 3 {
		t.Fatalf("want 3 deployments (idle cancel adds no row), got %d", len(all))
	}
	for i, d := range all {
		match(t, "deployment row", deploymentRow(Obj{"project_id": id, "image_tags": nil, "commit_sha": nil}), d)
		if i > 0 && all[i-1].(map[string]any)["created_at"].(string) < d.(map[string]any)["created_at"].(string) {
			t.Fatalf("deployments must be newest first")
		}
	}
	firstID := all[0].(map[string]any)["id"]
	c.Get(t, "/api/deployments/"+id+"?limit=2").Expect(t, 200, ArrayOf(AnyObj, 2))
	if got := len(c.Get(t, "/api/deployments/"+id+"?limit=2").Arr(t)); got != 2 {
		t.Fatalf("limit=2 returned %d", got)
	}
	off := c.Get(t, "/api/deployments/"+id+"?limit=1&offset=1").Arr(t)
	if len(off) != 1 || off[0].(map[string]any)["id"] != all[1].(map[string]any)["id"] {
		t.Fatalf("offset=1 returned %v", off)
	}
	// Non-numeric or zero limit falls back to 10.
	if got := len(c.Get(t, "/api/deployments/"+id+"?limit=abc&offset=-x").Arr(t)); got != 3 {
		t.Fatalf("limit=abc returned %d", got)
	}
	if got := len(c.Get(t, "/api/deployments/"+id+"?limit=0").Arr(t)); got != 3 {
		t.Fatalf("limit=0 returned %d", got)
	}
	meta := c.Get(t, "/api/deployments/"+id+"?meta=1&limit=1").Expect(t, 200, Obj{"items": ArrayOf(AnyObj, 1), "total": 3}).(map[string]any)
	if meta["items"].([]any)[0].(map[string]any)["id"] != firstID {
		t.Fatalf("meta items[0] is not the newest deployment")
	}
	c.Get(t, "/api/deployments/"+id+"?meta=1&status=success").Expect(t, 200, Obj{"items": Arr{}, "total": 0})
	if !cfg.docker {
		c.Get(t, "/api/deployments/"+id+"?meta=1&status=%20failed%20").Expect(t, 200, Obj{"items": ArrayOf(AnyObj, 3), "total": 3})
	}
	c.Get(t, "/api/deployments/"+id+"?meta=0").Expect(t, 200, ArrayOf(AnyObj, 3))
	mainInst.Anon().Get(t, "/api/deployments/"+id).ExpectError(t, 401, "Authentication required")
}

func TestDeployWithoutDocker(t *testing.T) {
	requireNoDocker(t)
	c := admin.Client()
	name := uniq("deploy")
	p := createProject(t, c, name, dockerfileApp(), "domain", "Deploy-"+randHex(3)+".contract-test.dev")
	id := p["id"].(string)
	c.Post(t, base(id, "/env"), map[string]any{"key": "PUBLIC_ARG", "value": "1", "is_build_arg": true}).ExpectStatus(t, 201)

	s := c.Open(t, "POST", "/api/deployments/"+id+"/deploy", map[string]any{"trigger": "webhook", "commit_message": "contract commit"})
	if s.Status != 200 {
		t.Fatalf("deploy status %d: %s", s.Status, s.ReadAll())
	}
	if ct := s.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("deploy Content-Type = %q", ct)
	}
	body := s.ReadAll()
	if !strings.HasPrefix(body, "🚀 Starting deployment...\n") {
		t.Fatalf("deploy log must start with the start marker: %q", truncate(body, 200))
	}
	expectContains(t, "deploy log", body,
		"📦 BUILD: 1 repository Dockerfile",
		"🐳 app: Dockerfile",
		"Internal port: 8080",
		"Assigned new service: app (no host port) with domain: "+p["domain"].(string),
		"Container: "+containerName(name, id, "app"),
		"🔒 Host ports off",
		"WARNING: The following build arguments are configured but missing 'ARG' instructions in Dockerfile",
		"    - PUBLIC_ARG\n",
		"🐳 Building app with Dockerfile",
		"\n❌ Error: ")

	d := lastDeployment(t, c, id)
	match(t, "failed deployment", deploymentRow(Obj{"project_id": id, "status": "failed", "trigger": "webhook", "commit_message": "contract commit",
		"commit_sha": nil, "image_tags": nil, "logs": Contains("❌ Error: "), "finished_at": ISOms}), d)
	if !strings.HasPrefix(d["logs"].(string), "🚀 Starting deployment...\n") {
		t.Fatalf("persisted logs = %q", truncate(d["logs"].(string), 200))
	}
	c.Get(t, "/api/deployments/"+id+"/services").Expect(t, 200, Arr{serviceRow(Obj{
		"project_id": id, "name": "app", "dockerfile_path": "Dockerfile", "container_name": containerName(name, id, "app"),
		"port": nil, "internal_port": 8080, "domain": p["domain"], "status": "error",
	})})
	// Reading the project re-syncs status from Docker: no container => "stopped".
	c.Get(t, "/api/projects/"+id).Expect(t, 200, projectRow(Obj{"id": id, "name": name, "domain": p["domain"], "status": "stopped"}))

	t.Run("redeploy keeps the service row", func(t *testing.T) {
		body := deployAndWaitPath(t, c, id, "/redeploy")
		expectContains(t, "redeploy log", body, "Updating existing service: app\n", "\n❌ Error: ")
		if strings.Contains(body, "Assigned new service") {
			t.Fatal("redeploy must reuse the service")
		}
		if got := len(c.Get(t, "/api/deployments/"+id+"/services").Arr(t)); got != 1 {
			t.Fatalf("services after redeploy = %d", got)
		}
		match(t, "redeploy row", deploymentRow(Obj{"project_id": id, "status": "failed", "trigger": "manual", "commit_message": nil, "finished_at": ISOms}),
			lastDeployment(t, c, id))
	})
	t.Run("secret build var preflight", func(t *testing.T) {
		c.Post(t, base(id, "/env"), map[string]any{"key": "NPM_TOKEN", "value": "x", "is_build_arg": true, "is_secret": true}).ExpectStatus(t, 201)
		body := deployAndWait(t, c, id)
		expectContains(t, "preflight log", body, "❌ PREFLIGHT FAILED: secret build vars need Dockerfile mounts:",
			"    - NPM_TOKEN (in Dockerfile)\n", "\n❌ Error: Secret build vars missing Dockerfile mounts: NPM_TOKEN (in Dockerfile)\n")
		if strings.Contains(body, "Building app") {
			t.Fatal("preflight failure must stop before docker build")
		}
	})
	t.Run("reserved build variable", func(t *testing.T) {
		q := createProject(t, c, uniq("reserved"), dockerfileApp())
		qid := q["id"].(string)
		c.Post(t, base(qid, "/env"), map[string]any{"key": "DOCKER_HOST", "value": "x", "is_build_arg": true}).ExpectStatus(t, 201)
		expectContains(t, "reserved log", deployAndWait(t, c, qid), "\n❌ Error: Build variable DOCKER_HOST is reserved by DockLift and cannot be overridden\n")
	})
	t.Run("railpack project", func(t *testing.T) {
		q := createProject(t, c, uniq("railpack"), files("package.json", "{}", "index.js", "1"))
		qid := q["id"].(string)
		body := deployAndWait(t, c, qid)
		expectContains(t, "railpack log", body, "📦 BUILD: Railpack source (package.json)", "🛤️ app: [railpack]", "No Dockerfile found → analyzing . with Railpack", "\n❌ Error: ")
		c.Get(t, "/api/deployments/"+qid+"/services").Expect(t, 200, Arr{serviceRow(Obj{
			"project_id": qid, "name": "app", "dockerfile_path": "[railpack]", "internal_port": 3000, "port": nil, "domain": nil, "status": "error",
		})})
	})
	t.Run("broken build settings fail inside the stream", func(t *testing.T) {
		q := createProject(t, c, uniq("broken"), files("x.txt", "x"), "base_directory", "missing")
		body := deployAndWait(t, c, q["id"].(string))
		expectContains(t, "broken log", body, "\n❌ Error: Base directory not found: missing\n")
	})
}

func base(id, suffix string) string { return "/api/projects/" + id + suffix }

func deployAndWaitPath(t *testing.T, c *Client, projectID, action string) string {
	t.Helper()
	s := c.Open(t, "POST", "/api/deployments/"+projectID+action, nil)
	if s.Status != 200 {
		t.Fatalf("%s %s: status %d: %s", action, projectID, s.Status, s.ReadAll())
	}
	return s.ReadAll()
}

func TestDeploymentNotFoundAndValidation(t *testing.T) {
	c := admin.Client()
	for _, action := range []string{"/deploy", "/redeploy", "/stop", "/restart", "/cancel"} {
		c.Post(t, "/api/deployments/"+missingProjectID+action, nil).ExpectError(t, 404, "Project not found")
		mainInst.Anon().Post(t, "/api/deployments/"+missingProjectID+action, nil).ExpectError(t, 401, "Authentication required")
	}
	c.Get(t, "/api/deployments/"+missingProjectID+"/services").Expect(t, 200, Arr{})
	c.Get(t, "/api/deployments/"+missingProjectID+"/logs").Expect(t, 200, Obj{"logs": Obj{}})
	c.Get(t, "/api/deployments/"+missingProjectID+"/stats").Expect(t, 200, Obj{})

	t.Run("rollback validation", func(t *testing.T) {
		r := "/api/deployments/" + missingProjectID + "/rollback"
		c.Post(t, r, nil).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
		c.Post(t, r, map[string]any{"password": 12345}).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
		c.Post(t, r, map[string]any{"password": "wrong"}).Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
		c.Post(t, r, map[string]any{"password": admin.Password}).ExpectError(t, 400, "deploymentId is required")
		c.Post(t, r, map[string]any{"password": admin.Password, "deploymentId": "   "}).ExpectError(t, 400, "deploymentId is required")
		c.Post(t, r, map[string]any{"password": admin.Password, "deploymentId": "abc"}).ExpectError(t, 404, "Project not found")
		p := createProject(t, c, uniq("rollback"), dockerfileApp())
		pr := "/api/deployments/" + p["id"].(string) + "/rollback"
		c.Post(t, pr, map[string]any{"password": admin.Password, "deploymentId": missingProjectID}).ExpectError(t, 404, "Target successful deployment not found")
		if !cfg.docker {
			deployAndWait(t, c, p["id"].(string))
			failed := lastDeployment(t, c, p["id"].(string))["id"]
			c.Post(t, pr, map[string]any{"password": admin.Password, "deploymentId": failed}).ExpectError(t, 404, "Target successful deployment not found")
		}
	})
	t.Run("project without files", func(t *testing.T) {
		requireBoot(t)
		p := createProject(t, c, uniq("nofiles"), dockerfileApp())
		id := p["id"].(string)
		removeAllRetry(mainInst.Dirs.Deployments + "/" + id)
		c.Post(t, "/api/deployments/"+id+"/deploy", nil).ExpectError(t, 400, "Project files not found")
		c.Get(t, "/api/projects/"+id+"/build/detect").ExpectError(t, 400, "Project files not found")
		c.Get(t, "/api/files/"+id).ExpectError(t, 404, "Project files not found")
	})
}

func TestStopRestartCancelWithoutRuntime(t *testing.T) {
	requireNoDocker(t)
	c := admin.Client()
	name := uniq("lifecycle")
	p := createProject(t, c, name, dockerfileApp())
	id := p["id"].(string)

	// Deliberate fix 8: Go answers both with the same JSON 400 (the dashboard
	// shows `error` of any non-OK response); TS sent a 200 text error and a 500.
	runtimeMissing := "DockLift runtime configuration is missing. Deploy the project first."
	t.Run("stop before first deploy", func(t *testing.T) {
		r := c.Post(t, "/api/deployments/"+id+"/stop", nil)
		if targetIsGo() {
			r.ExpectError(t, 400, runtimeMissing)
		} else {
			r.ExpectStatus(t, 200)
			if string(r.Body) != "\n❌ Error: "+runtimeMissing+"\n" {
				t.Fatalf("stop body = %q", r.Body)
			}
		}
		c.Get(t, "/api/deployments/"+id).Expect(t, 200, Arr{})
	})
	t.Run("restart before first deploy", func(t *testing.T) {
		if targetIsGo() {
			c.Post(t, "/api/deployments/"+id+"/restart", nil).ExpectError(t, 400, runtimeMissing)
		} else {
			c.Post(t, "/api/deployments/"+id+"/restart", nil).ExpectError(t, 500, "Restart failed")
		}
		c.Get(t, "/api/deployments/"+id).Expect(t, 200, Arr{})
	})
	t.Run("idle cancel", func(t *testing.T) {
		r := c.Post(t, "/api/deployments/"+id+"/cancel", nil)
		r.ExpectStatus(t, 200)
		if ct := r.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Fatalf("cancel Content-Type = %q", ct)
		}
		body := string(r.Body)
		if !strings.HasPrefix(body, "❌ Cancelling — tearing down so you can start fresh...\n") {
			t.Fatalf("cancel body = %q", truncate(body, 300))
		}
		alias := "dl-" + name + "-" + shortID(id)
		expectContains(t, "cancel body", body,
			"ℹ️ No tracked build process — tearing down containers if present...\n",
			"ℹ️ No active deploy row — tearing down containers; history left unchanged.\n",
			"🔌 Detaching linked databases from project network...\n",
			"🛑 compose down -p "+alias+"\n",
			"🛑 compose down -p "+id+"\n",
			"❌ Cancel teardown incomplete for \""+alias+"\"\n",
			"❌ Compose teardown failed; project status unchanged\n")
		// The cancel lock is released afterwards.
		c.Post(t, "/api/deployments/"+id+"/cancel", nil).ExpectStatus(t, 200)
		c.Get(t, "/api/deployments/"+id).Expect(t, 200, Arr{})
	})
}

// TestDeployInProgressConflicts races requests against a running deploy. The
// deploy holds the project lock from its first log line until docker build
// fails, which is the window these requests land in.
func TestDeployInProgressConflicts(t *testing.T) {
	requireNoDocker(t)
	c := admin.Client()
	want := map[string]string{
		"deploy":   "A deployment is already running for this project",
		"redeploy": "A deployment is already running for this project",
		"stop":     "Cannot stop while a deployment is running; cancel it first",
		"restart":  "Cannot restart while a deployment is running",
		"delete":   "Cannot delete project while a deployment is in progress",
	}
	var last map[string]*Resp
	for attempt := 0; attempt < 5; attempt++ {
		p := createProject(t, c, uniq("race"), dockerfileApp())
		id := p["id"].(string)
		s := c.Open(t, "POST", "/api/deployments/"+id+"/deploy", nil)
		if _, ok := s.ReadUntil("🚀 Starting deployment...\n", 30*time.Second); !ok {
			t.Fatal("deploy did not start")
		}
		probes := map[string]func() (*Resp, error){
			"deploy":   func() (*Resp, error) { return c.raw("POST", "/api/deployments/"+id+"/deploy", nil, nil) },
			"redeploy": func() (*Resp, error) { return c.raw("POST", "/api/deployments/"+id+"/redeploy", nil, nil) },
			"stop":     func() (*Resp, error) { return c.raw("POST", "/api/deployments/"+id+"/stop", nil, nil) },
			"restart":  func() (*Resp, error) { return c.raw("POST", "/api/deployments/"+id+"/restart", nil, nil) },
			"delete":   func() (*Resp, error) { return c.raw("DELETE", "/api/projects/"+id, nil, nil) },
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		got := map[string]*Resp{}
		for name, fn := range probes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := fn()
				if err != nil {
					r = &Resp{Status: -1, Body: []byte(err.Error())}
				}
				mu.Lock()
				got[name] = r
				mu.Unlock()
			}()
		}
		wg.Wait()
		body := s.ReadAll()
		last = got
		allConflict := true
		for name, r := range got {
			if r.Status != 409 || !strings.Contains(string(r.Body), want[name]) {
				allConflict = false
				t.Logf("attempt %d: %s -> %d %s (deploy finished before the probe?)", attempt, name, r.Status, truncate(string(r.Body), 200))
			}
		}
		if !allConflict {
			continue
		}
		for name, r := range got {
			r.Method, r.Path = "probe", name
			r.ExpectError(t, 409, want[name])
		}
		expectContains(t, "racing deploy log", body, "\n❌ Error: ")
		// The rejected second deploy creates no history row of its own; only the racing deploy is recorded.
		hist := c.Get(t, "/api/deployments/"+id).Arr(t)
		if len(hist) != 1 {
			t.Fatalf("history after race = %d rows: %v", len(hist), short(hist))
		}
		// The lock is released once the deploy ends (the failed deploy wrote
		// no runtime compose, so Go answers the deliberate-fix-8 400, not a 409).
		if targetIsGo() {
			c.Post(t, "/api/deployments/"+id+"/stop", nil).ExpectError(t, 400, "DockLift runtime configuration is missing. Deploy the project first.")
		} else {
			c.Post(t, "/api/deployments/"+id+"/stop", nil).ExpectStatus(t, 200)
		}
		return
	}
	t.Fatalf("never observed every probe inside the deploy window; last: %v", last)
}

func TestServiceDomainsAndSSL(t *testing.T) {
	c := admin.Client()
	p1, s1 := deployedProject(t, c, "svcdom")
	p2, s2 := deployedProject(t, c, "svcdom2")
	id1, sid1 := p1["id"].(string), s1["id"].(string)
	id2, sid2 := p2["id"].(string), s2["id"].(string)
	svc := func(pid, sid, suffix string) string { return "/api/deployments/" + pid + "/services/" + sid + suffix }
	host := "svc-" + randHex(4) + ".contract-test.dev"
	missing := Obj{"status": "missing", "domain": Str, "expiresAt": nil, "error": nil, "diagnosticCommand": nil, "certPath": nil}
	if targetIsGo() {
		// SSL status responses are certs.Report: the six keys plus the expiry health.
		missing = missing.merge(Obj{"expired": false, "renewalOverdue": false, "daysUntilExpiry": nil})
	}
	event := func(msg string) Obj { return Obj{"at": ISOms, "level": "info", "message": msg} }

	c.Get(t, svc(id1, sid1, "/ssl")).Expect(t, 200, Obj{"ssl": Obj{}, "events": Arr{}})
	c.Get(t, svc(id2, sid1, "/ssl")).ExpectError(t, 404, "Service not found")
	c.Post(t, svc(id1, sid1, "/ssl/retry"), nil).ExpectError(t, 400, "Service has no domain or is not deployed")
	c.Post(t, svc(id2, sid1, "/ssl/retry"), nil).ExpectError(t, 404, "Service not found")
	c.Post(t, svc(id2, sid1, "/dns-check"), map[string]any{}).ExpectError(t, 404, "Service not found")

	c.Put(t, svc(id1, sid1, ""), map[string]any{"domain": 5}).ExpectError(t, 400, "Domain must be a comma-separated string")
	c.Put(t, svc(id1, sid1, ""), map[string]any{"domain": []string{"a.b"}}).ExpectError(t, 400, "Domain must be a comma-separated string")
	c.Put(t, svc(id1, sid1, ""), map[string]any{"domain": "bad_domain.test"}).ExpectError(t, 400, domainFormatMsg)
	c.Put(t, svc(id2, sid1, ""), map[string]any{"domain": host}).ExpectError(t, 404, "Service not found or access denied")

	upper := strings.ToUpper(host[:1]) + host[1:]
	ssl := c.Put(t, svc(id1, sid1, ""), map[string]any{"domain": upper + ", " + host + ",www." + host}).Expect(t, 200, Obj{
		"success": true,
		"ssl":     Obj{host: missing.merge(Obj{"domain": host}), "www." + host: missing.merge(Obj{"domain": "www." + host})},
		"events":  Arr{event("Domain added: " + host + ", www." + host)},
	})
	_ = ssl
	match(t, "service domain", serviceRow(Obj{"id": sid1, "domain": host + ", www." + host}), findBy(c.Get(t, "/api/deployments/"+id1+"/services").Arr(t), "id", sid1))
	c.Get(t, svc(id1, sid1, "/ssl")).Expect(t, 200, Obj{"ssl": AnyObj, "events": Arr{event("Domain added: " + host + ", www." + host)}})

	// Hostnames are owned by one service.
	c.Put(t, svc(id2, sid2, ""), map[string]any{"domain": "WWW." + host}).ExpectError(t, 409, `Hostname "www.`+host+`" is already assigned to another service`)
	// Re-saving your own hostname is fine.
	c.Put(t, svc(id1, sid1, ""), map[string]any{"domain": host}).Expect(t, 200, Obj{"success": true, "ssl": Obj{host: AnyObj}, "events": Arr{event("Domain added: " + host + ", www." + host)}})

	c.Post(t, svc(id1, sid1, "/ssl/retry"), nil).Expect(t, 200, Obj{"success": true, "ssl": Obj{host: missing.merge(Obj{"domain": host})},
		"events": Arr{event("Manual SSL retry requested.")}})

	t.Run("dns-check input filtering", func(t *testing.T) {
		c.Post(t, svc(id1, sid1, "/dns-check"), map[string]any{"domains": []any{"not a domain", "a,b.test", 7, ""}}).
			Expect(t, 200, Obj{"checks": Arr{}, "serverIp": OrNull(Str)})
		c.Post(t, svc(id1, sid1, "/dns-check"), map[string]any{"domains": []any{}}).Expect(t, 200, Obj{"checks": Arr{}, "serverIp": OrNull(Str)})
	})
	t.Run("dns-check real lookups", func(t *testing.T) {
		requireNetwork(t)
		c.Post(t, svc(id1, sid1, "/dns-check"), nil).Expect(t, 200, Obj{"checks": ArrayOf(Obj{"domain": Str, "status": Either("ok", "mismatch", "missing", "unknown"),
			"a": AnyArr, "aaaa": AnyArr, "serverIp": OrNull(Str), "message": Str}, 1), "serverIp": OrNull(Str)})
	})

	if targetIsGo() {
		// Deliberate fix 5: omitting the field leaves the mapping unchanged;
		// an explicit "" clears it.
		c.Put(t, svc(id1, sid1, ""), map[string]any{}).Expect(t, 200, Obj{"success": true, "ssl": Obj{host: AnyObj}, "events": AnyArr})
		match(t, "unchanged domain", serviceRow(Obj{"id": sid1, "domain": host}), findBy(c.Get(t, "/api/deployments/"+id1+"/services").Arr(t), "id", sid1))
		c.Put(t, svc(id1, sid1, ""), map[string]any{"domain": ""}).Expect(t, 200, Obj{"success": true, "ssl": Obj{}, "events": Arr{}})
	} else {
		// Omitting the field clears the mapping (normalizeDomainList(undefined) => []).
		c.Put(t, svc(id1, sid1, ""), map[string]any{}).Expect(t, 200, Obj{"success": true, "ssl": Obj{}, "events": Arr{}})
	}
	match(t, "cleared domain", serviceRow(Obj{"id": sid1, "domain": nil}), findBy(c.Get(t, "/api/deployments/"+id1+"/services").Arr(t), "id", sid1))
	// Now the second service may take it.
	c.Put(t, svc(id2, sid2, ""), map[string]any{"domain": host}).ExpectStatus(t, 200)
	c.Put(t, svc(id2, sid2, ""), map[string]any{"domain": nil}).Expect(t, 200, Obj{"success": true, "ssl": Obj{}, "events": Arr{}})
}

func TestDeploymentLogsAndStats(t *testing.T) {
	requireNoDocker(t)
	c := admin.Client()
	p, svc := deployedProject(t, c, "logs")
	id := p["id"].(string)
	if targetIsGo() {
		// Deliberate fix 9: an unreachable Docker is a 503 (the dashboard treats
		// any non-OK stats response as "no stats").
		c.Get(t, "/api/deployments/"+id+"/logs").ExpectError(t, 503, "Docker is not available")
		c.Get(t, "/api/deployments/"+id+"/stats").ExpectError(t, 503, "Docker is not available")
	} else {
		c.Get(t, "/api/deployments/"+id+"/logs").Expect(t, 200, Obj{"logs": Obj{"app": ""}})
		c.Get(t, "/api/deployments/"+id+"/stats").Expect(t, 200, Obj{"app": nil})
	}
	_ = svc
}
