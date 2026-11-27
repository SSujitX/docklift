package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	nameRequiredMsg   = "Project name is required (max 120 characters)"
	sourceRequiredMsg = "A valid source is required: GitHub repository URL or ZIP upload"
	githubURLMsg      = "A valid GitHub repository URL is required"
	domainFormatMsg   = "Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com)."
	missingProjectID  = "5eed0000-0000-4000-8000-00000000beef"
)

func projectNames(t *testing.T, c *Client) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, p := range c.Get(t, "/api/projects").Arr(t) {
		names[p.(map[string]any)["name"].(string)] = true
	}
	return names
}

func TestProjectCreateValidation(t *testing.T) {
	c := admin.Client()
	okZip := buildZip(t, dockerfileApp()...)
	cases := []struct {
		name string
		body any
		code int
		msg  string
	}{
		{"empty body", map[string]any{}, 400, nameRequiredMsg},
		{"blank name", map[string]any{"name": "   ", "github_url": "https://github.com/a/b"}, 400, nameRequiredMsg},
		{"non-string name", map[string]any{"name": 7, "github_url": "https://github.com/a/b"}, 400, nameRequiredMsg},
		{"name too long", map[string]any{"name": strings.Repeat("n", 121), "github_url": "https://github.com/a/b"}, 400, nameRequiredMsg},
		{"no source", map[string]any{"name": "x"}, 400, sourceRequiredMsg},
		{"unknown source type", map[string]any{"name": "x", "source_type": "ftp"}, 400, sourceRequiredMsg},
		{"blank github url", map[string]any{"name": "x", "github_url": "   "}, 400, sourceRequiredMsg},
		{"upload without zip", map[string]any{"name": "x", "source_type": "upload"}, 400, "ZIP upload is required for upload projects"},
		{"github without url", map[string]any{"name": "x", "source_type": "github"}, 400, githubURLMsg},
		{"non-github host", map[string]any{"name": "x", "github_url": "https://gitlab.com/a/b"}, 400, githubURLMsg},
		{"github owner only", map[string]any{"name": "x", "github_url": "https://github.com/only-owner"}, 400, githubURLMsg},
		{"github .git owner only", map[string]any{"name": "x", "github_url": "https://github.com/owner.git"}, 400, githubURLMsg},
		{"ssh style url", map[string]any{"name": "x", "github_url": "git@github.com:a/b.git"}, 400, githubURLMsg},
		{"non-http scheme", map[string]any{"name": "x", "github_url": "ftp://github.com/a/b"}, 400, githubURLMsg},
		{"lookalike host", map[string]any{"name": "x", "github_url": "https://github.com.evil.test/a/b"}, 400, githubURLMsg},
		// Domain is validated before any clone, so this never touches the network.
		{"github with bad domain", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "domain": "bad domain"}, 400, domainFormatMsg},
		{"github with injected domain", map[string]any{"name": "x", "github_url": "https://www.github.com/a/b", "domain": "a.com;evil"}, 400, domainFormatMsg},
		{"zip with bad domain", zipUpload("x", okZip, "domain", "ok.example.test, -bad.example.test"), 400, domainFormatMsg},
		{"zip with single label domain", zipUpload("x", okZip, "domain", "localhost"), 400, domainFormatMsg},
		{"zip under wrong field", &Multipart{Fields: [][2]string{{"name", "x"}}, Files: []FilePart{{Field: "zip", Filename: "a.zip", Data: okZip}}}, 400, "Upload failed"},
		{"two zip files", &Multipart{Fields: [][2]string{{"name", "x"}}, Files: []FilePart{{Field: "files", Filename: "a.zip", Data: okZip}, {Field: "files", Filename: "b.zip", Data: okZip}}}, 400, "Upload failed"},
		{"multipart without name", &Multipart{Files: []FilePart{{Field: "files", Filename: "a.zip", Data: okZip}}}, 400, nameRequiredMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.Post(t, "/api/projects", tc.body).ExpectError(t, tc.code, tc.msg)
		})
	}
	mainInst.Anon().Post(t, "/api/projects", map[string]any{"name": "x"}).ExpectError(t, 401, "Authentication required")
}

