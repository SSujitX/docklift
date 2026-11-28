package backup

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"docklift/internal/store"
)

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
