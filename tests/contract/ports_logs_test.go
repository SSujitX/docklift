package contract

import (
	"fmt"
	"testing"
)

func TestPorts(t *testing.T) {
	c := admin.Client()
	pending := createProject(t, c, uniq("ports-pending"), dockerfileApp())
	publish := createProject(t, c, uniq("ports-publish"), dockerfileApp())
	c.Patch(t, "/api/projects/"+publish["id"].(string), map[string]any{"publish_host_port": true}).ExpectStatus(t, 200)

	got := c.Get(t, "/api/ports").Expect(t, 200, Obj{
		"ports":           ArrayOf(Either(Obj{"port": Int, "project_id": nil, "is_locked": false}, Obj{"port": Int, "project_id": OrNull(UUID), "is_locked": OrNull(Bool), "project": OrNull(Obj{"id": UUID, "name": Str, "status": OrNull(Str)})}), portRangeEnd-portRangeStart+1),
		"private_running": AnyArr,
		"pool":            Obj{"start": portRangeStart, "end": portRangeEnd},
	}).(map[string]any)
	ports := got["ports"].([]any)
	if len(ports) != portRangeEnd-portRangeStart+1 {
		t.Fatalf("want one entry per pool port, got %d", len(ports))
	}
	for i, p := range ports {
		if p.(map[string]any)["port"] != float64(portRangeStart+i) {
			t.Fatalf("ports[%d] = %v", i, p)
		}
	}
	priv := got["private_running"].([]any)
	match(t, "pending project", Obj{"id": pending["id"], "name": pending["name"], "status": "pending", "project_type": "app", "db_engine": nil,
		"publish_host_port": false, "reason": "Private by default — add a domain or enable Publish host ports + redeploy"}, findBy(priv, "id", pending["id"]))
	match(t, "publish project", Obj{"id": publish["id"], "name": publish["name"], "status": "pending", "project_type": "app", "db_engine": nil,
		"publish_host_port": true, "reason": "Publish host ports is on — redeploy to claim a pool port"}, findBy(priv, "id", publish["id"]))

	c.Delete(t, "/api/ports/abc").ExpectError(t, 400, "Invalid port")
	c.Delete(t, "/api/ports/"+fmt.Sprint(portRangeEnd)).Expect(t, 200, Obj{"status": "deleted"})
	c.Delete(t, "/api/ports/99999").Expect(t, 200, Obj{"status": "deleted"})
	// parseInt semantics: a numeric prefix is accepted.
	c.Delete(t, "/api/ports/77abc").Expect(t, 200, Obj{"status": "deleted"})
	mainInst.Anon().Get(t, "/api/ports").ExpectError(t, 401, "Authentication required")

	t.Run("deploy claims a pool port", func(t *testing.T) {
		requireNoDocker(t)
		id := publish["id"].(string)
		log := deployAndWait(t, c, id)
		expectContains(t, "deploy log", log, "🔓 Host port publish enabled for this project", "(Host port: ")
		svc := c.Get(t, "/api/deployments/"+id+"/services").Arr(t)[0].(map[string]any)
		port := svc["port"]
		match(t, "service port", Int, port)
		portStr := fmt.Sprint(int(port.(float64)))
		list := c.Get(t, "/api/ports").Obj(t)
		entry := findBy(list["ports"].([]any), "port", port)
		match(t, "allocated entry", Obj{"port": port, "project_id": id, "is_locked": true, "project": Obj{"id": id, "name": publish["name"], "status": Str}}, entry)
		if findBy(list["private_running"].([]any), "id", id) != nil {
			t.Fatal("a project holding a pool port is not private")
		}
		c.Get(t, "/api/projects/"+id).Expect(t, 200, projectRow(Obj{"id": id, "name": publish["name"], "port": port, "publish_host_port": true, "status": "stopped"}))
		c.Delete(t, "/api/ports/"+portStr).ExpectError(t, 409, "Port is still assigned to a service or project")
		// Redeploy keeps the same port.
		expectContains(t, "redeploy log", deployAndWait(t, c, id), "Updating existing service: app (Host port: "+portStr+")")
		// Turning publish off releases it on the next deploy.
		c.Patch(t, "/api/projects/"+id, map[string]any{"publish_host_port": false}).ExpectStatus(t, 200)
		expectContains(t, "release log", deployAndWait(t, c, id), "Updating existing service: app (host port released)")
		entry = findBy(c.Get(t, "/api/ports").Obj(t)["ports"].([]any), "port", port)
		match(t, "released entry", Obj{"port": port, "project_id": nil, "is_locked": false, "project": nil}, entry)
		// The project row still remembers the port, so delete stays refused.
		c.Delete(t, "/api/ports/"+portStr).ExpectError(t, 409, "Port is still assigned to a service or project")
	})
}

