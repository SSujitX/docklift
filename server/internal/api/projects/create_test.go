package projects

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"docklift/internal/gitops"
	"docklift/internal/store"
)

func TestCreateValidation(t *testing.T) {
	f := newFixture(t, Options{})
	zipData := appZip(t)
	cases := []struct {
		name string
		body any
		code int
		msg  string
	}{
		{"empty body", map[string]any{}, 400, msgNameRequired},
		{"no body", nil, 400, msgNameRequired},
		{"blank name", map[string]any{"name": "   ", "github_url": "https://github.com/a/b"}, 400, msgNameRequired},
		{"non-string name", map[string]any{"name": 7, "github_url": "https://github.com/a/b"}, 400, msgNameRequired},
		{"name too long", map[string]any{"name": strings.Repeat("n", 121), "github_url": "https://github.com/a/b"}, 400, msgNameRequired},
		{"no source", map[string]any{"name": "x"}, 400, msgSourceRequired},
		{"unknown source", map[string]any{"name": "x", "source_type": "ftp"}, 400, msgSourceRequired},
		{"blank github url", map[string]any{"name": "x", "github_url": "   "}, 400, msgSourceRequired},
		{"upload without zip", map[string]any{"name": "x", "source_type": "upload"}, 400, msgZipRequired},
		{"github without url", map[string]any{"name": "x", "source_type": "github"}, 400, msgGithubURL},
		{"github url not a string", map[string]any{"name": "x", "source_type": "github", "github_url": 5}, 400, msgGithubURL},
		{"non-github host", map[string]any{"name": "x", "github_url": "https://gitlab.com/a/b"}, 400, msgGithubURL},
		{"credentials in url", map[string]any{"name": "x", "github_url": "https://x-access-token:t@github.com/a/b"}, 400, msgGithubURL},
		{"bad domain", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "domain": "bad domain"}, 400, msgDomainFormat},
		{"domain not a string", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "domain": 5}, 400, msgDomainFormat},
		{"bad branch", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "github_branch": "--upload-pack=x"}, 400, msgInvalidRef},
		{"branch not a string", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "github_branch": 5}, 400, "github_branch must be a string"},
		{"description not a string", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "description": true}, 400, "description must be a string"},
		{"project_type not a string", map[string]any{"name": "x", "github_url": "https://github.com/a/b", "project_type": []any{"a"}}, 400, "project_type must be a string"},
		{"zip with bad domain", upload("x", zipData, "domain", "ok.example.test, -bad.example.test"), 400, msgDomainFormat},
		{"zip under wrong field", &form{fields: [][2]string{{"name", "x"}}, files: []filePart{{"zip", "a.zip", zipData}}}, 400, msgUploadFailed},
		{"two zip files", &form{fields: [][2]string{{"name", "x"}}, files: []filePart{{"files", "a.zip", zipData}, {"files", "b.zip", zipData}}}, 400, msgUploadFailed},
		{"multipart without name", &form{files: []filePart{{"files", "a.zip", zipData}}}, 400, msgNameRequired},
		{"repeated name field", &form{fields: [][2]string{{"name", "a"}, {"name", "b"}}, files: []filePart{{"files", "a.zip", zipData}}}, 400, msgNameRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.do("POST", "/", tc.body).expectError(t, tc.code, tc.msg)
		})
	}
	if left := f.uploadsLeft(); len(left) != 0 {
		t.Fatalf("temporary uploads left behind: %v", left)
	}
	if list := f.do("GET", "/", nil).arr(t); len(list) != 0 {
		t.Fatalf("no project may be created: %v", list)
	}
}

