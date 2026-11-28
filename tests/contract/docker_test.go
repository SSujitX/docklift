package contract

import (
	"strings"
	"testing"
)

// Docker-backed success paths. They run only with DOCKLIFT_DOCKER=1 against a
// server that can reach a Docker daemon (see README.md).

func TestDockerDeployLifecycle(t *testing.T) {
	requireDocker(t)
	c := admin.Client()
	name := uniq("dock")
	p := createProject(t, c, name, files("Dockerfile", "FROM alpine:3.20\nCMD [\"sleep\", \"3600\"]\n"))
	id := p["id"].(string)

	log := deployAndWait(t, c, id)
	expectContains(t, "deploy log", log, "✅ DEPLOY SUCCESSFUL!", "📊 Deployment complete! Status: SUCCESS ✅")
	d := lastDeployment(t, c, id)
	match(t, "success row", deploymentRow(Obj{"project_id": id, "status": "success", "image_tags": Obj{"app": Prefix("docklift-" + id[:8] + "-app:")}, "finished_at": ISOms}), d)
	c.Get(t, "/api/projects/"+id).Expect(t, 200, projectRow(Obj{"id": id, "name": name, "status": "running"}))
	c.Get(t, "/api/deployments/"+id+"/stats").Expect(t, 200, Obj{"app": Obj{"cpu_percent": Str, "memory_usage": Contains(" MB"), "memory_limit": Contains(" MB"), "memory_percent": Str}})
	c.Get(t, "/api/deployments/"+id+"/logs").Expect(t, 200, Obj{"logs": Obj{"app": Str}})
	c.Get(t, "/api/logs/"+id+"/containers").Expect(t, 200, ArrayOf(Obj{"id": UUID, "name": "app", "container_name": Str, "port": nil, "internal_port": Int,
		"dockerfile_path": "Dockerfile", "status": "running", "running": true}, 1))

	r := c.Post(t, "/api/deployments/"+id+"/restart", nil)
	r.ExpectStatus(t, 200)
	expectContains(t, "restart log", string(r.Body), "RESTART SUCCESSFUL")
	r = c.Post(t, "/api/deployments/"+id+"/stop", nil)
	r.ExpectStatus(t, 200)
	expectContains(t, "stop log", string(r.Body), "✅ STOP SUCCESSFUL!")
	c.Get(t, "/api/projects/"+id).Expect(t, 200, projectRow(Obj{"id": id, "name": name, "status": "stopped"}))

	// A second success makes the first one a rollback target.
	deployAndWait(t, c, id)
	r = c.Post(t, "/api/deployments/"+id+"/rollback", map[string]any{"password": admin.Password, "deploymentId": d["id"]})
	r.ExpectStatus(t, 200)
	expectContains(t, "rollback log", string(r.Body), "ROLLBACK SUCCESSFUL")

	c.Delete(t, "/api/projects/"+id).Expect(t, 200, Obj{"status": "deleted"})
	c.Get(t, "/api/projects/"+id).ExpectError(t, 404, "Project not found")
}

func TestDockerStorage(t *testing.T) {
	requireDocker(t)
	c := admin.Client()
	p, _ := deployedProject(t, c, "dockstore")
	id := p["id"].(string)
	row := c.Post(t, "/api/projects/"+id+"/storage", map[string]any{"name": "data", "service_name": "app", "mount_path": "/data/"}).Expect(t, 201, Obj{
		"id": UUID, "project_id": id, "service_name": "app", "name": Prefix("dl-" + strings.ReplaceAll(id, "-", "")[:8] + "-data-"), "display_name": "data", "mount_path": "/data", "created_at": ISOms,
	}).(map[string]any)
	c.Post(t, "/api/projects/"+id+"/storage", map[string]any{"name": "data", "service_name": "app", "mount_path": "/other"}).ExpectError(t, 409, "That storage name or mount path already exists")
	c.Get(t, "/api/projects/"+id+"/storage").Expect(t, 200, Arr{row})
	c.Delete(t, "/api/projects/"+id+"/storage/"+row["id"].(string)+"?removeVolume=true").Expect(t, 200, Obj{"status": "deleted"})
}
