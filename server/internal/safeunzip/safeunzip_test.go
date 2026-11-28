package safeunzip

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type entry struct {
	name string
	body string
	mode os.FileMode
}

func writeZip(t *testing.T, entries ...entry) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			fh.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExtractWritesFiles(t *testing.T) {
	zp := writeZip(t,
		entry{name: "app/"},
		entry{name: "app/index.js", body: "console.log(1)"},
		entry{name: `win\style\path.txt`, body: "bs"},
		entry{name: "./dot/./x.txt", body: "dot"},
		entry{name: "a//b.txt", body: "ab"},
		entry{name: "empty-dir/"},
		entry{name: "link", body: "/etc/passwd", mode: os.ModeSymlink | 0o777},
	)
	dest := filepath.Join(t.TempDir(), "out")
	if err := Extract(context.Background(), zp, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"app/index.js":       "console.log(1)",
		"win/style/path.txt": "bs",
		"dot/x.txt":          "dot",
		"a/b.txt":            "ab",
		"link":               "/etc/passwd",
	} {
		if got := readFile(t, filepath.Join(dest, filepath.FromSlash(p))); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	if fi, err := os.Lstat(filepath.Join(dest, "link")); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("symlink entry must become a regular file: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "empty-dir")); !os.IsNotExist(err) {
		t.Errorf("directory entries are skipped like unzipper: %v", err)
	}
}

func TestExtractRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"../evil.txt", "Path traversal in archive: ../evil.txt"},
		{"a/../../evil.txt", "Path traversal in archive: a/../../evil.txt"},
		{`..\evil.txt`, `Path traversal in archive: ..\evil.txt`},
		{"/etc/passwd", "Unsafe path in archive: /etc/passwd"},
		{`\abs.txt`, `Unsafe path in archive: \abs.txt`},
		{"nul\x00byte", "Unsafe path in archive: nul\x00byte"},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests,
			struct{ name, msg string }{`C:\Windows\evil.txt`, `Unsafe path in archive: C:\Windows\evil.txt`},
			struct{ name, msg string }{"C:evil.txt", "Unsafe path in archive: C:evil.txt"},
			struct{ name, msg string }{"file.txt:stream", "Unsafe path in archive: file.txt:stream"},
			struct{ name, msg string }{".. /evil.txt", "Path traversal in archive: .. /evil.txt"},
			struct{ name, msg string }{".../evil.txt", "Path traversal in archive: .../evil.txt"},
		)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dest := filepath.Join(root, "out")
			err := Extract(context.Background(), writeZip(t, entry{name: tc.name, body: "x"}), dest, Options{})
			if err == nil || err.Error() != tc.msg || !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("err = %v, want %q", err, tc.msg)
			}
			if _, err := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(err) {
				t.Error("file escaped the destination")
			}
		})
	}
}

func TestExtractQuotas(t *testing.T) {
	ctx := context.Background()
	many := writeZip(t, entry{name: "a", body: "1"}, entry{name: "b", body: "2"}, entry{name: "dir/"}, entry{name: "c", body: "3"})
	err := Extract(ctx, many, t.TempDir(), Options{MaxFiles: 2})
	if err == nil || err.Error() != "Archive contains too many files (max 2)" || !errors.Is(err, ErrTooManyFiles) {
		t.Errorf("max files: %v", err)
	}
	if err := Extract(ctx, many, t.TempDir(), Options{MaxFiles: 3}); err != nil {
		t.Errorf("directories do not count: %v", err)
	}

	big := writeZip(t, entry{name: "a", body: strings.Repeat("x", 600)}, entry{name: "b", body: strings.Repeat("y", 600)})
	err = Extract(ctx, big, t.TempDir(), Options{MaxUncompressedBytes: 1000})
	if err == nil || err.Error() != "Archive uncompressed size exceeds limit (1000 bytes)" || !errors.Is(err, ErrTooLarge) {
		t.Errorf("total bytes: %v", err)
	}
	err = Extract(ctx, big, t.TempDir(), Options{MaxUncompressedBytes: 500})
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("single entry over limit: %v", err)
	}
}

// A lying header (declared size smaller than the real stream) is still
// stopped by the count of bytes actually written.
func TestExtractCountsActualBytes(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateRaw(&zip.FileHeader{Name: "bomb", Method: zip.Store, UncompressedSize64: 1, CompressedSize64: 4096, CRC32: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("z"), 4096)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zp := filepath.Join(t.TempDir(), "bomb.zip")
	if err := os.WriteFile(zp, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Extract(context.Background(), zp, t.TempDir(), Options{MaxUncompressedBytes: 1024})
	if err == nil {
		t.Fatal("expected an error for a lying header")
	}
}

func TestExtractRefusesExistingLinkOutside(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "out")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{dest, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := linkDir(outside, filepath.Join(dest, "sub")); err != nil {
		t.Skipf("cannot create a directory link: %v", err)
	}
	err := Extract(context.Background(), writeZip(t, entry{name: "sub/evil.txt", body: "x"}), dest, Options{})
	if err == nil || err.Error() != "Zip-slip blocked: sub/evil.txt" {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); !os.IsNotExist(err) {
		t.Error("write followed a link out of the destination")
	}
}

func TestExtractCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Extract(ctx, writeZip(t, entry{name: "a", body: "1"}), t.TempDir(), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestExtractBadArchive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(p, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Extract(context.Background(), p, t.TempDir(), Options{}); err == nil {
		t.Error("expected an error")
	}
}
