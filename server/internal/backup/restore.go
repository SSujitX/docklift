package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/fscopy"
	"docklift/internal/maintenance"
	"docklift/internal/nginx"
	"docklift/internal/safeunzip"
	"docklift/internal/store"
)

// Kind is the entry point of a restore (it changes the log header and what
// happens to the archive afterwards).
type Kind int

const (
	// KindBackup is POST /restore/:filename (a server-created backup).
	KindBackup Kind = iota
	// KindUpload is POST /restore-upload (uploaded with the request).
	KindUpload
	// KindStoredUpload is POST /restore-from-upload/:filename.
	KindStoredUpload
)

// Request is one restore.
type Request struct {
	Kind Kind
	// ArchivePath is the zip on disk.
	ArchivePath string
	// Filename is its name in BACKUP_PATH or BACKUP_PATH/uploads.
	Filename string
	// Size is shown in the header of upload restores.
	Size int64
	// SetupAuth is a fresh-install restore authorised by the setup token.
	SetupAuth bool
}

// Outcome is how a restore ended; the caller maps it to the maintenance
// mode and restore lock.
type Outcome int

const (
	// OutcomeCommitted: the restored data is live. Keep maintenance and the
	// lock and restart the process (as 2.1.6 did).
	OutcomeCommitted Outcome = iota
	// OutcomeAborted: the commit policy refused the restore and the database
	// was rolled back (or never replaced). Exit maintenance, release the lock.
	OutcomeAborted
	// OutcomeFailed: an error stopped the restore; the database is as before
	// (never replaced, or rolled back). Exit maintenance, release the lock.
	OutcomeFailed
	// OutcomeCritical: the database could not be rolled back. The seal is on
	// disk; maintenance and the lock stay on until an operator clears it.
	OutcomeCritical
)

// Stage names a restore step. They appear in the in-progress marker (and
// so in the seal detail after a crash) and are the failure-injection points
// of the tests.
type Stage string

// Restore stages in execution order.
const (
	StageExtract        Stage = "extract"
	StageValidate       Stage = "validate"
	StageStopContainers Stage = "stop-containers"
	StageMarker         Stage = "marker"
	StagePreRestore     Stage = "pre-restore copy"
	StageDatabase       Stage = "database"
	StageMigrate        Stage = "database migration"
	StageDeployments    Stage = "deployments"
	StageNginx          Stage = "nginx-conf"
	StageLetsEncrypt    Stage = "letsencrypt"
	StageGitHubKey      Stage = "github-app.pem"
	StageMarkRestored   Stage = "mark archive"
	StageReconcile      Stage = "reconcile"
	StageCommit         Stage = "commit"
	StageRollback       Stage = "rollback"
)

// Reloader reloads nginx-proxy at the end of reconciliation
// (*nginx.Service: d.Edge.Nginx).
type Reloader interface {
	Reload(ctx context.Context) error
}

// SetupSecrets consumes the one-time setup credentials after a committed
// fresh-install restore (*auth.Bootstrap).
type SetupSecrets interface {
	ConsumeSetupRestoreSecrets()
}

// Restorer applies backup archives to the live installation. One value can
// serve many restores; the restore lock (held by the caller) serialises them.
type Restorer struct {
	Paths  Paths
	DB     *db.DB
	Store  *store.Store
	State  *maintenance.State
	Docker Docker
	Nginx  Reloader
	Setup  SetupSecrets
	Log    *slog.Logger
	// Now is the clock (time.Now when nil).
	Now func() time.Time

	// hook runs before each stage; a non-nil error is injected as that
	// stage's failure (tests only).
	hook func(Stage) error
}

func (r *Restorer) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// run is the state of one restore.
type run struct {
	r       *Restorer
	req     Request
	out     func(string)
	started time.Time
	tempDir string

	staged     stagedDB
	hasDB      bool
	links      []linkEntry
	markerOn   bool
	dbReplaced bool
}

func (x *run) stage(s Stage) error {
	if x.r.hook == nil {
		return nil
	}
	return x.r.hook(s)
}

