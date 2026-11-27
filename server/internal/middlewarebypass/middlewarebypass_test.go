package middlewarebypass

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPatchAddsHostsAndKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	mw := filepath.Join(dir, "src", "middleware.ts")
	src := "import x from 'y';\nconst allowedHosts = [\"example.com\", 'www.example.com'];\nexport default x;\n"
	write(t, mw, src)

	r := Patch(Options{ProjectPath: dir, Port: 5501, Domains: []string{" app.example.com ", "", "example.com"}, ServerIP: "10.0.0.5"})
	if !r.Found || !r.Patched || r.FilePath != mw || r.Error != "" {
		t.Fatalf("result = %+v", r)
	}
	if want := []string{"example.com", "www.example.com"}; !reflect.DeepEqual(r.OriginalHosts, want) {
		t.Errorf("original = %v", r.OriginalHosts)
	}
	if want := []string{"localhost", "localhost:5501", "10.0.0.5", "10.0.0.5:5501", "app.example.com"}; !reflect.DeepEqual(r.AddedHosts, want) {
		t.Errorf("added = %v", r.AddedHosts)
	}
	got, _ := os.ReadFile(mw)
	want := "import x from 'y';\nconst allowedHosts = [\"example.com\", \"www.example.com\", \"localhost\", \"localhost:5501\", \"10.0.0.5\", \"10.0.0.5:5501\", \"app.example.com\"];\nexport default x;\n"
	if string(got) != want {
		t.Errorf("patched file:\n%s", got)
	}
	if bak, _ := os.ReadFile(mw + ".bak"); string(bak) != src {
		t.Error("backup must hold the original content")
	}

	again := Patch(Options{ProjectPath: dir, Port: 5501, Domains: []string{"app.example.com"}, ServerIP: "10.0.0.5"})
	if !again.Found || again.Patched || len(again.AddedHosts) != 0 {
		t.Errorf("second run must be a no-op: %+v", again)
	}
}

func TestPatchSearchRules(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "node_modules", "pkg", "middleware.js"), "const allowedHosts = []")
	write(t, filepath.Join(dir, "a", "b", "c", "d", "middleware.js"), "const allowedHosts = []")
	write(t, filepath.Join(dir, "middleware.js"), "export const config = {}")
	if r := Patch(Options{ProjectPath: dir, Port: 1, ServerIP: "1.2.3.4"}); r.Found || r.Patched {
		t.Errorf("skipped dirs, depth limit and files without allowedHosts must be ignored: %+v", r)
	}
	write(t, filepath.Join(dir, "a", "b", "c", "middleware.js"), "const allowedHosts=[]")
	r := Patch(Options{ProjectPath: dir, Port: 1, ServerIP: "1.2.3.4"})
	if !r.Patched || !strings.HasSuffix(r.FilePath, filepath.Join("a", "b", "c", "middleware.js")) {
		t.Errorf("depth-3 file must be patched: %+v", r)
	}
	if r := Patch(Options{ProjectPath: filepath.Join(dir, "missing")}); r.Found {
		t.Error("missing project directory finds nothing")
	}
}

func TestLog(t *testing.T) {
	var out strings.Builder
	w := func(s string) { out.WriteString(s) }
	Log(Result{}, w)
	if out.Len() != 0 {
		t.Error("nothing is logged when no middleware was found")
	}
	Log(Result{Found: true, Patched: true, FilePath: "/p/middleware.ts", OriginalHosts: []string{"a"}, AddedHosts: []string{"b", "c"}}, w)
	for _, want := range []string{"Middleware Host Check Detected", "File: /p/middleware.ts", "Original hosts: a", "Added hosts: b, c", "Backup created: /p/middleware.ts.bak"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	Log(Result{Found: true, FilePath: "f", Error: "denied"}, w)
	if !strings.Contains(out.String(), "Error patching: denied") || strings.Contains(out.String(), "Backup") {
		t.Errorf("error log:\n%s", out.String())
	}
	out.Reset()
	Log(Result{Found: true, FilePath: "f"}, w)
	if !strings.Contains(out.String(), "Already has required hosts") {
		t.Errorf("unchanged log:\n%s", out.String())
	}
	if ip := ServerIP(); ip == "" {
		t.Error("ServerIP must return an address or localhost")
	}
}
