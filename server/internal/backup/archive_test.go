package backup

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateArchiveLayout(t *testing.T) {
	e := newEnv(t)
	p := e.seed("src")
	links := symlinksSupported(t)
	var log strings.Builder
	at := time.Date(2026, 11, 27, 9, 52, 0, 0, time.UTC)
	c := &Creator{Paths: e.paths, DB: e.d.DB, Log: quiet(), Now: func() time.Time { return at }}
	name := c.Create(context.Background(), "nightly", func(s string) { log.WriteString(s) })
	if name != "nightly-backup-2026-11-27_09-52-00.zip" {
		t.Fatalf("name %q:\n%s", name, log.String())
	}
	mustContain(t, log.String(), "\n"+rule+"\n  CREATING BACKUP\n  2026-11-27T09:52:00.000Z\n"+rule+"\n\n",
		"[1/4] Adding database...\n      + docklift.db (", ") [consistent snapshot]\n",
		"\n[2/4] Adding deployments...\n      + /deployments/ (",
		"\n[3/5] Adding nginx configs...\n      + /nginx-conf/ (",
		"\n[4/5] Adding TLS certificates...\n      + /letsencrypt/ (",
		"\n[5/5] Adding GitHub App key...\n      + github-app.pem\n",
		"\nFinalizing backup...\n", "  BACKUP COMPLETE\n", "  File: "+name+"\n", "  Size: ", "  Location: "+e.paths.BackupDir+"\n")

	zr, err := zip.OpenReader(filepath.Join(e.paths.BackupDir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	got := map[string]*zip.File{}
	for _, f := range zr.File {
		if strings.Contains(f.Name, `\`) || strings.HasPrefix(f.Name, "/") {
			t.Fatalf("bad entry name %q", f.Name)
		}
		got[f.Name] = f
	}
	want := []string{
		"database/docklift.db",
		"deployments/.gitkeep",
		"deployments/" + p.ID + "/",
		"deployments/" + p.ID + "/Dockerfile",
		"deployments/" + p.ID + "/empty-dir/",
		"deployments/" + p.ID + "/src/",
		"deployments/" + p.ID + "/src/main.txt",
		"deployments/.docklift/",
		"deployments/.docklift/" + p.ID + "/",
		"deployments/.docklift/" + p.ID + "/compose.yml",
		"nginx-conf/service-src.conf",
		"letsencrypt/archive/",
		"letsencrypt/archive/src.test/cert1.pem",
		"github-app.pem",
	}
	for _, w := range want {
		f, ok := got[w]
		if !ok {
			t.Fatalf("archive lacks %s; has %d entries", w, len(got))
		}
		isDir := strings.HasSuffix(w, "/")
		if f.Mode().IsDir() != isDir {
			t.Fatalf("%s mode %v", w, f.Mode())
		}
		wantMethod := zip.Deflate
		if isDir {
			wantMethod = zip.Store
		}
		if f.Method != wantMethod || f.CreatorVersion>>8 != 3 {
			t.Fatalf("%s method %d creator %d", w, f.Method, f.CreatorVersion>>8)
		}
	}
	for _, top := range []string{"deployments/", "nginx-conf/", "letsencrypt/"} {
		if _, ok := got[top]; ok {
			t.Fatalf("archiver never stored the root entry %s", top)
		}
	}
	// The database entry is a SQLite file holding the live data.
	rc, _ := got["database/docklift.db"].Open()
	head := make([]byte, 16)
	_, _ = io.ReadFull(rc, head)
	rc.Close()
	if string(head) != "SQLite format 3\x00" {
		t.Fatalf("database entry head %q", head)
	}
	if links {
		f := got["letsencrypt/live/src.test/cert.pem"]
		if f == nil || f.Mode()&fs.ModeSymlink == 0 || f.Method != zip.Store {
			t.Fatalf("certbot link must be a stored symlink entry: %+v", f)
		}
		rc, _ := f.Open()
		target, _ := io.ReadAll(rc)
		rc.Close()
		if string(target) != "../../archive/src.test/cert1.pem" {
			t.Fatalf("link target %q", target)
		}
	}
	// No work files are left behind.
	for _, pattern := range []string{
		filepath.Join(e.paths.BackupDir, "*"+partialSuffix),
		filepath.Join(filepath.Dir(e.paths.DatabasePath), snapshotPrefix+"*"),
	} {
		if m, _ := filepath.Glob(pattern); len(m) > 0 {
			t.Fatalf("leftovers %v", m)
		}
	}
}

func TestCreateWithoutOptionalParts(t *testing.T) {
	e := newEnv(t)
	var log strings.Builder
	name := (&Creator{Paths: e.paths, DB: e.d.DB, Log: quiet()}).Create(context.Background(), DefaultLabel, func(s string) { log.WriteString(s) })
	if !strings.HasPrefix(name, "docklift-backup-") {
		t.Fatalf("name %q:\n%s", name, log.String())
	}
	mustContain(t, log.String(), "      ! Deployments directory not found\n", "      - Nginx config directory not found (skipped)\n",
		"      - No Let's Encrypt data (skipped)\n", "      - No GitHub App key configured (skipped)\n")
}

func TestCreateRefusesMissingDatabase(t *testing.T) {
	e := newEnv(t)
	e.paths.DatabasePath = filepath.Join(t.TempDir(), "missing.db")
	var log strings.Builder
	if name := (&Creator{Paths: e.paths, DB: e.d.DB, Log: quiet()}).Create(context.Background(), "x", func(s string) { log.WriteString(s) }); name != "" {
		t.Fatalf("backup without database created %q", name)
	}
	mustContain(t, log.String(), "\n[ERROR] Database not found at "+e.paths.DatabasePath+" — refusing incomplete backup\n")
	if m, _ := filepath.Glob(filepath.Join(e.paths.BackupDir, "*")); len(m) > 0 {
		t.Fatalf("files left: %v", m)
	}
}

func TestCreateFailureLeavesNoPartialArchive(t *testing.T) {
	e := newEnv(t)
	e.seed("src")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var log strings.Builder
	if name := (&Creator{Paths: e.paths, DB: e.d.DB, Log: quiet()}).Create(ctx, "x", func(s string) { log.WriteString(s) }); name != "" {
		t.Fatalf("cancelled backup created %q", name)
	}
	mustContain(t, log.String(), "\n[ERROR] Backup failed: ")
	entries, _ := os.ReadDir(e.paths.BackupDir)
	if len(entries) > 0 {
		t.Fatalf("files left: %v", entries)
	}
}

// The snapshot is consistent while writers keep committing (WAL): every
// row a writer committed before the snapshot is in it, and the copy passes
// an integrity check.
func TestSnapshotUnderConcurrentWrites(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.d.DB.Writer().Exec(`CREATE TABLE "t" ("n" INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	var stop atomic.Bool
	var wg sync.WaitGroup
	var committed atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load(); i++ {
			if _, err := e.d.DB.Writer().Exec(`INSERT INTO "t" VALUES (?)`, i); err == nil {
				committed.Add(1)
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	before := committed.Load()
	snap, err := SnapshotDatabase(ctx, e.d.DB, e.paths.DatabasePath)
	stop.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	defer removeSnapshot(snap)
	if err := checkIntegrity(ctx, snap); err != nil {
		t.Fatal(err)
	}
	conn, err := openSingle(snap)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int64
	if err := conn.QueryRow(`SELECT COUNT(*) FROM "t"`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < before {
		t.Fatalf("snapshot has %d rows, %d were committed before it", n, before)
	}
}

func TestSafeLinkTarget(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		ok           bool
	}{
		{"letsencrypt/live/a/cert.pem", "../../archive/a/cert1.pem", true},
		{"letsencrypt/live/a/cert.pem", "../../../etc/passwd", false},
		{"letsencrypt/live/a/cert.pem", "/etc/letsencrypt/archive/a/cert1.pem", false},
		{"letsencrypt/live/a/cert.pem", `..\..\archive\a\cert1.pem`, false},
		{"letsencrypt/live/a/cert.pem", "C:/x", false},
		{"letsencrypt/x", "y", true},
		{"letsencrypt/x", "..", false},
		{"letsencrypt/x", "", false},
		{"deployments/x", "y", false},
	} {
		if got := safeLinkTarget("letsencrypt", tc.name, tc.target); got != tc.ok {
			t.Errorf("safeLinkTarget(%q -> %q) = %v, want %v", tc.name, tc.target, got, tc.ok)
		}
	}
}

func TestSafeRelPath(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"deployments/a/b", true},
		{"deployments/./a", true},
		{"../x", false},
		{"a/../../x", false},
		{`a\..\x`, false},
		{"/abs", false},
		{"c:/x", false},
		{"a/.. /x", false},
		{"", false},
	} {
		if got := safeRelPath(tc.in); got != tc.ok {
			t.Errorf("safeRelPath(%q) = %v, want %v", tc.in, got, tc.ok)
		}
	}
}

func TestRelinkArea(t *testing.T) {
	if !symlinksSupported(t) {
		t.Skip("symlinks not supported")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "live", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"live/a/cert.pem", "live/a/evil.pem"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(f)), []byte("link text"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := relinkArea([]linkEntry{
		{"letsencrypt/live/a/cert.pem", "../../archive/a/cert1.pem"},
		{"letsencrypt/live/a/evil.pem", "../../../../etc/passwd"},
		{"letsencrypt/live/a/missing.pem", "x"},
		{"nginx-conf/x", "y"},
	}, "letsencrypt", dir)
	if err != nil || n != 1 {
		t.Fatalf("relinked %d, err %v", n, err)
	}
	if target, err := os.Readlink(filepath.Join(dir, "live", "a", "cert.pem")); err != nil || filepath.ToSlash(target) != "../../archive/a/cert1.pem" {
		t.Fatalf("link %q %v", target, err)
	}
	if fi, _ := os.Lstat(filepath.Join(dir, "live", "a", "evil.pem")); fi.Mode()&fs.ModeSymlink != 0 {
		t.Fatal("unsafe link target must stay a file")
	}
}

// Links chained through other links pass the lexical check but resolve
// outside the area on disk: x/l2 -> .. and x/l1 -> l2/../etc/passwd. In
// either archive order the escaping link is removed and the error reported.
func TestRelinkAreaChainedEscape(t *testing.T) {
	if !symlinksSupported(t) {
		t.Skip("symlinks not supported")
	}
	for _, order := range []string{"l1 first", "l2 first"} {
		t.Run(order, func(t *testing.T) {
			outside := t.TempDir()
			if err := os.MkdirAll(filepath.Join(outside, "etc"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "etc", "passwd"), []byte("root:x:0:0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(outside, "letsencrypt")
			if err := os.MkdirAll(filepath.Join(dir, "x"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{"x/l1", "x/l2"} {
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(f)), []byte("link text"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			l1 := linkEntry{"letsencrypt/x/l1", "l2/../etc/passwd"}
			l2 := linkEntry{"letsencrypt/x/l2", ".."}
			if !safeLinkTarget("letsencrypt", l1.name, l1.target) || !safeLinkTarget("letsencrypt", l2.name, l2.target) {
				t.Fatal("both targets pass the lexical check (that is the point of the test)")
			}
			links := []linkEntry{l1, l2}
			if order == "l2 first" {
				links = []linkEntry{l2, l1}
			}
			n, err := relinkArea(links, "letsencrypt", dir)
			if err == nil || n != 1 {
				t.Fatalf("relinked %d, err %v; want 1 and an error", n, err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "x", "l1")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("escaping link must be removed (err=%v)", err)
			}
			if b, err := os.ReadFile(filepath.Join(dir, "x", "l1")); err == nil {
				t.Fatalf("read through escaping link: %q", b)
			}
			if fi, err := os.Lstat(filepath.Join(dir, "x", "l2")); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
				t.Fatalf("contained link must stay (err=%v)", err)
			}
		})
	}
}

func TestLinkInside(t *testing.T) {
	if !symlinksSupported(t) {
		t.Skip("symlinks not supported")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "live", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(name, target string) string {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.Symlink(filepath.FromSlash(target), p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	real, _ := filepath.EvalSymlinks(root)
	for _, tc := range []struct {
		name, target string
		ok           bool
	}{
		{"live/a/cert.pem", "../../archive/a/cert1.pem", true},
		{"live/a/up", "../../..", false},
		{"live/a/self", "self", false},
	} {
		if got := linkInside(real, mk(tc.name, tc.target)); got != tc.ok {
			t.Errorf("linkInside(%s -> %s) = %v, want %v", tc.name, tc.target, got, tc.ok)
		}
	}
}
