package pathsec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var base = abs(filepath.Join("deployments", "project-1"))

func at(parts ...string) string { return filepath.Join(append([]string{base}, parts...)...) }

func TestTouchesGitDir(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"git dir", at(".git"), true},
		{"git config", at(".git", "config"), true},
		{"hook", at(".git", "hooks", "post-merge"), true},
		{"nested repo", at("packages", "api", ".git", "config"), true},
		{"upper case", at(".GIT", "config"), true},
		{"mixed case", at("src", ".Git", "x"), true},
		{"dot segments", at("src", "..", ".git", "config"), true},
		{"gitignore", at(".gitignore"), false},
		{"gitattributes", at(".gitattributes"), false},
		{"github workflows", at(".github", "workflows", "ci.yml"), false},
		{"git.ts", at("src", "git.ts"), false},
		{"dotted name", at("docs", "my.git.notes.md"), false},
		{"dockerfile", at("Dockerfile"), false},
		{"base itself", base, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TouchesGitDir(base, tc.path); got != tc.want {
				t.Errorf("TouchesGitDir(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestTouchesGitDirBackslashSegments(t *testing.T) {
	// A backslash separates segments on every OS, as in the TS split(/[\\/]+/).
	if !TouchesGitDir(base, base+string(filepath.Separator)+`sub\.git\config`) {
		t.Error("backslash-separated .git segment not detected")
	}
}

func TestIsGitSegmentWindowsSpellings(t *testing.T) {
	for _, seg := range []string{".git.", ".git ", ".git. .", ".git::$INDEX_ALLOCATION", "GIT~1", ".GIT"} {
		if !isGitSegment(seg, true) {
			t.Errorf("windows: %q not treated as .git", seg)
		}
	}
	for _, seg := range []string{".git.", ".git ", "GIT~1", ".gitignore", ".github"} {
		if isGitSegment(seg, false) {
			t.Errorf("unix: %q treated as .git", seg)
		}
	}
	for _, seg := range []string{".gitignore", ".github", "git", "x.git"} {
		if isGitSegment(seg, true) {
			t.Errorf("windows: %q treated as .git", seg)
		}
	}
}

func TestIsInside(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"base", base, true},
		{"child", at("src", "index.ts"), true},
		{"dot-dot named file", at("..env"), true},
		{"sibling project", at("..", "project-2", "x"), false},
		{"sibling prefix", base + "-sibling", false},
		{"parent", at(".."), false},
		{"root", string(filepath.Separator), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsInside(base, tc.path); got != tc.want {
				t.Errorf("IsInside(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
	if runtime.GOOS == "windows" {
		if !IsInside(`C:\Deploy\P1`, `c:\deploy\p1\x`) {
			t.Error("windows comparison must ignore case")
		}
		if IsInside(`C:\Deploy\P1`, `D:\Deploy\P1\x`) {
			t.Error("another volume is outside")
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		p    string
		want string
	}{
		{"", base},
		{"src/app.js", at("src", "app.js")},
		{"./src/../index.html", at("index.html")},
		{"../x", abs(filepath.Join(base, "..", "x"))},
	}
	for _, tc := range tests {
		if got := Resolve(base, tc.p); got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.p, got, tc.want)
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{`C:\Windows\win.ini`, `\Windows\win.ini`, `/Windows/win.ini`, `C:win.ini`, `\\server\share\x`} {
			if IsInside(base, Resolve(base, p)) {
				t.Errorf("Resolve(%q) = %q stays inside", p, Resolve(base, p))
			}
		}
	} else {
		if got := Resolve(base, "/etc/passwd"); got != "/etc/passwd" {
			t.Errorf("absolute path = %q", got)
		}
		// A backslash is an ordinary file-name character on Unix.
		if !IsInside(base, Resolve(base, `..\x`)) {
			t.Error(`..\x is a file name inside base on Unix`)
		}
	}
}

func TestRealPathFollowsLinks(t *testing.T) {
	root := t.TempDir()
	realRoot, err := RealPath(root)
	if err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "inside")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{inside, outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := RealPath(filepath.Join(outside, "secret.txt"))
	if err != nil || got != filepath.Join(realRoot, "outside", "secret.txt") {
		t.Fatalf("RealPath(plain file) = %q, %v", got, err)
	}

	link := filepath.Join(inside, "linked")
	if err := linkDir(outside, link); err != nil {
		t.Skipf("cannot create a directory link: %v", err)
	}
	got, err = RealPath(filepath.Join(link, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(realRoot, "outside", "secret.txt"); got != want {
		t.Errorf("RealPath through directory link = %q, want %q", got, want)
	}
	if IsInside(filepath.Join(realRoot, "inside"), got) {
		t.Error("linked file must resolve outside of inside/")
	}
	if _, err := RealPath(filepath.Join(root, "missing")); err == nil {
		t.Error("missing path must fail")
	}
}

func TestOpenOverwriteRefusesFinalSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("O_NOFOLLOW is Unix-only; Windows callers open fully resolved paths")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenOverwrite(link); err == nil {
		f.Close()
		t.Fatal("OpenOverwrite followed a symlink")
	}
	f, err := OpenOverwrite(target)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Errorf("OpenOverwrite truncated the file: %q", b)
	}
	if _, err := OpenOverwrite(filepath.Join(dir, "new")); err == nil {
		t.Error("OpenOverwrite must not create files")
	}
}