func TestProjectCreateUnsafeZip(t *testing.T) {
	c := admin.Client()
	cases := map[string][]byte{
		"not a zip":           []byte("this is not a zip archive"),
		"parent traversal":    buildZip(t, zipEntry{Name: "ok.txt", Body: "ok"}, zipEntry{Name: "../escape.txt", Body: "x"}),
		"nested traversal":    buildZip(t, zipEntry{Name: "a/../../escape.txt", Body: "x"}),
		"absolute path":       buildZip(t, zipEntry{Name: "/abs.txt", Body: "x"}),
		"backslash traversal": buildZip(t, zipEntry{Name: `..\escape.txt`, Body: "x"}),
	}
	for label, data := range cases {
		t.Run(label, func(t *testing.T) {
			name := uniq("unsafe")
			c.Post(t, "/api/projects", zipUpload(name, data)).ExpectError(t, 400, "Invalid or unsafe ZIP upload")
			if projectNames(t, c)[name] {
				t.Fatalf("project %s must be rolled back", name)
			}
		})
	}
	if mainInst.Booted() {
		if _, err := os.Stat(filepath.Join(filepath.Dir(mainInst.Dirs.Deployments), "escape.txt")); !os.IsNotExist(err) {
			t.Fatalf("zip-slip wrote outside the deployments dir")
		}
	}
}

func TestProjectCreateUploadFields(t *testing.T) {
	c := admin.Client()
	name := uniq("fields")
	r := c.Post(t, "/api/projects", zipUpload(name, buildZip(t, dockerfileApp()...),
		"description", "contract project", "domain", "App.Contract-Test.dev, www.contract-test.dev", "build_type", "dockerfile",
		"base_directory", "  .  ", "dockerfile_path", " Dockerfile ", "internal_port", "8080", "project_type", "worker",
		"github_branch", "ignored-for-upload"))
	r.Expect(t, 201, projectRow(Obj{
		"name": name, "description": "contract project", "domain": "App.Contract-Test.dev, www.contract-test.dev",
		"build_type": "dockerfile", "dockerfile_path": "Dockerfile", "internal_port": 8080, "project_type": "worker",
		"github_branch": "ignored-for-upload",
	}))
	for in, want := range map[string]int{"70000": 65535, "abc": 3000, "0": 3000, "-4": 1, "443.9": 443} {
		p := c.Post(t, "/api/projects", zipUpload(uniq("port"), buildZip(t, dockerfileApp()...), "internal_port", in)).Obj(t)
		if got := p["internal_port"]; got != float64(want) {
			t.Errorf("internal_port %q -> %v, want %d", in, got, want)
		}
	}
	p := c.Post(t, "/api/projects", zipUpload(uniq("bt"), buildZip(t, dockerfileApp()...), "build_type", "nonsense", "base_directory", "")).Obj(t)
	match(t, "normalized", Obj{"build_type": "auto", "base_directory": "."}, map[string]any{"build_type": p["build_type"], "base_directory": p["base_directory"]})
	// source_type=upload is explicit-able, and an explicit source wins over a github_url field.
	p = c.Post(t, "/api/projects", zipUpload(uniq("explicit"), buildZip(t, dockerfileApp()...), "source_type", "upload", "github_url", "https://github.com/a/b")).Obj(t)
	match(t, "explicit upload", projectRow(Obj{"name": Str}), p)
}

