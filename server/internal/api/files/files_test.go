package files

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/httpx"
)

const projectID = "11111111-2222-4333-8444-555555555555"

type fixture struct {
	d    *app.Deps
	h    http.Handler
	proj string
	root string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := apptest.NewDeps(t)
	r := chi.NewRouter()
	r.Use(httpx.ParseBody)
	Mount(r, d)
	proj := filepath.Join(d.Config.DeploymentsPath, projectID)
	f := &fixture{d: d, h: r, proj: proj, root: filepath.Dir(d.Config.DeploymentsPath)}
	for p, body := range map[string]string{
		"Dockerfile":           "FROM nginx\n",
		"index.html":           "<h1>hi</h1>\n",
		"logo.png":             "\x89PNG\x00\xff",
		"src/app.js":           "console.log(1)\n",
		"src/lib/util.js":      "export {}\n",
		".git/config":          "[core]\n",
		"node_modules/x.js":    "x",
		"dist":                 "a file named dist is skipped too",
		"README.md":            "r",
		"readme.txt":           "r",
		"_private/x":           "x",
		"Makefile":             "all:",
		".env":                 "A=1",
		"assets/.png":          "hidden png",
		"assets/IMAGE.JPEG":    "j",
		"packages/a/.git/HEAD": "ref",
	} {
		write(t, filepath.Join(proj, filepath.FromSlash(p)), body)
	}
	return f
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f *fixture) do(t *testing.T, method, target string, body string) (int, any) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	var out any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: %d %q", method, target, rec.Code, rec.Body.String())
	}
	return rec.Code, out
}

func contentURL(id, p string) string {
	return "/" + id + "/content?path=" + url.QueryEscape(p)
}

func expectErr(t *testing.T, code int, body any, wantCode int, wantMsg string) {
	t.Helper()
	m, _ := body.(map[string]any)
	if code != wantCode || m["error"] != wantMsg {
		t.Errorf("got %d %v, want %d %q", code, body, wantCode, wantMsg)
	}
}

func linkDir(target, link string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
	}
	return os.Symlink(target, link)
}

func TestTree(t *testing.T) {
	f := newFixture(t)
	code, body := f.do(t, "GET", "/"+projectID, "")
	if code != 200 {
		t.Fatalf("code %d %v", code, body)
	}
	got, _ := json.Marshal(body)
	arr := body.([]any)
	var names []string
	for _, e := range arr {
		names = append(names, e.(map[string]any)["name"].(string))
	}
	wantNames := []string{"_private", "assets", "packages", "src", ".env", "Dockerfile", "index.html", "logo.png", "Makefile", "README.md", "readme.txt"}
	if !slices.Equal(names, wantNames) {
		t.Errorf("names = %v\nwant    %v (%s)", names, wantNames, got)
	}
	find := func(list []any, name string) map[string]any {
		for _, e := range list {
			if m := e.(map[string]any); m["name"] == name {
				return m
			}
		}
		t.Fatalf("%s not listed", name)
		return nil
	}
	src := find(arr, "src")
	if src["type"] != "folder" || src["path"] != "src" || len(src) != 4 {
		t.Errorf("src = %v", src)
	}
	lib := find(src["children"].([]any), "lib")
	util := find(lib["children"].([]any), "util.js")
	if lib["path"] != "src/lib" || util["path"] != "src/lib/util.js" || util["size"] != float64(10) || util["editable"] != true || len(util) != 5 {
		t.Errorf("forward-slash paths: lib %v util %v", lib, util)
	}
	if find(arr, "logo.png")["editable"] != false || find(arr, "Makefile")["editable"] != true {
		t.Error("editable flags")
	}
	assets := find(arr, "assets")["children"].([]any)
	if find(assets, ".png")["editable"] != false || find(assets, "IMAGE.JPEG")["editable"] != false {
		t.Error("extension check is case-insensitive and uses the last dot")
	}
	pkgA := find(find(arr, "packages")["children"].([]any), "a")
	if len(pkgA["children"].([]any)) != 0 {
		t.Errorf("nested .git listed: %v", pkgA)
	}

	code, body = f.do(t, "GET", "/22222222-0000-4000-8000-000000000000", "")
	expectErr(t, code, body, 404, "Project files not found")
	for _, bad := range []string{"a.b", "x%20y", "a%2Fb", "%2e%2e", "%41%2Fb"} {
		code, body = f.do(t, "GET", "/"+bad, "")
		expectErr(t, code, body, 400, "Invalid project ID")
	}
	// Params are decoded once, like Express.
	if code, _ := f.do(t, "GET", "/%41bc", ""); code != 404 {
		t.Errorf("%%41bc = %d, want 404 (decoded to Abc)", code)
	}
}