func TestContainerLogs(t *testing.T) {
	c := admin.Client()
	p, svc := deployedProject(t, c, "clogs")
	id, cname := p["id"].(string), svc["container_name"].(string)
	t.Run("containers", func(t *testing.T) {
		requireNoDocker(t)
		c.Get(t, "/api/logs/"+id+"/containers").Expect(t, 200, Arr{Obj{
			"id": svc["id"], "name": "app", "container_name": cname, "port": nil, "internal_port": 8080, "dockerfile_path": "Dockerfile",
			"status": "not_found", "running": false,
		}})
	})
	empty := createProject(t, c, uniq("clogs-empty"), dockerfileApp())
	c.Get(t, "/api/logs/"+empty["id"].(string)+"/containers").Expect(t, 200, Arr{})
	c.Get(t, "/api/logs/"+missingProjectID+"/containers").ExpectError(t, 404, "Project not found")
	// The container list is Bearer-only; SSE tokens are refused there.
	sse := c.Post(t, "/api/auth/sse-token", nil).Obj(t)["token"].(string)
	mainInst.Anon().Get(t, "/api/logs/"+id+"/containers?token="+sse).ExpectError(t, 401, "Authentication required")
	mainInst.As(sse).Get(t, "/api/logs/"+id+"/containers").ExpectError(t, 401, "Invalid token for this endpoint")

	stream := "/api/logs/" + id + "/stream/" + cname
	t.Run("stream auth", func(t *testing.T) {
		mainInst.Anon().Get(t, stream).ExpectError(t, 401, "SSE token required")
		// Bearer session tokens do not work for SSE; only ?token= with purpose sse.
		c.Get(t, stream).ExpectError(t, 401, "SSE token required")
		mainInst.Anon().Get(t, stream+"?token="+admin.Token).ExpectError(t, 401, "SSE token required")
		term := c.Post(t, "/api/auth/terminal-token", nil).Obj(t)["token"].(string)
		mainInst.Anon().Get(t, stream+"?token="+term).ExpectError(t, 401, "SSE token required")
		mainInst.Anon().Get(t, stream+"?token=garbage").ExpectError(t, 401, "Invalid token")
	})
	t.Run("stream ownership", func(t *testing.T) {
		mainInst.Anon().Get(t, "/api/logs/"+empty["id"].(string)+"/stream/"+cname+"?token="+sse).ExpectError(t, 404, "Container not found for this project")
		mainInst.Anon().Get(t, "/api/logs/"+id+"/stream/docklift-backend?token="+sse).ExpectError(t, 404, "Container not found for this project")
	})
	t.Run("stream without docker", func(t *testing.T) {
		requireNoDocker(t)
		r := mainInst.Anon().Get(t, stream+"?token="+sse+"&tail=5")
		r.ExpectStatus(t, 200)
		assertSSEHeaders(t, r)
		ev := sseEvents(t, string(r.Body))
		if len(ev) != 1 {
			t.Fatalf("want one SSE event, got %v", ev)
		}
		match(t, "error event", Obj{"type": "error", "message": Prefix("Container not found: ")}, ev[0])
	})
}

func assertSSEHeaders(t *testing.T, r *Resp) {
	t.Helper()
	for k, v := range map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-cache, no-transform", "X-Accel-Buffering": "no"} {
		if got := r.Header.Get(k); got != v {
			t.Errorf("SSE %s = %q, want %q", k, got, v)
		}
	}
}