func TestProjectReadUpdate(t *testing.T) {
	c := admin.Client()
	first := createProject(t, c, uniq("older"), dockerfileApp())
	second := createProject(t, c, uniq("newer"), dockerfileApp())
	id := first["id"].(string)

	t.Run("list is newest first", func(t *testing.T) {
		list := c.Get(t, "/api/projects").Arr(t)
		pos := map[string]int{}
		for i, p := range list {
			m := p.(map[string]any)
			pos[m["id"].(string)] = i
			if m["id"] == id {
				match(t, "listed row", projectRow(Obj{"id": id, "name": first["name"]}), m)
			}
		}
		if pos[second["id"].(string)] > pos[id] {
			t.Fatalf("newer project must be listed first")
		}
	})
	t.Run("get", func(t *testing.T) {
		c.Get(t, "/api/projects/"+id).Expect(t, 200, projectRow(Obj{"id": id, "name": first["name"], "created_at": first["created_at"]}))
		c.Get(t, "/api/projects/"+missingProjectID).ExpectError(t, 404, "Project not found")
		c.Get(t, "/api/projects/not-a-uuid").ExpectError(t, 404, "Project not found")
	})
	t.Run("patch", func(t *testing.T) {
		got := c.Patch(t, "/api/projects/"+id, map[string]any{
			"name": "renamed-" + randHex(3), "description": "desc", "domain": "patched.contract-test.dev", "build_type": "railpack",
			"base_directory": "  src  ", "dockerfile_path": "  docker/Dockerfile ", "internal_port": 99999, "publish_host_port": "yes",
			"project_type": "worker", "github_url": "not validated on patch",
		}).Expect(t, 200, projectRow(Obj{
			"id": id, "name": Prefix("renamed-"), "description": "desc", "domain": "patched.contract-test.dev", "build_type": "railpack",
			"base_directory": "src", "dockerfile_path": "docker/Dockerfile", "internal_port": 65535, "publish_host_port": true,
			"project_type": "worker", "github_url": "not validated on patch", "created_at": first["created_at"],
		})).(map[string]any)
		if got["updated_at"].(string) < first["updated_at"].(string) {
			t.Fatalf("updated_at must advance")
		}
		// Falsy name/project_type are ignored; explicit nulls/empties normalise.
		c.Patch(t, "/api/projects/"+id, map[string]any{
			"name": "", "project_type": "", "description": nil, "domain": nil, "dockerfile_path": "", "base_directory": "",
			"internal_port": "0", "publish_host_port": 0, "build_type": "weird",
		}).Expect(t, 200, projectRow(Obj{
			"id": id, "name": got["name"], "project_type": "worker", "description": nil, "domain": nil, "dockerfile_path": nil,
			"base_directory": ".", "internal_port": 3000, "publish_host_port": false, "build_type": "auto",
			"github_url": "not validated on patch",
		}))
		c.Patch(t, "/api/projects/"+id, map[string]any{"internal_port": -5}).Expect(t, 200, projectRow(Obj{
			"id": id, "name": got["name"], "project_type": "worker", "internal_port": 1, "github_url": "not validated on patch",
		}))
		c.Patch(t, "/api/projects/"+id, map[string]any{"domain": "bad_domain"}).ExpectError(t, 400, domainFormatMsg)
		c.Patch(t, "/api/projects/"+missingProjectID, map[string]any{"name": "x"}).ExpectError(t, 500, "Failed to update project")
		mainInst.Anon().Patch(t, "/api/projects/"+id, map[string]any{"name": "x"}).ExpectError(t, 401, "Authentication required")
	})
}