func TestTreeDepthLimit(t *testing.T) {
	f := newFixture(t)
	deep := f.proj
	for i := 0; i < 13; i++ {
		deep = filepath.Join(deep, "d")
	}
	write(t, filepath.Join(deep, "leaf.txt"), "x")
	_, body := f.do(t, "GET", "/"+projectID, "")
	level := 0
	cur := body.([]any)
	for {
		var next []any
		for _, e := range cur {
			if m := e.(map[string]any); m["name"] == "d" {
				next = m["children"].([]any)
			}
		}
		if next == nil {
			break
		}
		level++
		cur = next
	}
	// Folders at depth 0..10 are read; the folder at depth 11 has no children.
	if level != 11 || len(cur) != 0 {
		t.Errorf("depth = %d, last children %v", level, cur)
	}
}

func TestTreeSkipsLinks(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(f.root, "outside")
	write(t, filepath.Join(outside, "secret.txt"), "s")
	if err := linkDir(outside, filepath.Join(f.proj, "linked")); err != nil {
		t.Skipf("cannot create a directory link: %v", err)
	}
	_ = os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(f.proj, "file-link"))
	_, body := f.do(t, "GET", "/"+projectID, "")
	for _, e := range body.([]any) {
		if n := e.(map[string]any)["name"]; n == "linked" || n == "file-link" {
			t.Errorf("link listed: %v", e)
		}
	}
}

func TestReadWrite(t *testing.T) {
	f := newFixture(t)
	get := func(p string) (int, any) { return f.do(t, "GET", contentURL(projectID, p), "") }
	put := func(p, body string) (int, any) { return f.do(t, "PUT", contentURL(projectID, p), body) }

	code, body := get("src/app.js")
	if m := body.(map[string]any); code != 200 || m["name"] != "app.js" || m["content"] != "console.log(1)\n" {
		t.Errorf("read = %d %v", code, body)
	}
	if _, body := get("./src/../index.html"); body.(map[string]any)["name"] != "index.html" {
		t.Errorf("dot segments = %v", body)
	}
	if _, body := get("logo.png"); body.(map[string]any)["content"] != "\uFFFDPNG\x00\uFFFD" {
		t.Errorf("binary read = %q", body.(map[string]any)["content"])
	}
	code, body = get("src")
	expectErr(t, code, body, 400, "Path is a directory")
	code, body = f.do(t, "GET", "/"+projectID+"/content", "")
	expectErr(t, code, body, 400, "Path is a directory")
	code, body = get("nope.txt")
	expectErr(t, code, body, 404, "File not found")
	code, body = get("a\x00b")
	expectErr(t, code, body, 404, "File not found")
	code, body = f.do(t, "GET", "/22222222-0000-4000-8000-000000000000/content?path=x", "")
	expectErr(t, code, body, 404, "File not found")
	code, body = f.do(t, "GET", "/"+projectID+"/content?path=a&path=b", "")
	expectErr(t, code, body, 500, "Failed to read file")
	code, body = f.do(t, "GET", "/a.b/content?path=x", "")
	expectErr(t, code, body, 400, "Invalid project ID")

	code, body = put("src/app.js", `{"content":"updated\n","filename":"src/app.js"}`)
	if code != 200 || body.(map[string]any)["status"] != "updated" || read(t, filepath.Join(f.proj, "src", "app.js")) != "updated\n" {
		t.Errorf("write = %d %v", code, body)
	}
	if code, _ := put("src/app.js", `{"content":""}`); code != 200 || read(t, filepath.Join(f.proj, "src", "app.js")) != "" {
		t.Errorf("empty write = %d", code)
	}
	if code, _ := put("src/app.js", `{"content":"ünïcödé \ud83d\ude00"}`); code != 200 || read(t, filepath.Join(f.proj, "src", "app.js")) != "ünïcödé 😀" {
		t.Error("UTF-8 write")
	}
	code, body = put("new.txt", `{"content":"x"}`)
	expectErr(t, code, body, 404, "File not found")
	if _, err := os.Stat(filepath.Join(f.proj, "new.txt")); !os.IsNotExist(err) {
		t.Error("the editor created a file")
	}
	code, body = put("src", `{"content":"x"}`)
	expectErr(t, code, body, 500, "Failed to update file")
	for _, bad := range []string{`{}`, `{"content":7}`, `{"content":null}`, `{"content":["x"]}`, `{"content":{"a":1}}`, ``} {
		code, body = put("index.html", bad)
		expectErr(t, code, body, 400, "File content must be a string")
	}
	if read(t, filepath.Join(f.proj, "index.html")) != "<h1>hi</h1>\n" {
		t.Error("rejected writes changed the file")
	}
}

