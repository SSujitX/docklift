package contract

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// relPath is how the server spells a project-relative path in the file tree.
// TS uses path.relative, so a Windows-hosted server answers with backslashes;
// Go answers with forward slashes on every OS (deliberate fix, the dashboard
// splits paths on "/").
func relPath(p string) any {
	if targetIsGo() {
		return p
	}
	if mainInst.Booted() {
		return filepath.FromSlash(p)
	}
	return Either(p, filepath.FromSlash(p))
}

func contentURL(id, p string) string {
	return "/api/files/" + id + "/content?path=" + url.QueryEscape(p)
}

func TestFilesTree(t *testing.T) {
	c := admin.Client()
	p := createProject(t, c, uniq("files"), dockerfileApp())
	id := p["id"].(string)
	file := func(name, path string, size int, editable bool) Obj {
		return Obj{"name": name, "path": relPath(path), "type": "file", "size": size, "editable": editable}
	}
	c.Get(t, "/api/files/"+id).Expect(t, 200, Arr{
		Obj{"name": "src", "path": "src", "type": "folder", "children": Arr{
			Obj{"name": "lib", "path": relPath("src/lib"), "type": "folder", "children": Arr{
				file("util.js", "src/lib/util.js", 20, true),
			}},
			file("app.js", "src/app.js", 24, true),
		}},
		file("Dockerfile", "Dockerfile", 51, true),
		file("index.html", "index.html", 18, true),
		file("logo.png", "logo.png", 9, false),
		file("package.json", "package.json", 20, true),
	})
	c.Get(t, "/api/files/"+missingProjectID).ExpectError(t, 404, "Project files not found")
	for _, bad := range []string{"a.b", "..", "%2e%2e", "x%20y", "a%2Fb"} {
		c.Get(t, "/api/files/"+bad).ExpectError(t, 400, "Invalid project ID")
		c.Get(t, "/api/files/"+bad+"/content?path=x").ExpectError(t, 400, "Invalid project ID")
		c.Put(t, "/api/files/"+bad+"/content?path=x", map[string]any{"content": "x"}).ExpectError(t, 400, "Invalid project ID")
	}
	mainInst.Anon().Get(t, "/api/files/"+id).ExpectError(t, 401, "Authentication required")
}

func TestFilesReadWrite(t *testing.T) {
	c := admin.Client()
	p := createProject(t, c, uniq("files-rw"), dockerfileApp())
	id := p["id"].(string)

	c.Get(t, contentURL(id, "src/app.js")).Expect(t, 200, Obj{"name": "app.js", "content": "console.log('contract')\n"})
	c.Get(t, contentURL(id, "./src/../index.html")).Expect(t, 200, Obj{"name": "index.html", "content": "<h1>contract</h1>\n"})
	// Binary-looking files are still served as text by the reader.
	c.Get(t, contentURL(id, "logo.png")).Expect(t, 200, Obj{"name": "logo.png", "content": Str})
	c.Get(t, contentURL(id, "src")).ExpectError(t, 400, "Path is a directory")
	c.Get(t, "/api/files/"+id+"/content").ExpectError(t, 400, "Path is a directory")
	c.Get(t, contentURL(id, "nope.txt")).ExpectError(t, 404, "File not found")
	c.Get(t, contentURL(missingProjectID, "x")).ExpectError(t, 404, "File not found")

	c.Put(t, contentURL(id, "src/app.js"), map[string]any{"content": "updated\n"}).Expect(t, 200, Obj{"status": "updated"})
	c.Get(t, contentURL(id, "src/app.js")).Expect(t, 200, Obj{"name": "app.js", "content": "updated\n"})
	c.Put(t, contentURL(id, "src/app.js"), map[string]any{"content": ""}).Expect(t, 200, Obj{"status": "updated"})
	c.Get(t, contentURL(id, "src/app.js")).Expect(t, 200, Obj{"name": "app.js", "content": ""})
	// The editor never creates files.
	c.Put(t, contentURL(id, "new.txt"), map[string]any{"content": "x"}).ExpectError(t, 404, "File not found")
	c.Put(t, contentURL(id, "src"), map[string]any{"content": "x"}).ExpectError(t, 500, "Failed to update file")
	// A missing or non-string content field: TS reaches fs.writeFileSync(undefined)
	// and answers 500; Go rejects it with a 400 (deliberate fix).
	for _, body := range []map[string]any{{}, {"content": 7}, {"content": nil}} {
		if targetIsGo() {
			c.Put(t, contentURL(id, "index.html"), body).ExpectError(t, 400, "File content must be a string")
		} else {
			c.Put(t, contentURL(id, "index.html"), body).ExpectError(t, 500, "Failed to update file")
		}
	}
	c.Get(t, contentURL(id, "index.html")).Expect(t, 200, Obj{"name": "index.html", "content": "<h1>contract</h1>\n"})
}

