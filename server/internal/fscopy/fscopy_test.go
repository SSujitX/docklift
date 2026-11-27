package fscopy

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
)

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

func names(t *testing.T, dir string) []string {
	t.Helper()
	n, err := readNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	for _, n := range names(t, dir) {
		if strings.HasPrefix(n, ".docklift-restore-") {
			out = append(out, n)
		}
	}
	return out
}

func absent(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Errorf("%s should not exist (%v)", p, err)
	}
}

// withRename swaps the package rename for the duration of the test.
func withRename(t *testing.T, fn func(from, to string) error) {
	t.Helper()
	t.Cleanup(func() { rename = os.Rename })
	rename = fn
}

func dirs(t *testing.T) (source, target string) {
	root := t.TempDir()
	return filepath.Join(root, "source"), filepath.Join(root, "target")
}

func TestReplacesTargetContents(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "app", "new.txt"), "new")
	write(t, filepath.Join(target, "old-app", "old.txt"), "old")
	if err := ReplaceDirContents(source, target); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(target, "app", "new.txt")); got != "new" {
		t.Errorf("new.txt = %q", got)
	}
	absent(t, filepath.Join(target, "old-app"))
	if l := leftovers(t, target); len(l) != 0 {
		t.Errorf("leftovers %v", l)
	}
}

func TestClearsTargetWhenSourceEmpty(t *testing.T) {
	source, target := dirs(t)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(target, "gone.txt"), "x")
	if err := ReplaceDirContents(source, target); err != nil {
		t.Fatal(err)
	}
	if n := names(t, target); len(n) != 0 {
		t.Errorf("target = %v", n)
	}
}

func TestNeverRenamesTarget(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "a.txt"), "a")
	write(t, filepath.Join(target, "b.txt"), "b")
	var targetRenames []string
	withRename(t, func(from, to string) error {
		if filepath.Clean(from) == filepath.Clean(target) {
			targetRenames = append(targetRenames, from+" -> "+to)
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EBUSY}
		}
		return os.Rename(from, to)
	})
	if err := ReplaceDirContents(source, target); err != nil {
		t.Fatal(err)
	}
	if len(targetRenames) != 0 {
		t.Errorf("target renamed: %v", targetRenames)
	}
	if read(t, filepath.Join(target, "a.txt")) != "a" {
		t.Error("a.txt missing")
	}
	absent(t, filepath.Join(target, "b.txt"))
}

func TestRestoresWhenPromoteFails(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "new.txt"), "new")
	write(t, filepath.Join(target, "old.txt"), "old")
	errPromote := errors.New("promote fail")
	withRename(t, func(from, to string) error {
		if strings.Contains(from, StagingPrefix) {
			return errPromote
		}
		return os.Rename(from, to)
	})
	if err := ReplaceDirContents(source, target); !errors.Is(err, errPromote) {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(target, "old.txt")) != "old" {
		t.Error("old.txt not restored")
	}
	absent(t, filepath.Join(target, "new.txt"))
	if l := leftovers(t, target); len(l) != 0 {
		t.Errorf("leftovers %v", l)
	}
}

func TestRestoresAfterPartialPromote(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "a.txt"), "new-a")
	write(t, filepath.Join(source, "b.txt"), "new-b")
	write(t, filepath.Join(target, "old.txt"), "old")
	promotes := 0
	errPromote := errors.New("second promote fail")
	withRename(t, func(from, to string) error {
		if strings.Contains(from, StagingPrefix) {
			promotes++
			if promotes >= 2 {
				return errPromote
			}
		}
		return os.Rename(from, to)
	})
	if err := ReplaceDirContents(source, target); !errors.Is(err, errPromote) {
		t.Fatalf("err = %v", err)
	}
	if got := names(t, target); !slices.Equal(got, []string{"old.txt"}) {
		t.Errorf("target = %v, want only old.txt", got)
	}
}

func TestRestoresWhenMoveAsideFails(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "new.txt"), "new")
	write(t, filepath.Join(target, "a.txt"), "a")
	write(t, filepath.Join(target, "b.txt"), "b")
	asideMoves := 0
	errAside := errors.New("aside fail")
	withRename(t, func(from, to string) error {
		if strings.Contains(to, PrevPrefix) {
			asideMoves++
			if asideMoves >= 2 {
				return errAside
			}
		}
		return os.Rename(from, to)
	})
	if err := ReplaceDirContents(source, target); !errors.Is(err, errAside) {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(target, "a.txt")) != "a" || read(t, filepath.Join(target, "b.txt")) != "b" {
		t.Error("target not restored")
	}
	absent(t, filepath.Join(target, "new.txt"))
}