func TestCreateMalformedMultipart(t *testing.T) {
	f := newFixture(t, Options{})
	for _, ct := range []string{"multipart/form-data", "multipart/form-data; boundary=xyz"} {
		req := httptest.NewRequest("POST", "/", strings.NewReader("--xyz\r\nContent-Disposition: form-data; name=\"files\"; filename=\"a.zip\"\r\n\r\nunterminated"))
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		resp{code: rec.Code, raw: rec.Body.Bytes()}.expectError(t, 400, msgUploadFailed)
	}
	if left := f.uploadsLeft(); len(left) != 0 {
		t.Fatalf("temporary uploads left behind: %v", left)
	}
}

func TestCreateUploadTooLarge(t *testing.T) {
	f := newFixture(t, Options{})
	prev := maxUploadBytes
	maxUploadBytes = 64
	t.Cleanup(func() { maxUploadBytes = prev })
	f.do("POST", "/", upload("big", bytes.Repeat([]byte("z"), 65))).expectError(t, 400, "Upload too large (max 64 bytes)")
	if left := f.uploadsLeft(); len(left) != 0 {
		t.Fatalf("temporary uploads left behind: %v", left)
	}
}

func TestCreateUpload(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.do("POST", "/", upload("  fields  ", appZip(t), "description", "d", "domain", "App.Example.dev, www.example.dev",
		"build_type", "dockerfile", "base_directory", "  .  ", "dockerfile_path", " Dockerfile ", "internal_port", "8080",
		"project_type", "worker", "github_branch", "kept-for-upload", "github_url", "https://github.com/a/b", "source_type", "upload")).expect(t, 201)
	want := map[string]any{
		"name": "fields", "description": "d", "source_type": "upload", "project_type": "worker", "github_url": nil,
		"github_branch": "kept-for-upload", "domain": "App.Example.dev, www.example.dev", "status": "pending", "auto_deploy": false,
		"webhook_secret": nil, "build_type": "dockerfile", "base_directory": ".", "dockerfile_path": "Dockerfile",
		"internal_port": float64(8080), "publish_host_port": false, "db_engine": nil, "port": nil, "container_name": nil,
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("%s = %#v, want %#v", k, p[k], v)
		}
	}
	dir := filepath.Join(f.d.Config.DeploymentsPath, p["id"].(string))
	if b, err := os.ReadFile(filepath.Join(dir, "src", "app.js")); err != nil || string(b) != "x" {
		t.Fatalf("extracted file: %q %v", b, err)
	}
	if left := f.uploadsLeft(); len(left) != 0 {
		t.Fatalf("temporary uploads left behind: %v", left)
	}
	// Defaults and normalisation.
	p = f.do("POST", "/", upload("defaults", appZip(t), "build_type", "nonsense", "base_directory", "", "internal_port", "abc")).expect(t, 201)
	if p["build_type"] != "auto" || p["base_directory"] != "." || p["internal_port"] != float64(3000) || p["project_type"] != "app" {
		t.Fatalf("defaults: %v", p)
	}
}

func TestCreateUnsafeZipRollsBack(t *testing.T) {
	f := newFixture(t, Options{})
	cases := map[string][]byte{
		"not a zip":     []byte("nope"),
		"traversal":     zipOf(t, "ok.txt", "ok", "../escape.txt", "x"),
		"absolute":      zipOf(t, "/abs.txt", "x"),
		"backslash dot": zipOf(t, `..\escape.txt`, "x"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			f.do("POST", "/", upload(name, data)).expectError(t, 400, msgUnsafeZip)
		})
	}
	if list := f.do("GET", "/", nil).arr(t); len(list) != 0 {
		t.Fatalf("projects must be rolled back: %v", list)
	}
	entries, _ := os.ReadDir(f.d.Config.DeploymentsPath)
	if len(entries) != 0 {
		t.Fatalf("project dirs must be removed: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.d.Config.DeploymentsPath), "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("zip-slip wrote outside the deployments dir")
	}
	if left := f.uploadsLeft(); len(left) != 0 {
		t.Fatalf("temporary uploads left behind: %v", left)
	}
}

