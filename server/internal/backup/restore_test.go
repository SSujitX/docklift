package backup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"docklift/internal/db"
	"docklift/internal/maintenance"
	"docklift/internal/store"
)

// fixture is a source installation with an archive and a target
// installation with different content.
type fixture struct {
	src, dst         *env
	archive          string
	srcProject       *store.Project
	srcDeploy        map[string]string
	srcNginx         map[string]string
	srcLE            map[string]string
	dstDeploy        map[string]string
	dstNginx         map[string]string
	dstLE            map[string]string
	dstKey           string
	links            bool
	dstAdmins        []string
	dstPreRestoreKey string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{src: newEnv(t), dst: newEnv(t), links: symlinksSupported(t)}
	f.srcProject = f.src.seed("src")
	f.archive = f.src.backup()
	f.srcDeploy = tree(t, f.src.paths.DeploymentsPath)
	f.srcNginx = tree(t, f.src.paths.NginxConfPath)
	f.srcLE = tree(t, f.src.paths.LetsEncryptPath)
	f.dst.seed("dst")
	f.dst.docker.running = []string{"c1", "c2"}
	f.dstDeploy = tree(t, f.dst.paths.DeploymentsPath)
	f.dstNginx = tree(t, f.dst.paths.NginxConfPath)
	f.dstLE = tree(t, f.dst.paths.LetsEncryptPath)
	f.dstKey = readFile(t, f.dst.paths.GitHubKeyPath)
	f.dstAdmins = f.dst.admins()
	return f
}

func (f *fixture) expectOld(t *testing.T, what ...string) {
	t.Helper()
	for _, w := range what {
		switch w {
		case "db":
			if got := f.dst.admins(); strings.Join(got, ",") != strings.Join(f.dstAdmins, ",") {
				t.Fatalf("database must be the old one: admins %v", got)
			}
		case "deployments":
			sameTree(t, "deployments (old)", tree(t, f.dst.paths.DeploymentsPath), f.dstDeploy)
		case "nginx":
			sameTree(t, "nginx (old)", tree(t, f.dst.paths.NginxConfPath), f.dstNginx)
		case "letsencrypt":
			sameTree(t, "letsencrypt (old)", tree(t, f.dst.paths.LetsEncryptPath), f.dstLE)
		case "key":
			if readFile(t, f.dst.paths.GitHubKeyPath) != f.dstKey {
				t.Fatal("github key must be the old one")
			}
		}
	}
}

func (f *fixture) expectNew(t *testing.T, what ...string) {
	t.Helper()
	for _, w := range what {
		switch w {
		case "db":
			if got := f.dst.admins(); strings.Join(got, ",") != "src@example.test" {
				t.Fatalf("database must be the restored one: admins %v", got)
			}
		case "deployments":
			sameTree(t, "deployments (new)", tree(t, f.dst.paths.DeploymentsPath), f.srcDeploy)
		case "nginx":
			sameTree(t, "nginx (new)", tree(t, f.dst.paths.NginxConfPath), f.srcNginx)
		case "letsencrypt":
			sameTree(t, "letsencrypt (new)", tree(t, f.dst.paths.LetsEncryptPath), f.srcLE)
		case "key":
			if readFile(t, f.dst.paths.GitHubKeyPath) != "KEY src\n" {
				t.Fatal("github key must be the restored one")
			}
		}
	}
}