func TestProjectBuildDetect(t *testing.T) {
	c := admin.Client()
	detect := func(t *testing.T, entries []zipEntry, fields ...string) *Resp {
		t.Helper()
		p := createProject(t, c, uniq("detect"), entries, fields...)
		return c.Get(t, "/api/projects/"+p["id"].(string)+"/build/detect")
	}
	t.Run("root Dockerfile", func(t *testing.T) {
		detect(t, dockerfileApp()).Expect(t, 200, Obj{"requestedType": "auto", "resolvedType": "dockerfile", "baseDirectory": ".",
			"dockerfilePath": "Dockerfile", "detected": "1 repository Dockerfile", "manifests": Arr{"package.json"}})
	})
	t.Run("nested Dockerfile", func(t *testing.T) {
		detect(t, files("services/web/Dockerfile", "FROM alpine\n", "README.md", "x")).Expect(t, 200, Obj{"requestedType": "auto",
			"resolvedType": "dockerfile", "baseDirectory": ".", "dockerfilePath": "services/web/Dockerfile", "detected": "1 repository Dockerfile", "manifests": Arr{}})
	})
	t.Run("Dockerfile below scan depth falls back to railpack", func(t *testing.T) {
		detect(t, files("a/b/c/Dockerfile", "FROM alpine\n")).Expect(t, 200, Obj{"requestedType": "auto", "resolvedType": "railpack",
			"baseDirectory": ".", "dockerfilePath": nil, "detected": "Railpack source (framework detection runs during build)", "manifests": Arr{}})
	})
	t.Run("ignored dirs are not scanned", func(t *testing.T) {
		detect(t, files("node_modules/x/Dockerfile", "FROM alpine\n", "requirements.txt", "flask\n", "package.json", "{}")).Expect(t, 200, Obj{
			"requestedType": "auto", "resolvedType": "railpack", "baseDirectory": ".", "dockerfilePath": nil,
			"detected": "Railpack source (package.json, requirements.txt)", "manifests": Arr{"package.json", "requirements.txt"}})
	})
	t.Run("base directory", func(t *testing.T) {
		detect(t, files("apps/api/go.mod", "module x\n", "apps/api/main.go", "package main\n"), "base_directory", "apps/api").Expect(t, 200, Obj{
			"requestedType": "auto", "resolvedType": "railpack", "baseDirectory": "apps/api", "dockerfilePath": nil,
			"detected": "Railpack source (go.mod)", "manifests": Arr{"go.mod"}})
	})
	t.Run("forced dockerfile", func(t *testing.T) {
		detect(t, files("docker/Prod.Dockerfile", "FROM alpine\n"), "build_type", "dockerfile", "dockerfile_path", "docker/Prod.Dockerfile").Expect(t, 200, Obj{
			"requestedType": "dockerfile", "resolvedType": "dockerfile", "baseDirectory": ".", "dockerfilePath": "docker/Prod.Dockerfile",
			"detected": "Configured Dockerfile", "manifests": Arr{}})
	})
	t.Run("forced railpack ignores Dockerfiles", func(t *testing.T) {
		detect(t, dockerfileApp(), "build_type", "railpack").Expect(t, 200, Obj{"requestedType": "railpack", "resolvedType": "railpack",
			"baseDirectory": ".", "dockerfilePath": nil, "detected": "Railpack source (package.json)", "manifests": Arr{"package.json"}})
	})
	t.Run("errors", func(t *testing.T) {
		detect(t, files("x.txt", "x"), "build_type", "dockerfile", "dockerfile_path", "nope/Dockerfile").ExpectError(t, 400, "Dockerfile not found: nope/Dockerfile")
		detect(t, files("x.txt", "x"), "build_type", "dockerfile").ExpectError(t, 400, "Dockerfile not found: Dockerfile")
		detect(t, files("x.txt", "x"), "base_directory", "../outside").ExpectError(t, 400, "Build paths must stay inside the project directory")
		detect(t, files("x.txt", "x"), "base_directory", "missing").ExpectError(t, 400, "Base directory not found: missing")
		detect(t, files("x.txt", "x"), "base_directory", "x.txt").ExpectError(t, 400, "Base directory not found: x.txt")
		detect(t, files("x.txt", "x"), "build_type", "dockerfile", "dockerfile_path", "../../etc/passwd").ExpectError(t, 400, "Build paths must stay inside the project directory")
		c.Get(t, "/api/projects/"+missingProjectID+"/build/detect").ExpectError(t, 404, "Project not found")
	})
}