func TestReadTooLarge(t *testing.T) {
	f := newFixture(t)
	big := filepath.Join(f.proj, "big.log")
	fh, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := fh.Truncate(MaxEditableBytes + 1); err != nil {
		t.Fatal(err)
	}
	fh.Close()
	code, body := f.do(t, "GET", contentURL(projectID, "big.log"), "")
	expectErr(t, code, body, 413, "File is too large to open in the editor")
}

func TestPathSecurity(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(f.d.Config.DeploymentsPath, "other")
	write(t, filepath.Join(other, "index.html"), "other")
	write(t, filepath.Join(f.proj+"-evil", "x"), "evil")
	escapes := []string{"../other/index.html", "../../data/docklift.db", "src/../../x", "/etc/passwd", "..", "../", "../" + projectID + "-evil/x"}
	if runtime.GOOS == "windows" {
		escapes = append(escapes, `C:\Windows\win.ini`, `..\other\index.html`, `\Windows\win.ini`, `C:win.ini`, `\\localhost\c$\Windows\win.ini`, `src\..\..\other\index.html`)
	}
	for _, e := range escapes {
		code, body := f.do(t, "GET", contentURL(projectID, e), "")
		expectErr(t, code, body, 403, "Access denied")
		code, body = f.do(t, "PUT", contentURL(projectID, e), `{"content":"pwned"}`)
		expectErr(t, code, body, 403, "Access denied")
	}
	if read(t, filepath.Join(other, "index.html")) != "other" {
		t.Fatal("escape write succeeded")
	}
	gits := []string{".git/config", ".git", "sub/.git/hooks/pre-commit", ".GIT/config", "src/.Git/x", "./.git/../.git/HEAD", "packages/a/.git/HEAD"}
	if runtime.GOOS == "windows" {
		gits = append(gits, `.git\config`, ".git./config", ".git /config", "GIT~1/config", ".git::$INDEX_ALLOCATION/config")
	}
	for _, g := range gits {
		code, body := f.do(t, "GET", contentURL(projectID, g), "")
		expectErr(t, code, body, 403, "Access denied - git metadata is not accessible")
		code, body = f.do(t, "PUT", contentURL(projectID, g), `{"content":"x"}`)
		expectErr(t, code, body, 403, "Access denied - git metadata is not editable")
	}
	if read(t, filepath.Join(f.proj, ".git", "config")) != "[core]\n" {
		t.Fatal(".git/config modified")
	}
	for _, ok := range []string{".gitignore", "my.git"} {
		code, body := f.do(t, "GET", contentURL(projectID, ok), "")
		expectErr(t, code, body, 404, "File not found")
	}
}

