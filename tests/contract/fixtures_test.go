package contract

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type zipEntry struct {
	Name string
	Body string
	Dir  bool
}

// buildZip makes an in-memory ZIP; names are written verbatim (traversal
// names are allowed on purpose).
func buildZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for _, e := range entries {
		name := e.Name
		if e.Dir && !strings.HasSuffix(name, "/") {
			name += "/"
		}
		fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		if !e.Dir {
			fw.Write([]byte(e.Body))
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func files(kv ...string) []zipEntry {
	var out []zipEntry
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, zipEntry{Name: kv[i], Body: kv[i+1]})
	}
	return out
}

// dockerfileApp is a small upload with one root Dockerfile (service "app").
func dockerfileApp() []zipEntry {
	return files(
		"Dockerfile", "FROM alpine:3.20\nEXPOSE 8080\nCMD [\"sleep\", \"3600\"]\n",
		"index.html", "<h1>contract</h1>\n",
		"src/app.js", "console.log('contract')\n",
		"src/lib/util.js", "export const x = 1;\n",
		"logo.png", "\x89PNG fake",
		"package.json", "{\"name\":\"contract\"}\n",
		"node_modules/dep/index.js", "module.exports = 1\n",
		".git/config", "[core]\n",
	)
}

func zipUpload(name string, data []byte, fields ...string) *Multipart {
	mp := &Multipart{Files: []FilePart{{Field: "files", Filename: "project.zip", ContentType: "application/zip", Data: data}}}
	mp.Fields = append(mp.Fields, [2]string{"name", name})
	for i := 0; i+1 < len(fields); i += 2 {
		mp.Fields = append(mp.Fields, [2]string{fields[i], fields[i+1]})
	}
	return mp
}

// projectRow is the Prisma Project row for a freshly created upload project.
func projectRow(over Obj) Obj {
	row := Obj{
		"id": UUID, "name": Str, "description": nil, "source_type": "upload", "project_type": "app",
		"github_url": nil, "github_branch": nil, "domain": nil, "port": nil, "status": "pending",
		"container_name": nil, "auto_deploy": false, "webhook_secret": nil, "build_type": "auto",
		"base_directory": ".", "dockerfile_path": nil, "internal_port": 3000, "publish_host_port": false,
		"db_engine": nil, "created_at": ISOms, "updated_at": ISOms,
	}
	for k, v := range over {
		row[k] = v
	}
	return row
}

func serviceRow(over Obj) Obj {
	row := Obj{
		"id": UUID, "project_id": UUID, "name": Str, "dockerfile_path": Str, "container_name": OrNull(Str),
		"port": OrNull(Int), "internal_port": OrNull(Int), "domain": OrNull(Str), "status": OrNull(Str), "created_at": ISOms,
	}
	for k, v := range over {
		row[k] = v
	}
	return row
}

func deploymentRow(over Obj) Obj {
	row := Obj{
		"id": UUID, "project_id": UUID, "status": Str, "trigger": Str, "commit_message": OrNull(Str),
		"commit_sha": OrNull(Str), "image_tags": Any, "logs": OrNull(Str), "created_at": ISOms, "finished_at": OrNull(ISOms),
	}
	for k, v := range over {
		row[k] = v
	}
	return row
}

func envRow(over Obj) Obj {
	row := Obj{
		"id": UUID, "project_id": UUID, "service_name": "", "key": Str, "value": Str,
		"is_build_arg": false, "is_runtime": true, "is_secret": false, "created_at": ISOms,
	}
	for k, v := range over {
		row[k] = v
	}
	return row
}

// createProject uploads a ZIP project and returns its row.
func createProject(t *testing.T, c *Client, name string, entries []zipEntry, fields ...string) map[string]any {
	t.Helper()
	r := c.Post(t, "/api/projects", zipUpload(name, buildZip(t, entries...), fields...))
	if len(fields) == 0 {
		return r.Expect(t, 201, projectRow(Obj{"name": name})).(map[string]any)
	}
	r.ExpectStatus(t, 201)
	return r.Obj(t)
}

// deployAndWait runs a deploy and returns the streamed log.
func deployAndWait(t *testing.T, c *Client, projectID string) string {
	t.Helper()
	s := c.Open(t, "POST", "/api/deployments/"+projectID+"/deploy", nil)
	if s.Status != 200 {
		body := s.ReadAll()
		t.Fatalf("deploy %s: status %d: %s", projectID, s.Status, body)
	}
	return s.ReadAll()
}

// deployedProject creates a Dockerfile project and runs one (failing, without
// Docker) deploy so that its "app" service row exists.
func deployedProject(t *testing.T, c *Client, prefix string, fields ...string) (project map[string]any, service map[string]any) {
	t.Helper()
	project = createProject(t, c, uniq(prefix), dockerfileApp(), fields...)
	id := project["id"].(string)
	deployAndWait(t, c, id)
	svcs := c.Get(t, "/api/deployments/"+id+"/services").Arr(t)
	if len(svcs) != 1 {
		t.Fatalf("want 1 service after deploy, got %d", len(svcs))
	}
	return project, svcs[0].(map[string]any)
}

// sseEvents parses `data: {...}` frames from an SSE body.
func sseEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, frame := range strings.Split(body, "\n\n") {
		frame = strings.TrimSpace(frame)
		if frame == "" {
			continue
		}
		if !strings.HasPrefix(frame, "data: ") {
			t.Fatalf("unexpected SSE frame %q", frame)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &m); err != nil {
			t.Fatalf("SSE frame JSON %q: %v", frame, err)
		}
		out = append(out, m)
	}
	return out
}