// mark advances the durable in-progress marker to s. A marker that cannot
// be written stops the restore: without it a crash would not seal.
func (x *run) mark(s Stage) error {
	if err := writeMarker(x.r.Paths, markerFor(x.r.Paths, x.req.Filename, string(s), x.started)); err != nil {
		return fmt.Errorf("write restore marker: %w", err)
	}
	x.markerOn = true
	return nil
}

func (x *run) unmark() {
	if !x.markerOn {
		return
	}
	if err := clearMarker(x.r.Paths); err != nil {
		x.r.Log.Error("could not remove the restore-in-progress marker", "error", err)
		return
	}
	x.markerOn = false
}

// Run restores req.ArchivePath, writing the progress log to out, and
// returns the outcome. ctx may be cancelled only until the point of no
// return; after it the restore always runs to commit or rollback.
//
// Power-loss / crash analysis (what the next boot finds):
//
//   - header, extraction, validation (stages extract, validate): only the
//     temporary directory BACKUP_PATH/temp-restore-* exists; live data is
//     untouched and no marker is written. Boot: CleanupOnBoot removes the
//     directory; the instance starts normally.
//   - stop-containers: user containers may be stopped; data untouched, no
//     marker. Boot: normal start (deployment recovery reconciles statuses).
//   - marker written (point of no return) → every later crash boots sealed:
//     RecoverInterrupted turns .restore-in-progress into .restore-critical
//     (maintenance + lock), naming the stage that was running.
//   - pre-restore copy: written to a temp file and renamed, so .pre-restore
//     is the old complete copy or the new complete copy.
//   - database: one SQLite transaction (online-backup API); the live
//     database is entirely old or entirely new.
//   - database migration: Ensure on the live database runs migrations in
//     their own transactions, already proven on the staged copy.
//   - deployments / nginx-conf / letsencrypt: fscopy.ReplaceDirContents
//     stages the new tree inside the target, moves live entries to a prev
//     directory and promotes staging entry by entry. A crash leaves a mix of
//     promoted entries and the complete old content in
//     .docklift-restore-prev-* inside the target (the seal tells the
//     operator; the next restore folds and reclaims it).
//   - github-app.pem: temp file + rename: old or new key.
//   - mark archive, reconcile: data fully restored; the seal is still
//     raised (fail closed) because the commit decision was not reached.
//   - commit: the marker is removed only after the commit decision (and
//     after a successful rollback for aborts); a crash after that boots
//     normally into the restored (or rolled back) data.
func (r *Restorer) Run(ctx context.Context, req Request, out func(string)) Outcome {
	x := &run{r: r, req: req, out: out, started: r.now()}
	x.header()
	x.tempDir = filepath.Join(r.Paths.BackupDir, tempRestorePrefix+strconv.FormatInt(x.started.UnixMilli(), 10))
	// Temporary files go on every path; a crash leaves them to CleanupOnBoot.
	defer func() {
		if err := os.RemoveAll(x.tempDir); err != nil {
			r.Log.Warn("could not remove restore temp directory", "path", x.tempDir, "error", err)
		}
	}()

	if err := x.prepare(ctx); err != nil {
		out("\n[ERROR] Restore failed: " + err.Error() + "\n")
		r.Log.Error("Restore error", "error", err)
		return OutcomeFailed
	}
	// Past this point a half-applied restore must finish or roll back: client
	// disconnects and shutdown no longer cancel it.
	live := context.WithoutCancel(ctx)
	if err := x.apply(live); err != nil {
		return x.fail(live, err)
	}
	return x.finish(live, x.reconcile(live))
}