func TestSymlinks(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(f.root, "outside-secret.txt")
	write(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(f.proj, "escape.txt")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Symlink(filepath.Join(f.proj, "index.html"), filepath.Join(f.proj, "inside-link.txt")))
	must(os.Symlink(filepath.Join(f.proj, ".git", "config"), filepath.Join(f.proj, "gitcfg")))
	must(os.Symlink(filepath.Join(f.proj, "missing"), filepath.Join(f.proj, "dangling")))

	code, body := f.do(t, "GET", contentURL(projectID, "escape.txt"), "")
	expectErr(t, code, body, 403, "Access denied - symlink outside project")
	code, body = f.do(t, "PUT", contentURL(projectID, "escape.txt"), `{"content":"pwned"}`)
	expectErr(t, code, body, 403, "Access denied - symlink outside project")
	if read(t, outside) != "secret" {
		t.Fatal("write followed a symlink out of the project")
	}
	code, body = f.do(t, "GET", contentURL(projectID, "gitcfg"), "")
	expectErr(t, code, body, 403, "Access denied - git metadata is not accessible")
	code, body = f.do(t, "PUT", contentURL(projectID, "gitcfg"), `{"content":"x"}`)
	expectErr(t, code, body, 403, "Access denied - git metadata is not editable")
	code, body = f.do(t, "GET", contentURL(projectID, "dangling"), "")
	expectErr(t, code, body, 404, "File not found")

	code, body = f.do(t, "GET", contentURL(projectID, "inside-link.txt"), "")
	if m := body.(map[string]any); code != 200 || m["name"] != "inside-link.txt" || m["content"] != "<h1>hi</h1>\n" {
		t.Errorf("inside link read = %d %v", code, body)
	}
	// A write through an inside link lands in its target; the link stays a link.
	code, _ = f.do(t, "PUT", contentURL(projectID, "inside-link.txt"), `{"content":"via link"}`)
	if code != 200 || read(t, filepath.Join(f.proj, "index.html")) != "via link" {
		t.Errorf("inside link write = %d", code)
	}
	if fi, err := os.Lstat(filepath.Join(f.proj, "inside-link.txt")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("inside link replaced by a file")
	}
}

func TestDirLinkEscape(t *testing.T) {
	f := newFixture(t)
	outsideDir := filepath.Join(f.root, "outside-dir")
	write(t, filepath.Join(outsideDir, "secret.txt"), "secret")
	if err := linkDir(outsideDir, filepath.Join(f.proj, "linked")); err != nil {
		t.Skipf("cannot create a directory link: %v", err)
	}
	code, body := f.do(t, "GET", contentURL(projectID, "linked/secret.txt"), "")
	expectErr(t, code, body, 403, "Access denied - symlink outside project")
	code, body = f.do(t, "PUT", contentURL(projectID, "linked/secret.txt"), `{"content":"pwned"}`)
	expectErr(t, code, body, 403, "Access denied - symlink outside project")
	if read(t, filepath.Join(outsideDir, "secret.txt")) != "secret" {
		t.Fatal("write followed a directory link out of the project")
	}
	// A project directory that is itself a link is resolved too.
	linkedProject := filepath.Join(f.d.Config.DeploymentsPath, "linkedproj")
	if err := linkDir(f.proj, linkedProject); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.do(t, "GET", contentURL("linkedproj", "index.html"), ""); code != 200 {
		t.Errorf("linked project read = %d", code)
	}
}

func TestLocaleCompare(t *testing.T) {
	// Reference order produced by Node (ICU) with names.sort((a, b) => a.localeCompare(b)).
	want := []string{"_a", "-a", ".a", ".env", ".gitignore", "(x)", "[x]", "{x}", "@x", "#x", "+x", "=x", "~x", "$x", "1", "10", "2",
		"a", "A", "a b", "a,b", "a;b", "a.b", "a10", "a2", "ab", "ab", "aB", "Ab", "AB", "b", "B", "Dockerfile", "e", "é", "f",
		"file_a.txt", "file-a.txt", "file.txt", "index.html", "logo.png", "package.json", "readme.md", "README.md", "x!", "x?", "z", "Z"}
	got := slices.Clone(want)
	slices.Reverse(got)
	slices.SortStableFunc(got, localeCompare)
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