// expectClean asserts that no restore residue is left: no marker, no seal,
// no extraction directory.
func (f *fixture) expectClean(t *testing.T) {
	t.Helper()
	noFile(t, markerPath(f.dst.paths))
	if f.dst.d.Maintenance.Critical() {
		t.Fatal("instance must not be sealed")
	}
	matches, _ := filepath.Glob(filepath.Join(f.dst.paths.BackupDir, tempRestorePrefix+"*"))
	if len(matches) > 0 {
		t.Fatalf("extraction directories left: %v", matches)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	f := newFixture(t)
	f.dst.docker.running = []string{"c1"}
	outcome, log := f.dst.restore(f.archive, Request{Kind: KindBackup})
	if outcome != OutcomeCommitted {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	mustContain(t, log,
		"  RESTORING BACKUP\n", "  Source: "+filepath.Base(f.archive)+"\n\n",
		"[1/7] Extracting backup...\n", "      + Extraction complete\n", "      + Database integrity verified\n",
		"\n[2/7] Stopping user containers...\n      + Stopped user containers\n",
		"\n[3/7] Restoring database...\n", "      + Created backup of current database\n", "      + Database restored\n",
		"\n[4/7] Restoring deployments...\n      + Deployments restored\n",
		"\n[5/8] Restoring nginx configs...\n      + Nginx configs restored\n      + Nginx proxy reloaded\n",
		"\n[6/8] Restoring TLS certificates...\n      + Let's Encrypt certificates restored\n",
		"\n[7/8] Restoring GitHub App key...\n      + GitHub key restored\n",
		"\n[8/8] Restore archive retained for rollback...\n      + Kept: ",
		"  RECONCILING SYSTEM STATE\n", "[1/3] Reading restored database...\n      + Found 1 projects in database\n",
		"      > Redeploying src-app ("+f.srcProject.ID+")...\n", "        + Success (dl-src-app-",
		"\n[3/3] Reloading Nginx Proxy...\n      + Nginx configuration reloaded\n",
		"  RESTORE COMPLETE\n", "  [!] Restarting backend service to apply changes...\n")
	if strings.Contains(log, "[ERROR]") || strings.Contains(log, "uploaded file has been kept") {
		t.Fatalf("unexpected lines:\n%s", log)
	}
	f.expectNew(t, "db", "deployments", "nginx", "letsencrypt", "key")
	f.expectClean(t)
	if f.links && !strings.Contains(log, "      + Recreated 1 symlink(s)\n") {
		t.Fatalf("certbot link not recreated:\n%s", log)
	}
	// The rollback copy holds the previous database.
	pre := openDB(t, f.dst.paths.PreRestorePath())
	var n int
	if err := pre.Reader().QueryRow(`SELECT COUNT(*) FROM "users" WHERE "email" = 'dst@example.test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf(".pre-restore must hold the old database (n=%d err=%v)", n, err)
	}
	// Reconcile: volumes before compose up, then the proxy attached.
	d := f.dst.docker
	if strings.Join(d.stopped, ",") != "c1" || strings.Join(d.volumes, ",") != "dl-vol-src" ||
		len(d.ups) != 1 || !strings.HasSuffix(d.ups[0], "|"+f.srcProject.ID) || strings.Join(d.proxies, ",") != f.srcProject.ID {
		t.Fatalf("docker calls: %+v", d)
	}
	// The live handle works without a reopen and has the full schema.
	if got := appliedMigrations(context.Background(), f.dst.d.DB.Writer()); got < 5 {
		t.Fatalf("live ledger has %d migrations", got)
	}
	var mode string
	if err := f.dst.d.DB.Writer().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("live database must stay in WAL mode, got %q (%v)", mode, err)
	}
}

func TestRestoreUploadMarksArchive(t *testing.T) {
	f := newFixture(t)
	up := filepath.Join(f.dst.paths.UploadsDir(), "restore-1-00.zip")
	if err := os.MkdirAll(filepath.Dir(up), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.archive, up); err != nil {
		t.Fatal(err)
	}
	outcome, log := f.dst.restore(up, Request{Kind: KindUpload, Size: 1536})
	if outcome != OutcomeCommitted {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	mustContain(t, log, "  RESTORING FROM UPLOADED BACKUP\n", "  File: restore-1-00.zip\n  Size: 1.5 KB\n\n",
		"\n[8/8] Marking file as restored...\n      + Marked as restored: restore-1-00.restored-",
		"\n  The uploaded file has been kept. You can delete it manually from Settings.\n")
	noFile(t, up)
	matches, _ := filepath.Glob(filepath.Join(f.dst.paths.UploadsDir(), "restore-1-00.restored-*.zip"))
	if len(matches) != 1 {
		t.Fatalf("restored upload not kept: %v", matches)
	}
	// Restoring a marked file again keeps its name.
	again := newEnv(t)
	again.seed("again")
	_, log = again.restore(matches[0], Request{Kind: KindStoredUpload})
	mustContain(t, log, "  RESTORING FROM UPLOADED FILE\n", "      - Already marked as restored\n")
}

// Every stage fails once; the restore must end in the documented state.
func TestRestoreFailureAtEveryStage(t *testing.T) {
	type want struct {
		outcome Outcome
		old     []string
		new     []string
		stopped bool
		log     []string
	}
	all := []string{"db", "deployments", "nginx", "letsencrypt", "key"}
	for _, tc := range []struct {
		stage Stage
		want  want
	}{
		{StageExtract, want{OutcomeFailed, all, nil, false, []string{"[ERROR] Restore failed: injected extract"}}},
		{StageValidate, want{OutcomeFailed, all, nil, false, []string{"[ERROR] Restore failed: injected validate"}}},
		// Stopping containers only warns.
		{StageStopContainers, want{OutcomeCommitted, nil, all, false, []string{"      ! Container stop warning: injected stop-containers"}}},
		{StageMarker, want{OutcomeFailed, all, nil, true, nil}},
		{StagePreRestore, want{OutcomeFailed, all, nil, true, nil}},
		{StageDatabase, want{OutcomeFailed, all, nil, true, nil}},
		{StageMigrate, want{OutcomeFailed, all, nil, true, []string{"[ROLLBACK] Attempting database rollback", "+ Rolled database back from .pre-restore"}}},
		{StageDeployments, want{OutcomeFailed, all, nil, true, []string{"+ Rolled database back from .pre-restore"}}},
		// Like 2.1.6, trees already swapped stay restored; the database is rolled back.
		{StageNginx, want{OutcomeFailed, []string{"db", "nginx", "letsencrypt", "key"}, []string{"deployments"}, true, nil}},
		{StageLetsEncrypt, want{OutcomeFailed, []string{"db", "letsencrypt", "key"}, []string{"deployments", "nginx"}, true, nil}},
		{StageGitHubKey, want{OutcomeFailed, []string{"db", "key"}, []string{"deployments", "nginx", "letsencrypt"}, true, nil}},
		{StageMarkRestored, want{OutcomeFailed, []string{"db"}, []string{"deployments", "nginx", "letsencrypt", "key"}, true, nil}},
		// A reconcile failure keeps an authenticated restore (INCOMPLETE).
		{StageReconcile, want{OutcomeCommitted, nil, all, true, []string{"[ERROR] Reconcile failed: injected reconcile", "[ERROR] RESTORE INCOMPLETE"}}},
		{StageCommit, want{OutcomeFailed, []string{"db"}, []string{"deployments", "nginx", "letsencrypt", "key"}, true, []string{"+ Rolled database back from .pre-restore"}}},
		// Never reached on the success path.
		{StageRollback, want{OutcomeCommitted, nil, all, true, nil}},
	} {
		t.Run(string(tc.stage), func(t *testing.T) {
			f := newFixture(t)
			f.dst.r.hook = func(s Stage) error {
				if s == tc.stage {
					return fmt.Errorf("injected %s", s)
				}
				return nil
			}
			outcome, log := f.dst.restore(f.archive, Request{Kind: KindBackup})
			if outcome != tc.want.outcome {
				t.Fatalf("outcome %v, want %v:\n%s", outcome, tc.want.outcome, log)
			}
			f.expectOld(t, tc.want.old...)
			f.expectNew(t, tc.want.new...)
			f.expectClean(t)
			if stopped := len(f.dst.docker.stopped) > 0; stopped != tc.want.stopped {
				t.Fatalf("containers stopped = %v, want %v", stopped, tc.want.stopped)
			}
			mustContain(t, log, tc.want.log...)
			if outcome == OutcomeFailed && !strings.Contains(log, "\n[ERROR] Restore failed: injected ") {
				t.Fatalf("failure not reported:\n%s", log)
			}
		})
	}
}

// errCrash simulates the process dying (power loss, kill -9) at a stage: the
// hook panics, nothing after it runs except deferred cleanups.
type errCrash struct{ stage Stage }

// A crash at every stage, then a boot: the instance is sealed exactly when
// the crash happened past the point of no return, and the live database is
// always one consistent version.
func TestRestoreCrashAtEveryStage(t *testing.T) {
	sealedFrom := map[Stage]bool{
		StagePreRestore: true, StageDatabase: true, StageMigrate: true, StageDeployments: true, StageNginx: true,
		StageLetsEncrypt: true, StageGitHubKey: true, StageMarkRestored: true, StageReconcile: true, StageCommit: true,
	}
	for _, stage := range []Stage{
		StageExtract, StageValidate, StageStopContainers, StageMarker, StagePreRestore, StageDatabase, StageMigrate,
		StageDeployments, StageNginx, StageLetsEncrypt, StageGitHubKey, StageMarkRestored, StageReconcile, StageCommit,
	} {
		t.Run(string(stage), func(t *testing.T) {
			f := newFixture(t)
			f.dst.r.hook = func(s Stage) error {
				if s == stage {
					panic(errCrash{s})
				}
				return nil
			}
			func() {
				defer func() {
					if r := recover(); r == nil {
						t.Fatal("crash was not reached")
					}
				}()
				f.dst.restore(f.archive, Request{Kind: KindBackup})
			}()

			// Boot: a fresh process state over the same data directory.
			state := maintenance.New(f.dst.paths.DataPath, quiet())
			state.LoadCriticalOnBoot()
			RecoverInterrupted(f.dst.paths, state, quiet())
			CleanupOnBoot(f.dst.paths, quiet())
			noFile(t, markerPath(f.dst.paths))
			if state.Critical() != sealedFrom[stage] {
				t.Fatalf("sealed = %v, want %v", state.Critical(), sealedFrom[stage])
			}
			if sealedFrom[stage] {
				seal := state.ReadCritical()
				if seal == nil || !strings.Contains(seal.Detail, "Restore was interrupted during") ||
					seal.PreRestorePath != f.dst.paths.PreRestorePath() || !state.Active() || state.TryAcquireRestoreLock("x") {
					t.Fatalf("seal %+v active=%v", seal, state.Active())
				}
			} else {
				f.expectOld(t, "db", "deployments", "nginx", "letsencrypt", "key")
			}
			// Whatever happened, the live database is one consistent version.
			if err := checkIntegrity(context.Background(), f.dst.paths.DatabasePath); err != nil {
				t.Fatal(err)
			}
			if got := f.dst.admins(); len(got) != 1 {
				t.Fatalf("admins %v", got)
			}
			matches, _ := filepath.Glob(filepath.Join(f.dst.paths.BackupDir, tempRestorePrefix+"*"))
			if len(matches) > 0 {
				t.Fatalf("boot cleanup left %v", matches)
			}
		})
	}
}

func TestRestoreRollbackFailureSeals(t *testing.T) {
	f := newFixture(t)
	f.dst.r.hook = func(s Stage) error {
		if s == StageNginx || s == StageRollback {
			return errors.New("injected " + string(s))
		}
		return nil
	}
	outcome, log := f.dst.restore(f.archive, Request{Kind: KindBackup})
	if outcome != OutcomeCritical {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	mustContain(t, log, "      ! DB rollback FAILED: injected rollback\n", "\n[CRITICAL] Rollback failed — sealed; do not retry\n",
		"  Paths: "+f.dst.paths.DatabasePath+" | "+f.dst.paths.PreRestorePath()+"\n", "\n[ERROR] Restore failed: injected nginx-conf\n")
	st := f.dst.d.Maintenance
	seal := st.ReadCritical()
	if !st.Critical() || seal == nil || seal.Detail != "injected nginx-conf" || !st.Active() || !st.RestoreLocked() {
		t.Fatalf("not sealed: %+v", seal)
	}
	noFile(t, markerPath(f.dst.paths))
	// The seal survives a restart and blocks the next restore.
	boot := maintenance.New(f.dst.paths.DataPath, quiet())
	boot.LoadCriticalOnBoot()
	RecoverInterrupted(f.dst.paths, boot, quiet())
	if !boot.Critical() || boot.TryAcquireRestoreLock("again") || boot.ReadCritical().Detail != "injected nginx-conf" {
		t.Fatal("seal must survive the restart unchanged")
	}
}

func TestRestoreCommitPolicyFlows(t *testing.T) {
	t.Run("setup restore with failed reconcile rolls back and keeps the secrets", func(t *testing.T) {
		f := newFixture(t)
		f.dst.nginx.err = errors.New("nginx down")
		tok, err := f.dst.d.Auth.Bootstrap().EnsureSetupToken()
		if err != nil {
			t.Fatal(err)
		}
		outcome, log := f.dst.restore(f.archive, Request{Kind: KindUpload, SetupAuth: true})
		if outcome != OutcomeAborted {
			t.Fatalf("outcome %v:\n%s", outcome, log)
		}
		mustContain(t, log, "      [ERROR] Nginx reload failed: nginx down\n",
			"[ERROR] Reconcile finished with 1 failure(s) — restore is incomplete\n",
			"\n[ROLLBACK] Restore not committed: Reconciliation incomplete\n", "      + Rolled database back from .pre-restore\n",
			"  [security] Setup credentials retained for retry\n", "  [ERROR] RESTORE ABORTED\n")
		f.expectOld(t, "db")
		f.expectClean(t)
		if !f.dst.d.Auth.Bootstrap().ValidateSetupToken(tok) {
			t.Fatal("setup token must survive an aborted restore")
		}
	})
	t.Run("setup restore commits and consumes the secrets", func(t *testing.T) {
		f := newFixture(t)
		b := f.dst.d.Auth.Bootstrap()
		tok, _ := b.EnsureSetupToken()
		if _, err := b.EnsureSecret(); err != nil {
			t.Fatal(err)
		}
		outcome, log := f.dst.restore(f.archive, Request{Kind: KindUpload, SetupAuth: true})
		if outcome != OutcomeCommitted {
			t.Fatalf("outcome %v:\n%s", outcome, log)
		}
		mustContain(t, log, "\n  [security] Setup token + bootstrap secret consumed\n", "  RESTORE COMPLETE\n")
		if b.ValidateSetupToken(tok) || b.ReadSecret() != "" {
			t.Fatal("setup token and bootstrap secret must be consumed")
		}
	})
	t.Run("archive without a database on an install without admin aborts without rollback", func(t *testing.T) {
		dst := newEnv(t)
		archive := writeZip(t, map[string]string{"nginx-conf/README.txt": "restored\n"})
		outcome, log := dst.restore(archive, Request{Kind: KindUpload, SetupAuth: true})
		if outcome != OutcomeAborted {
			t.Fatalf("outcome %v:\n%s", outcome, log)
		}
		mustContain(t, log, "      ! No database in backup\n",
			"[ROLLBACK] Restore not committed: Restored database has no administrator (role=admin)",
			"[security] Setup credentials retained for retry", "[ERROR] RESTORE ABORTED")
		if strings.Contains(log, "Rolled database back") {
			t.Fatalf("nothing to roll back:\n%s", log)
		}
		noFile(t, markerPath(dst.paths))
	})
	t.Run("restored database without admin is rolled back", func(t *testing.T) {
		src := newEnv(t)
		archive := src.backup()
		f := newFixture(t)
		outcome, log := f.dst.restore(archive, Request{Kind: KindBackup})
		if outcome != OutcomeAborted {
			t.Fatalf("outcome %v:\n%s", outcome, log)
		}
		mustContain(t, log, "[ROLLBACK] Restore not committed: Restored database has no administrator (role=admin)", "+ Rolled database back from .pre-restore")
		f.expectOld(t, "db")
		f.expectClean(t)
	})
}

// writeZip builds an archive with the given file entries.
func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "archive.zip")
	out, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRestoreUpgradesOlderDatabase(t *testing.T) {
	// A database at the first migration only, with an admin.
	dir := t.TempDir()
	old := filepath.Join(dir, "old.db")
	d, err := db.Open(context.Background(), old)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Writer().Exec(migs[0].SQL); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkApplied(context.Background(), migs[0].Name); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Writer().Exec(`INSERT INTO "users" ("id","email","password","name","role","created_at") VALUES ('u1','old@example.test','x','Old','admin',1)`); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(old)
	if err != nil {
		t.Fatal(err)
	}
	archive := writeZip(t, map[string]string{"database/docklift.db": string(raw)})
	dst := newEnv(t)
	dst.seed("dst")
	outcome, log := dst.restore(archive, Request{Kind: KindBackup})
	if outcome != OutcomeCommitted {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	mustContain(t, log, fmt.Sprintf("      + Database schema upgraded (%d migration(s))\n", len(migs)-1))
	if got := dst.admins(); strings.Join(got, ",") != "old@example.test" {
		t.Fatalf("admins %v", got)
	}
	if got := appliedMigrations(context.Background(), dst.d.DB.Writer()); got != len(migs) {
		t.Fatalf("live ledger has %d of %d migrations", got, len(migs))
	}
}

func TestRestoreRefusesBadArchives(t *testing.T) {
	for _, tc := range []struct {
		name    string
		archive func(t *testing.T) string
		msg     string
	}{
		{"not a zip", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "x.zip")
			if err := os.WriteFile(p, []byte("not a zip"), 0o644); err != nil {
				t.Fatal(err)
			}
			return p
		}, "[ERROR] Restore failed: open zip: "},
		{"zip slip", func(t *testing.T) string {
			return writeZip(t, map[string]string{"../evil.txt": "x"})
		}, "[ERROR] Restore failed: Path traversal in archive: ../evil.txt"},
		{"database is not sqlite", func(t *testing.T) string {
			return writeZip(t, map[string]string{"database/docklift.db": "garbage garbage garbage", "nginx-conf/x.conf": "x"})
		}, "[ERROR] Restore failed: backup database is not usable: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := newEnv(t)
			dst.seed("dst")
			dst.docker.running = []string{"c1"}
			before := tree(t, dst.paths.NginxConfPath)
			outcome, log := dst.restore(tc.archive(t), Request{Kind: KindBackup})
			if outcome != OutcomeFailed {
				t.Fatalf("outcome %v:\n%s", outcome, log)
			}
			mustContain(t, log, tc.msg)
			sameTree(t, "nginx", tree(t, dst.paths.NginxConfPath), before)
			if len(dst.docker.stopped) != 0 {
				t.Fatal("a rejected archive must not stop containers")
			}
			noFile(t, markerPath(dst.paths))
			noFile(t, filepath.Join(filepath.Dir(dst.paths.DataPath), "evil.txt"))
		})
	}
}

func TestCheckSpace(t *testing.T) {
	dir := t.TempDir()
	if err := checkSpace(filepath.Join(dir, "missing", "deeper"), 1, "x"); err != nil {
		t.Fatalf("small need must pass: %v", err)
	}
	if _, ok := freeSpace(dir); !ok {
		t.Skip("free space unknown on this system")
	}
	err := checkSpace(dir, 1<<62, "extract the backup")
	if err == nil || !strings.HasPrefix(err.Error(), "Not enough disk space to extract the backup: ") {
		t.Fatalf("err = %v", err)
	}
}
