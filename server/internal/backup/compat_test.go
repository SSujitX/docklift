package backup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"docklift/internal/store"
)

// tsBackend returns the TypeScript backend directory (for the 2.1.6
// archiver and restore code) and the node binary, or skips.
func tsBackend(t *testing.T) (dir, node string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir, err = filepath.Abs(filepath.Join("..", "..", "..", "backend"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"node_modules/archiver/index.js", "node_modules/unzipper", "dist/lib/safeUnzip.js", "dist/lib/fsCopy.js"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Skipf("TypeScript backend not built (%s missing)", p)
		}
	}
	return dir, node
}

func runNode(t *testing.T, node string, args ...string) {
	t.Helper()
	out, err := exec.Command(node, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("node %s: %v\n%s", filepath.Base(args[0]), err, out)
	}
}

// An archive written by the 2.1.6 code (archiver, same calls as backup.ts)
// restores on the Go side, links included.
func TestRestoreArchiveFromTypeScript(t *testing.T) {
	backend, node := tsBackend(t)
	f := newFixture(t)
	snap, err := SnapshotDatabase(context.Background(), f.src.d.DB, f.src.paths.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer removeSnapshot(snap)
	archive := filepath.Join(t.TempDir(), "ts-backup.zip")
	runNode(t, node, filepath.Join("testdata", "ts_archive.mjs"), filepath.Join(backend, "node_modules"), archive, snap,
		f.src.paths.DeploymentsPath, f.src.paths.NginxConfPath, f.src.paths.LetsEncryptPath, f.src.paths.GitHubKeyPath)

	outcome, log := f.dst.restore(archive, Request{Kind: KindBackup})
	if outcome != OutcomeCommitted {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	f.expectNew(t, "db", "deployments", "nginx", "letsencrypt", "key")
	f.expectClean(t)
}

// An archive written by Go restores with the 2.1.6 code (safeExtractZip +
// replaceDirContents): same trees, a valid database. 2.1.6 writes link
// entries as files holding the link text, as it does for its own archives.
func TestTypeScriptRestoresGoArchive(t *testing.T) {
	backend, node := tsBackend(t)
	src := newEnv(t)
	src.seed("src")
	archive := src.backup()
	target := t.TempDir()
	runNode(t, node, filepath.Join("testdata", "ts_extract.mjs"), filepath.Join(backend, "dist", "lib"), archive, t.TempDir(), target)

	for area, dir := range map[string]string{
		"deployments": src.paths.DeploymentsPath, "nginx-conf": src.paths.NginxConfPath, "letsencrypt": src.paths.LetsEncryptPath,
	} {
		want := tree(t, dir)
		for k, v := range want {
			// unzipper skips directory entries (empty directories are lost)
			// and writes links as text files.
			if strings.HasSuffix(k, "/") {
				delete(want, k)
			}
			if strings.HasPrefix(v, "-> ") {
				want[k] = strings.TrimPrefix(v, "-> ")
			}
		}
		got := tree(t, filepath.Join(target, area))
		for k := range got {
			if strings.HasSuffix(k, "/") {
				delete(got, k)
			}
		}
		sameTree(t, area, got, want)
	}
	if readFile(t, filepath.Join(target, "github-app.pem")) != "KEY src\n" {
		t.Fatal("github key not restored by 2.1.6")
	}
	restored := openDB(t, filepath.Join(target, "docklift.db"))
	var email string
	if err := restored.Reader().QueryRow(`SELECT "email" FROM "users" WHERE "role" = 'admin'`).Scan(&email); err != nil || email != "src@example.test" {
		t.Fatalf("restored database admin %q (%v)", email, err)
	}
	if err := checkIntegrity(context.Background(), filepath.Join(target, "docklift.db")); err != nil {
		t.Fatal(err)
	}
}

// DOCKLIFT_REAL_BACKUP=<path to a 2.1.6 production backup> restores it into
// a temporary installation. Nothing of its content is printed.
func TestRestoreRealBackup(t *testing.T) {
	archive := os.Getenv("DOCKLIFT_REAL_BACKUP")
	if archive == "" {
		t.Skip("set DOCKLIFT_REAL_BACKUP to a 2.1.6 backup archive")
	}
	links, err := archiveLinks(archive)
	if err != nil {
		t.Fatal(err)
	}
	dst := newEnv(t)
	dst.seed("dst")
	canLink := symlinksSupported(t)
	outcome, log := dst.restore(archive, Request{Kind: KindBackup})
	if outcome != OutcomeCommitted {
		t.Fatalf("outcome %v; problems: %v", outcome, problems(log, canLink))
	}
	if errs := problems(log, canLink); len(errs) > 0 {
		t.Fatalf("restore reported problems: %v", errs)
	}
	if len(dst.admins()) == 0 {
		t.Fatal("restored database has no admin")
	}
	projects, err := dst.d.Store.ListProjects(context.Background(), store.ProjectFilter{}, store.OrderNone)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, area := range []struct{ name, dir string }{
		{"deployments", dst.paths.DeploymentsPath}, {"nginx-conf", dst.paths.NginxConfPath}, {"letsencrypt", dst.paths.LetsEncryptPath},
	} {
		counts[area.name] = len(tree(t, area.dir))
	}
	relinked := 0
	for _, m := range regexp.MustCompile(`\+ Recreated (\d+) symlink\(s\)`).FindAllStringSubmatch(log, -1) {
		n, _ := strconv.Atoi(m[1])
		relinked += n
	}
	t.Logf("restored %d projects; entries: %v; link entries in archive: %d; links recreated: %d", len(projects), counts, len(links), relinked)
	if canLink && relinked != len(links) {
		t.Fatalf("recreated %d of %d links", relinked, len(links))
	}
}

// problems lists the error and warning lines of a restore log, cut before
// their first colon so no path or content of the archive is printed. Link
// failures are expected where the OS refuses symlinks.
func problems(log string, canLink bool) []string {
	var out []string
	for _, l := range strings.Split(log, "\n") {
		l = strings.TrimSpace(l)
		if !strings.Contains(l, "[ERROR]") && !strings.Contains(l, "[CRITICAL]") && !strings.HasPrefix(l, "! ") {
			continue
		}
		if !canLink && strings.HasPrefix(l, "! Could not recreate symlinks") {
			continue
		}
		if c := strings.Index(l, ":"); c >= 0 {
			l = l[:c]
		}
		out = append(out, l)
	}
	return out
}