func TestCreateGithubCloneFailureRollsBack(t *testing.T) {
	f := newFixture(t, Options{})
	f.d.Git.Bin = filepath.Join(t.TempDir(), "no-such-git")
	// A file uploaded alongside a GitHub source is never used and is removed.
	body := upload("gh", appZip(t), "source_type", "github", "github_url", "https://github.com/a/b", "github_branch", "main")
	f.do("POST", "/", body).expectError(t, 500, "Failed to clone repository")
	if list := f.do("GET", "/", nil).arr(t); len(list) != 0 {
		t.Fatalf("project must be rolled back: %v", list)
	}
	if left := f.uploadsLeft(); len(left) != 0 {
		t.Fatalf("temporary uploads left behind: %v", left)
	}
}

type fakeGitHub struct {
	idErr, tokenErr error
	id, token       string
	calls           []string
}

func (g *fakeGitHub) InstallationIDForRepo(_ context.Context, owner, repo string) (string, error) {
	g.calls = append(g.calls, "id "+owner+"/"+repo)
	return g.id, g.idErr
}

func (g *fakeGitHub) InstallationToken(_ context.Context, id string) (string, error) {
	g.calls = append(g.calls, "token "+id)
	return g.token, g.tokenErr
}

func TestCloneCredentials(t *testing.T) {
	ctx := context.Background()
	t.Run("no app", func(t *testing.T) {
		f := newFixture(t, Options{})
		h := &handlers{d: f.d}
		if c := h.cloneCredentials(ctx, "https://github.com/o/r.git"); c != nil {
			t.Fatal("anonymous clone expected")
		}
	})
	t.Run("repo installation", func(t *testing.T) {
		f := newFixture(t, Options{})
		g := &fakeGitHub{id: "42", token: "tok"}
		h := &handlers{d: f.d, github: g}
		c := h.cloneCredentials(ctx, "https://github.com/o/r.git")
		if c == nil || *c != *gitops.InstallationCredentials("tok") {
			t.Fatalf("creds %v", c)
		}
		if strings.Join(g.calls, ",") != "id o/r,token 42" {
			t.Fatalf("calls %v", g.calls)
		}
	})
	t.Run("lookup fails: saved id", func(t *testing.T) {
		f := newFixture(t, Options{})
		if err := f.d.Store.SetSetting(ctx, "github_installation_id", "7"); err != nil {
			t.Fatal(err)
		}
		g := &fakeGitHub{idErr: errors.New("404"), token: "tok"}
		h := &handlers{d: f.d, github: g}
		if c := h.cloneCredentials(ctx, "https://github.com/o/r"); c == nil {
			t.Fatal("want creds from saved id")
		}
		if g.calls[1] != "token 7" {
			t.Fatalf("calls %v", g.calls)
		}
	})
	t.Run("lookup fails without saved id", func(t *testing.T) {
		f := newFixture(t, Options{})
		g := &fakeGitHub{idErr: errors.New("404")}
		h := &handlers{d: f.d, github: g}
		if c := h.cloneCredentials(ctx, "https://github.com/o/r"); c != nil || len(g.calls) != 1 {
			t.Fatalf("creds %v calls %v", c, g.calls)
		}
	})
	t.Run("token fails", func(t *testing.T) {
		f := newFixture(t, Options{})
		h := &handlers{d: f.d, github: &fakeGitHub{id: "1", tokenErr: errors.New("401")}}
		if c := h.cloneCredentials(ctx, "https://github.com/o/r"); c != nil {
			t.Fatal("public clone expected")
		}
	})
}

func TestRollbackIgnoresMissing(t *testing.T) {
	f := newFixture(t, Options{})
	h := &handlers{d: f.d}
	p := f.project(store.ProjectCreate{})
	dir := f.d.Deploy.ProjectPath(p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h.rollback(p.ID)
	h.rollback(p.ID)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("dir kept")
	}
	if _, err := f.d.Store.GetProject(context.Background(), p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("row kept")
	}
}