func TestReclaimsOrphanedPrev(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "fresh.txt"), "fresh")
	write(t, filepath.Join(target, "live.txt"), "live")
	write(t, filepath.Join(target, PrevPrefix+"oldcrash", "stranded.txt"), "stranded")
	write(t, filepath.Join(target, StagingPrefix+"oldcrash", "partial.txt"), "partial")
	if err := ReplaceDirContents(source, target); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(target, "fresh.txt")) != "fresh" {
		t.Error("fresh.txt missing")
	}
	absent(t, filepath.Join(target, "live.txt"))
	if l := leftovers(t, target); len(l) != 0 {
		t.Errorf("leftovers %v", l)
	}
}

func TestOrphanAndLiveShareName(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "app.txt"), "from-backup")
	write(t, filepath.Join(target, "app.txt"), "live")
	write(t, filepath.Join(target, PrevPrefix+"midpromote", "app.txt"), "old-aside")
	if err := ReplaceDirContents(source, target); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(target, "app.txt")); got != "from-backup" {
		t.Errorf("app.txt = %q", got)
	}
	if l := leftovers(t, target); len(l) != 0 {
		t.Errorf("leftovers %v", l)
	}
}

func TestSourceMissing(t *testing.T) {
	root := t.TempDir()
	err := ReplaceDirContents(filepath.Join(root, "missing"), filepath.Join(root, "target"))
	if err == nil || !strings.Contains(err.Error(), "source missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestMoveFallsBackToCopy(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "dir", "nested", "x.txt"), "x")
	write(t, filepath.Join(source, "file.txt"), "f")
	write(t, filepath.Join(target, "old", "y.txt"), "y")
	withRename(t, func(from, to string) error {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV}
	})
	if err := ReplaceDirContents(source, target); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(target, "dir", "nested", "x.txt")) != "x" || read(t, filepath.Join(target, "file.txt")) != "f" {
		t.Error("copy fallback lost content")
	}
	absent(t, filepath.Join(target, "old"))
	if l := leftovers(t, target); len(l) != 0 {
		t.Errorf("leftovers %v", l)
	}
}

func TestMoveDoesNotHideOtherErrors(t *testing.T) {
	source, target := dirs(t)
	write(t, filepath.Join(source, "a.txt"), "a")
	write(t, filepath.Join(target, "b.txt"), "b")
	withRename(t, func(from, to string) error {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.ENOENT}
	})
	if err := ReplaceDirContents(source, target); !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(target, "b.txt")) != "b" {
		t.Error("target changed")
	}
}

func TestCopyDirContents(t *testing.T) {
	source, dest := dirs(t)
	write(t, filepath.Join(source, "a", "b.txt"), "b")
	write(t, filepath.Join(source, "top.txt"), "new")
	write(t, filepath.Join(dest, "top.txt"), "old")
	write(t, filepath.Join(dest, "keep.txt"), "keep")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(source, "top.txt"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("a/b.txt", filepath.Join(source, "rel-link")); err != nil {
			t.Fatal(err)
		}
	}
	if err := CopyDirContents(source, dest); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dest, "a", "b.txt")) != "b" || read(t, filepath.Join(dest, "top.txt")) != "new" || read(t, filepath.Join(dest, "keep.txt")) != "keep" {
		t.Error("copy content mismatch")
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dest, "top.txt"))
		if err != nil || fi.Mode().Perm() != 0o750 {
			t.Errorf("mode = %v, %v", fi.Mode(), err)
		}
		target, err := os.Readlink(filepath.Join(dest, "rel-link"))
		if err != nil || target != filepath.Join(source, "a", "b.txt") {
			t.Errorf("link target = %q, %v (fs.cp resolves relative links)", target, err)
		}
	}
	if err := CopyDirContents(source, filepath.Join(source, "a", "inner")); err == nil {
		t.Error("copying into a subdirectory of itself must fail")
	}
	write(t, filepath.Join(dest, "clash", "x"), "x")
	write(t, filepath.Join(source, "clash"), "file")
	if err := CopyDirContents(source, dest); err == nil {
		t.Error("overwriting a directory with a file must fail")
	}
}