func TestFilesPathSecurity(t *testing.T) {
	c := admin.Client()
	p := createProject(t, c, uniq("files-sec"), dockerfileApp())
	other := createProject(t, c, uniq("files-other"), dockerfileApp())
	id := p["id"].(string)

	escapes := []string{"../" + other["id"].(string) + "/index.html", "../../data/smoke.db", "src/../../x", "/etc/passwd", "..", "../"}
	if runtime.GOOS == "windows" && mainInst.Booted() {
		escapes = append(escapes, `C:\Windows\win.ini`, `..\x`)
	}
	for _, e := range escapes {
		c.Get(t, contentURL(id, e)).ExpectError(t, 403, "Access denied")
		c.Put(t, contentURL(id, e), map[string]any{"content": "pwned"}).ExpectError(t, 403, "Access denied")
	}
	// Sibling-prefix paths are outside too (deployments/<id>-evil).
	c.Get(t, contentURL(id, "../"+id+"-evil/x")).ExpectError(t, 403, "Access denied")

	// .git is blocked whether or not it exists, in any letter case.
	for _, g := range []string{".git/config", ".git", "sub/.git/hooks/pre-commit", ".GIT/config", "src/.Git/x", "./.git/../.git/HEAD"} {
		c.Get(t, contentURL(id, g)).ExpectError(t, 403, "Access denied - git metadata is not accessible")
		c.Put(t, contentURL(id, g), map[string]any{"content": "x"}).ExpectError(t, 403, "Access denied - git metadata is not editable")
	}
	// Names that merely contain ".git" are fine.
	c.Get(t, contentURL(id, ".gitignore")).ExpectError(t, 404, "File not found")
	c.Get(t, contentURL(id, "my.git")).ExpectError(t, 404, "File not found")
}

func TestFilesSymlinks(t *testing.T) {
	requireBoot(t)
	c := admin.Client()
	p := createProject(t, c, uniq("files-link"), dockerfileApp())
	id := p["id"].(string)
	projDir := filepath.Join(mainInst.Dirs.Deployments, id)
	outside := filepath.Join(mainInst.Root, "outside-secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(projDir, "escape.txt")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	if err := os.Symlink(filepath.Join(projDir, "index.html"), filepath.Join(projDir, "inside-link.txt")); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(projDir, ".git")
	if err := os.Symlink(filepath.Join(gitDir, "config"), filepath.Join(projDir, "gitcfg")); err != nil {
		t.Fatal(err)
	}
	c.Get(t, contentURL(id, "escape.txt")).ExpectError(t, 403, "Access denied - symlink outside project")
	c.Put(t, contentURL(id, "escape.txt"), map[string]any{"content": "pwned"}).ExpectError(t, 403, "Access denied - symlink outside project")
	if string(readFileBytes(t, outside)) != "secret" {
		t.Fatal("write followed a symlink out of the project")
	}
	c.Get(t, contentURL(id, "gitcfg")).ExpectError(t, 403, "Access denied - git metadata is not accessible")
	c.Put(t, contentURL(id, "gitcfg"), map[string]any{"content": "x"}).ExpectError(t, 403, "Access denied - git metadata is not editable")
	// A link that stays inside the project is readable.
	c.Get(t, contentURL(id, "inside-link.txt")).Expect(t, 200, Obj{"name": "inside-link.txt", "content": "<h1>contract</h1>\n"})
	// Symlinks are never listed in the tree.
	for _, e := range c.Get(t, "/api/files/"+id).Arr(t) {
		switch e.(map[string]any)["name"] {
		case "escape.txt", "inside-link.txt", "gitcfg":
			t.Fatalf("tree lists symlink %v", e)
		}
	}
}

// TestFilesDirLinkEscape uses a directory link (an NTFS junction on Windows,
// which needs no privilege) pointing outside the project.
func TestFilesDirLinkEscape(t *testing.T) {
	requireBoot(t)
	c := admin.Client()
	p := createProject(t, c, uniq("files-junction"), dockerfileApp())
	id := p["id"].(string)
	outsideDir := filepath.Join(mainInst.Root, "outside-"+randHex(3))
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linkDir(outsideDir, filepath.Join(mainInst.Dirs.Deployments, id, "linked")); err != nil {
		t.Skipf("cannot create a directory link: %v", err)
	}
	c.Get(t, contentURL(id, "linked/secret.txt")).ExpectError(t, 403, "Access denied - symlink outside project")
	c.Put(t, contentURL(id, "linked/secret.txt"), map[string]any{"content": "pwned"}).ExpectError(t, 403, "Access denied - symlink outside project")
	if string(readFileBytes(t, filepath.Join(outsideDir, "secret.txt"))) != "secret" {
		t.Fatal("write followed a directory link out of the project")
	}
	for _, e := range c.Get(t, "/api/files/"+id).Arr(t) {
		if e.(map[string]any)["name"] == "linked" {
			t.Fatalf("tree lists the directory link: %v", e)
		}
	}
}