func (x *run) header() {
	x.out("\n" + rule + "\n")
	switch x.req.Kind {
	case KindBackup:
		x.out("  RESTORING BACKUP\n")
	case KindUpload:
		x.out("  RESTORING FROM UPLOADED BACKUP\n")
	default:
		x.out("  RESTORING FROM UPLOADED FILE\n")
	}
	x.out("  " + ISO(x.started) + "\n")
	x.out(rule + "\n")
	if x.req.Kind == KindBackup {
		x.out("  Source: " + x.req.Filename + "\n\n")
		return
	}
	x.out("  File: " + x.req.Filename + "\n")
	x.out("  Size: " + FormatBytes(x.req.Size) + "\n\n")
}

// prepare extracts and validates the archive without touching live data.
// (2.1.6 stopped the user containers first; doing it after validation means
// a corrupt or incompatible archive no longer takes every app down.)
func (x *run) prepare(ctx context.Context) error {
	x.out("[1/7] Extracting backup...\n")
	if err := os.RemoveAll(x.tempDir); err != nil {
		return err
	}
	if err := mkPrivateDir(x.tempDir); err != nil {
		return err
	}
	if err := x.stage(StageExtract); err != nil {
		return err
	}
	need, err := archiveSize(x.req.ArchivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	if err := checkSpace(x.tempDir, need, "extract the backup"); err != nil {
		return err
	}
	// The TS limits for restores: 50 000 files, 2 GiB uncompressed.
	if err := safeunzip.Extract(ctx, x.req.ArchivePath, x.tempDir, safeunzip.Options{}); err != nil {
		return err
	}
	if err := restoreDirs(x.req.ArchivePath, x.tempDir); err != nil {
		return err
	}
	x.out("      + Extraction complete\n")
	if x.links, err = archiveLinks(x.req.ArchivePath); err != nil {
		return fmt.Errorf("read zip: %w", err)
	}

	staged := filepath.Join(x.tempDir, filepath.FromSlash(entryDatabase))
	if st, err := os.Stat(staged); err == nil && st.Mode().IsRegular() {
		if err := x.stage(StageValidate); err != nil {
			return err
		}
		size, err := pageSize(ctx, x.r.DB)
		if err != nil {
			return fmt.Errorf("read live database page size: %w", err)
		}
		if x.staged, err = prepareStagedDB(ctx, staged, size, x.r.Log); err != nil {
			return fmt.Errorf("backup database is not usable: %w", err)
		}
		x.hasDB = true
		x.out("      + Database integrity verified\n")
		if x.staged.migrations > 0 {
			x.out(fmt.Sprintf("      + Database schema upgraded (%d migration(s))\n", x.staged.migrations))
		}
	} else if err == nil {
		return fmt.Errorf("%s in the archive is not a file", entryDatabase)
	}
	return x.checkTargetsSpace()
}

// checkTargetsSpace verifies that every restore target can hold its staged
// copy (ReplaceDirContents copies into the target before swapping) and that
// the data directory can hold the .pre-restore copy plus the WAL growth of
// the swap.
func (x *run) checkTargetsSpace() error {
	p := x.r.Paths
	for _, a := range []struct{ name, target string }{
		{entryDeployments, p.DeploymentsPath},
		{entryNginx, p.NginxConfPath},
		{entryLetsEncrypt, p.LetsEncryptPath},
	} {
		src := filepath.Join(x.tempDir, a.name)
		if !exists(src) {
			continue
		}
		if err := checkSpace(a.target, uint64(treeSize(src)), "restore "+a.name); err != nil {
			return err
		}
	}
	if x.hasDB {
		need := uint64(fileSize(p.DatabasePath)) + uint64(fileSize(x.staged.path))
		if err := checkSpace(filepath.Dir(p.DatabasePath), need, "restore the database"); err != nil {
			return err
		}
	}
	return nil
}

// checkSpace fails when the filesystem holding dir (or its nearest existing
// parent) has less than need bytes available. Unknown free space passes.
func checkSpace(dir string, need uint64, what string) error {
	probe := dir
	for !exists(probe) {
		parent := filepath.Dir(probe)
		if parent == probe {
			return nil
		}
		probe = parent
	}
	free, ok := freeSpace(probe)
	if !ok || free >= need {
		return nil
	}
	return fmt.Errorf("Not enough disk space to %s: %s needed, %s available on %s",
		what, FormatBytes(clampInt64(need)), FormatBytes(clampInt64(free)), probe)
}

func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// apply changes live data, stage by stage.
func (x *run) apply(ctx context.Context) error {
	x.stopContainers(ctx)

	// Point of no return: the durable marker goes down before the first live
	// change, so a crash from here on boots sealed.
	if err := x.stage(StageMarker); err != nil {
		return err
	}
	if err := x.mark(StageDatabase); err != nil {
		return err
	}

	x.out("\n[3/7] Restoring database...\n")
	if x.hasDB {
		if err := x.restoreDatabase(ctx); err != nil {
			return err
		}
	} else {
		x.out("      ! No database in backup\n")
	}

	p := x.r.Paths
	areas := []struct {
		stage                    Stage
		header, name, target     string
		restored, missing        string
		mkdir, reloadAfterChange bool
	}{
		{StageDeployments, "\n[4/7] Restoring deployments...\n", entryDeployments, p.DeploymentsPath,
			"      + Deployments restored\n", "      ! No deployments in backup\n", false, false},
		{StageNginx, "\n[5/8] Restoring nginx configs...\n", entryNginx, p.NginxConfPath,
			"      + Nginx configs restored\n", "      - No nginx configs in backup\n", true, true},
		{StageLetsEncrypt, "\n[6/8] Restoring TLS certificates...\n", entryLetsEncrypt, p.LetsEncryptPath,
			"      + Let's Encrypt certificates restored\n", "      - No certificates in backup\n", true, true},
	}
	for _, a := range areas {
		x.out(a.header)
		src := filepath.Join(x.tempDir, a.name)
		if !exists(src) {
			x.out(a.missing)
			continue
		}
		if err := x.mark(a.stage); err != nil {
			return err
		}
		if err := x.stage(a.stage); err != nil {
			return err
		}
		if a.mkdir {
			if err := os.MkdirAll(a.target, 0o755); err != nil {
				return err
			}
		}
		// Staged inside the target, swapped with rollback of the previous
		// content on failure (fscopy); the bind-mounted target itself is
		// never renamed.
		if err := fscopy.ReplaceDirContents(src, a.target); err != nil {
			return err
		}
		x.out(a.restored)
		x.relink(a.name, a.target)
		if a.reloadAfterChange {
			if err := x.r.Docker.ReloadProxy(ctx); err != nil {
				x.out("      - Nginx reload skipped\n")
			} else {
				x.out("      + Nginx proxy reloaded\n")
			}
		}
	}

	x.out("\n[7/8] Restoring GitHub App key...\n")
	key := filepath.Join(x.tempDir, entryGitHubKey)
	if exists(key) {
		if err := x.mark(StageGitHubKey); err != nil {
			return err
		}
		if err := x.stage(StageGitHubKey); err != nil {
			return err
		}
		if err := copyFileDurable(key, p.GitHubKeyPath, 0o600); err != nil {
			return err
		}
		x.out("      + GitHub key restored\n")
	} else {
		x.out("      - No GitHub key in backup\n")
	}

	if err := os.RemoveAll(x.tempDir); err != nil {
		x.r.Log.Warn("could not remove restore temp directory", "path", x.tempDir, "error", err)
	}
	if err := x.mark(StageMarkRestored); err != nil {
		return err
	}
	if err := x.stage(StageMarkRestored); err != nil {
		return err
	}
	x.markArchive()
	return x.mark(StageReconcile)
}

// stopContainers stops the running user containers so nothing writes into
// the trees being replaced. Failures are warnings, as in 2.1.6.
func (x *run) stopContainers(ctx context.Context) {
	x.out("\n[2/7] Stopping user containers...\n")
	if err := x.stage(StageStopContainers); err != nil {
		x.out("      ! Container stop warning: " + err.Error() + "\n")
		return
	}
	// `docker ps ... || echo ""`: a failing ps means "none running".
	ids, err := x.r.Docker.RunningUserContainers(ctx)
	if err != nil || len(ids) == 0 {
		x.out("      - No user containers running\n")
		return
	}
	if err := x.r.Docker.StopContainers(ctx, ids); err != nil {
		x.out("      ! Container stop warning: " + err.Error() + "\n")
		return
	}
	x.out("      + Stopped user containers\n")
}

// restoreDatabase keeps a rollback copy and swaps the validated database in.
func (x *run) restoreDatabase(ctx context.Context) error {
	if err := x.stage(StagePreRestore); err != nil {
		return err
	}
	if err := createPreRestore(ctx, x.r.DB, x.r.Paths); err != nil {
		return err
	}
	x.out("      + Created backup of current database\n")
	if err := x.stage(StageDatabase); err != nil {
		return err
	}
	if err := restoreInto(ctx, x.r.DB, x.staged.path); err != nil {
		x.out("      ! Database replace failed — live database unchanged\n")
		return err
	}
	x.dbReplaced = true
	x.out("      + Database restored\n")
	if err := x.mark(StageMigrate); err != nil {
		return err
	}
	if err := x.stage(StageMigrate); err != nil {
		return err
	}
	// Already applied to the staged copy; this confirms the live schema.
	return x.r.DB.Ensure(ctx, x.r.Log)
}

// relink recreates the area's symlink entries (certbot live/ links) after
// the swap. Failures keep the files extraction wrote (2.1.6 behaviour).
func (x *run) relink(area, target string) {
	n, err := relinkArea(x.links, area, target)
	if n > 0 {
		x.out(fmt.Sprintf("      + Recreated %d symlink(s)\n", n))
	}
	if err != nil {
		x.out("      ! Could not recreate symlinks: " + err.Error() + "\n")
	}
}

// markArchive is step 8: uploads are renamed to .restored-<date>.zip,
// server-created backups are kept as they are.
func (x *run) markArchive() {
	if x.req.Kind == KindBackup {
		x.out("\n[8/8] Restore archive retained for rollback...\n")
		x.out("      + Kept: " + x.req.Filename + "\n")
		return
	}
	x.out("\n[8/8] Marking file as restored...\n")
	if strings.Contains(x.req.Filename, ".restored-") {
		x.out("      - Already marked as restored\n")
		return
	}
	newName := RestoredFilename(x.req.Filename, x.r.now())
	if err := os.Rename(x.req.ArchivePath, filepath.Join(x.r.Paths.UploadsDir(), newName)); err != nil {
		x.out("      ! Could not mark file: " + err.Error() + "\n")
		return
	}
	x.out("      + Marked as restored: " + newName + "\n")
}

// reconcile is reconcileSystem: start every restored project from its
// runtime compose file and reload nginx-proxy. It reports whether every
// attempted step succeeded.
func (x *run) reconcile(ctx context.Context) bool {
	x.out("\n" + rule + "\n")
	x.out("  RECONCILING SYSTEM STATE\n")
	x.out(rule + "\n\n")
	if err := x.stage(StageReconcile); err != nil {
		x.out("\n[ERROR] Reconcile failed: " + err.Error() + "\n")
		return false
	}
	x.out("[1/3] Reading restored database...\n")
	projects, err := x.r.Store.ListProjects(ctx, store.ProjectFilter{}, store.OrderNone)
	if err != nil {
		x.out("\n[ERROR] Reconcile failed: " + err.Error() + "\n")
		x.r.Log.Error("Reconcile error", "error", err)
		return false
	}
	x.out(fmt.Sprintf("      + Found %d projects in database\n", len(projects)))

	failures := 0
	x.out("\n[2/3] Auto-redeploying projects...\n")
	dpl := x.r.Paths.DeploymentsPath
	for _, p := range projects {
		projectPath := filepath.Join(dpl, p.ID)
		if !exists(projectPath) {
			x.out("      - Skipped " + p.Name + " (No source code found)\n")
			continue
		}
		x.out("      > Redeploying " + p.Name + " (" + p.ID + ")...\n")
		// Only the compose file Docklift generated is ever started. TS fell
		// back to the repository's own docker-compose.yml, which let a
		// repository start any service (privileged, host mounts) with the
		// daemon's root rights; such projects are left for a redeploy.
		composeFile := deploy.RuntimeComposePath(dpl, p.ID)
		if !exists(composeFile) {
			x.out("        ! Skipped (No DockLift runtime compose; deploy manually)\n")
			continue
		}
		project := deploy.ComposeProjectName(p.Name, p.ID)
		// Compose runs in the runtime compose directory, never the checkout,
		// so no file of the repository (.env, overrides) is read.
		if err := x.startProject(ctx, p.ID, composeFile, project, filepath.Dir(composeFile)); err != nil {
			failures++
			x.out("        [ERROR] Failed: " + firstLine(err.Error()) + "\n")
			continue
		}
		x.out("        + Success (" + project + ")\n")
		// 2.1.6 never re-attached the edge proxy here, so restored apps on a
		// new host were unreachable through their domains until redeployed.
		if err := x.r.Docker.ConnectProxy(ctx, p.ID); err != nil {
			x.out("        ! Proxy network attach failed: " + firstLine(err.Error()) + "\n")
		}
	}

	x.out("\n[3/3] Reloading Nginx Proxy...\n")
	if err := x.r.Nginx.Reload(ctx); err != nil {
		failures++
		detail := err.Error()
		var re *nginx.ReloadError
		if errors.As(err, &re) {
			detail = re.Detail
		}
		x.out("      [ERROR] Nginx reload failed: " + firstLine(detail) + "\n")
	} else {
		x.out("      + Nginx configuration reloaded\n")
	}
	if failures > 0 {
		x.out(fmt.Sprintf("\n[ERROR] Reconcile finished with %d failure(s) — restore is incomplete\n", failures))
		return false
	}
	return true
}

// startProject creates the project's external volumes and runs compose up.
func (x *run) startProject(ctx context.Context, projectID, composeFile, project, dir string) error {
	volumes, err := x.r.Store.ListVolumes(ctx, projectID, store.OrderNone)
	if err != nil {
		return err
	}
	for _, v := range volumes {
		// Exists-ok; the outcome is ignored as the TS spawnSync was.
		_ = x.r.Docker.VolumeCreate(ctx, v.Name)
	}
	return x.r.Docker.ComposeUp(ctx, composeFile, project, dir)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// finish applies the commit policy (finishRestoreCommit) and writes the
// closing lines.
func (x *run) finish(ctx context.Context, reconcileOK bool) Outcome {
	if err := x.stage(StageCommit); err != nil {
		return x.fail(ctx, err)
	}
	admins, err := x.r.Store.CountUsersByRole(ctx, "admin")
	if err != nil {
		admins = 0
	}
	d := DecideCommit(CommitInput{ReconcileOK: reconcileOK, AdminCount: admins, DBReplaced: x.dbReplaced, SetupAuth: x.req.SetupAuth})
	if d.Action == ActionAbort {
		x.out("\n[ROLLBACK] Restore not committed: " + d.Reason + "\n")
		if d.RollbackDB && !x.rollback(ctx) {
			x.out("\n[CRITICAL] Database rollback failed — sealed; do not retry until files are repaired.\n")
			x.out("  Manual recovery paths:\n    " + x.r.Paths.DatabasePath + "\n    " + x.r.Paths.PreRestorePath() + "\n")
			x.seal(d.Reason)
			x.closeCritical()
			return OutcomeCritical
		}
		if x.req.SetupAuth {
			x.out("  [security] Setup credentials retained for retry\n")
		}
		x.unmark()
		x.out("\n" + rule + "\n")
		x.out("  [ERROR] RESTORE ABORTED\n")
		x.out(rule + "\n")
		return OutcomeAborted
	}

	// Commit point: from here a crash boots into the restored data.
	checkpoint(ctx, x.r.DB)
	x.unmark()
	if d.ConsumeSetup {
		x.r.Setup.ConsumeSetupRestoreSecrets()
		x.out("\n  [security] Setup token + bootstrap secret consumed\n")
	}
	x.out("\n" + rule + "\n")
	if reconcileOK {
		x.out("  RESTORE COMPLETE\n")
	} else {
		x.out("  [ERROR] RESTORE INCOMPLETE — data restored but some projects failed to start\n")
	}
	x.out(rule + "\n")
	x.out("\n  [!] Restarting backend service to apply changes...\n")
	if x.req.Kind != KindBackup {
		x.out("\n  The uploaded file has been kept. You can delete it manually from Settings.\n")
	}
	return OutcomeCommitted
}

// fail handles an error after the point of no return: the database is
// rolled back when it was replaced; a failed rollback seals the instance.
func (x *run) fail(ctx context.Context, err error) Outcome {
	x.r.Log.Error("Restore error", "error", err)
	if x.dbReplaced {
		x.out("\n[ROLLBACK] Attempting database rollback after failed restore...\n")
		if !x.rollback(ctx) {
			x.out("\n[CRITICAL] Rollback failed — sealed; do not retry\n")
			x.out("  Paths: " + x.r.Paths.DatabasePath + " | " + x.r.Paths.PreRestorePath() + "\n")
			x.seal(err.Error())
			// 2.1.6 wrote no [ERROR] line here, so the dashboard reported success.
			x.out("\n[ERROR] Restore failed: " + err.Error() + "\n")
			return OutcomeCritical
		}
	}
	x.unmark()
	x.out("\n[ERROR] Restore failed: " + err.Error() + "\n")
	return OutcomeFailed
}

// rollback is rollbackDatabaseFromPreRestore: put the .pre-restore copy back
// (one transaction, like the swap). It never claims success it did not have.
func (x *run) rollback(ctx context.Context) bool {
	live, pre := x.r.Paths.DatabasePath, x.r.Paths.PreRestorePath()
	if !preRestoreExists(x.r.Paths) {
		x.out("      ! No pre-restore database at " + pre + "\n")
		return false
	}
	err := x.stage(StageRollback)
	if err == nil {
		err = restoreInto(ctx, x.r.DB, pre)
	}
	if err != nil {
		x.out("      ! DB rollback FAILED: " + err.Error() + "\n")
		x.out("      ! CRITICAL: preserve both files for manual recovery:\n")
		x.out("          live: " + live + "\n")
		x.out("          pre-restore: " + pre + "\n")
		return false
	}
	x.dbReplaced = false
	x.out("      + Rolled database back from .pre-restore\n")
	x.out("      + Live DB: " + live + "\n")
	x.out("      + Pre-restore copy retained: " + pre + "\n")
	return true
}

// seal writes the restore-critical seal (maintenance and lock stay on),
// flushes it and only then drops the in-progress marker. If the seal cannot
// be written the marker stays, so the next boot seals instead.
func (x *run) seal(detail string) {
	st := x.r.State
	if err := st.EnterCritical(detail, x.r.Paths.DatabasePath); err != nil {
		x.r.Log.Error("[CRITICAL] could not write the restore-critical seal", "error", err)
		st.Enter("CRITICAL restore recovery required — " + detail)
		st.ForceRestoreLock("restore-critical")
		return
	}
	if err := syncFile(st.CriticalMarkerPath()); err != nil {
		x.r.Log.Warn("could not flush the restore-critical seal", "error", err)
	}
	syncDir(x.r.Paths.DataPath)
	x.unmark()
}

func (x *run) closeCritical() {
	x.out("\n" + rule + "\n")
	x.out("  [CRITICAL] RESTORE FAILED — MANUAL RECOVERY REQUIRED\n")
	x.out("  Seal: data/.restore-critical (lock held; restores blocked across restarts)\n")
	x.out("  [ERROR] Restore could not be rolled back — manual recovery required\n")
	x.out(rule + "\n")
}