func TestProjectStorage(t *testing.T) {
	c := admin.Client()
	plain := createProject(t, c, uniq("storage-plain"), dockerfileApp())
	pid := plain["id"].(string)
	c.Get(t, "/api/projects/"+pid+"/storage").Expect(t, 200, Arr{})
	c.Get(t, "/api/projects/"+missingProjectID+"/storage").Expect(t, 200, Arr{})
	c.Post(t, "/api/projects/"+missingProjectID+"/storage", map[string]any{"name": "data"}).ExpectError(t, 404, "Project not found")
	nameMsg := "Storage name must use 1-32 letters, numbers, _ or -"
	for _, bad := range []any{nil, "", "-data", "_data", "da ta", "data/x", strings.Repeat("a", 33), "dätä"} {
		c.Post(t, "/api/projects/"+pid+"/storage", map[string]any{"name": bad, "service_name": "app", "mount_path": "/data"}).ExpectError(t, 400, nameMsg)
	}
	c.Post(t, "/api/projects/"+pid+"/storage", map[string]any{"name": "data", "service_name": "app", "mount_path": "/data"}).
		ExpectError(t, 400, "Select a valid project service")
	c.Delete(t, "/api/projects/"+pid+"/storage/x").ExpectError(t, 400, "Confirm permanent data deletion with removeVolume=true")
	c.Delete(t, "/api/projects/"+pid+"/storage/x?removeVolume=1").ExpectError(t, 400, "Confirm permanent data deletion with removeVolume=true")
	c.Delete(t, "/api/projects/"+pid+"/storage/x?removeVolume=true").ExpectError(t, 404, "Persistent storage not found")

	t.Run("mount path rules", func(t *testing.T) {
		project, _ := deployedProject(t, c, "storage")
		id := project["id"].(string)
		post := func(mount any) *Resp {
			return c.Post(t, "/api/projects/"+id+"/storage", map[string]any{"name": "data", "service_name": " app ", "mount_path": mount})
		}
		for _, m := range []any{"relative/path", "C:/data", "/data:ro", "/da\x00ta"} {
			post(m).ExpectError(t, 400, "Mount path must be an absolute container path")
		}
		for _, m := range []any{nil, "", "/", "//", "/etc", "/etc/", "/proc/1", "/sys", "/dev/shm", "/var/run", "/var/run/docker.sock"} {
			post(m).ExpectError(t, 400, "This system mount path is not allowed")
		}
		c.Post(t, "/api/projects/"+id+"/storage", map[string]any{"name": "data", "service_name": "web", "mount_path": "/data"}).
			ExpectError(t, 400, "Select a valid project service")
		t.Run("volume create without docker", func(t *testing.T) {
			requireNoDocker(t)
			post("/etcetera/data/").Expect(t, 500, Obj{"error": NonEmpty})
			// The row is rolled back when docker volume create fails.
			c.Get(t, "/api/projects/"+id+"/storage").Expect(t, 200, Arr{})
		})
	})
}

func TestProjectAutoDeployUpload(t *testing.T) {
	c := admin.Client()
	p := createProject(t, c, uniq("autodeploy"), dockerfileApp())
	id := p["id"].(string)
	c.Get(t, "/api/projects/"+id+"/auto-deploy").Expect(t, 200, Obj{"auto_deploy": false, "webhook_secret": nil, "webhook_url": nil, "available": false})
	c.Patch(t, "/api/projects/"+id+"/auto-deploy", map[string]any{"enabled": true}).ExpectError(t, 400, "Auto-deploy is only available for GitHub projects")
	c.Get(t, "/api/projects/"+missingProjectID+"/auto-deploy").ExpectError(t, 404, "Project not found")
	c.Patch(t, "/api/projects/"+missingProjectID+"/auto-deploy", map[string]any{"enabled": true}).ExpectError(t, 404, "Project not found")
}

func TestProjectDelete(t *testing.T) {
	c := admin.Client()
	t.Run("missing project", func(t *testing.T) {
		c.Delete(t, "/api/projects/"+missingProjectID).ExpectError(t, 500, "Failed to delete project")
	})
	t.Run("teardown failure keeps the project", func(t *testing.T) {
		requireNoDocker(t)
		p := createProject(t, c, uniq("delete"), dockerfileApp())
		id := p["id"].(string)
		r := c.Delete(t, "/api/projects/"+id)
		r.Expect(t, 409, Obj{"error": Prefix(`Cannot delete project: Docker teardown failed for "dl-delete-`)})
		c.Get(t, "/api/projects/"+id).ExpectStatus(t, 200)
		if mainInst.Booted() {
			if _, err := os.Stat(filepath.Join(mainInst.Dirs.Deployments, id)); err != nil {
				t.Fatalf("project files must survive a failed delete: %v", err)
			}
		}
	})
	t.Run("delete with docker", func(t *testing.T) {
		requireDocker(t)
		p := createProject(t, c, uniq("delete"), dockerfileApp())
		id := p["id"].(string)
		c.Delete(t, "/api/projects/"+id).Expect(t, 200, Obj{"status": "deleted"})
		c.Get(t, "/api/projects/"+id).ExpectError(t, 404, "Project not found")
	})
	mainInst.Anon().Delete(t, "/api/projects/"+missingProjectID).ExpectError(t, 401, "Authentication required")
}
