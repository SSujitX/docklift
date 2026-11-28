package deployments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// Refusals of Stop, Restart, Cancel and Rollback.
var (
	ErrStopWhileDeploying    = reqErr(http.StatusConflict, "Cannot stop while a deployment is running; cancel it first")
	ErrRestartWhileDeploying = reqErr(http.StatusConflict, "Cannot restart while a deployment is running")
	ErrCancelInProgress      = reqErr(http.StatusConflict, "A cancel is already in progress for this project")
	// ErrNotDeployed answers stop/restart before the first deploy (2.x
	// routes answered a 200 text error and a 500; deliberate fix 8).
	ErrNotDeployed = reqErr(http.StatusBadRequest, deploy.ErrRuntimeMissing.Error())
	// ErrDockerUnavailable is deliberate fix 9.
	ErrDockerUnavailable = reqErr(http.StatusServiceUnavailable, "Docker is not available")
)

// goRun runs body in the background as run; a panic is logged and reported
// to the client, and the row (if any) is forced to a terminal status.
func (s *Service) goRun(run *Run, l *deployLog, body func()) {
	s.running.Add(1)
	go func() {
		defer s.running.Done()
		defer s.finish(run)
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("deployment operation panicked", "deployment", run.DeploymentID, "panic", fmt.Sprint(rec))
				l.toClient("\n❌ Error: internal error\n")
			}
			if run.DeploymentID != "" {
				s.ensureTerminal(run.DeploymentID, l)
			}
		}()
		body()
	}()
}

func (s *Service) getProject(ctx context.Context, projectID string) (*store.Project, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrProjectNotFound
	}
	return p, err
}

// finishRow writes the final status and logs of an operation row.
func (s *Service) finishRow(id, status string, logs string) error {
	now := db.Now()
	_, err := s.store.UpdateDeployment(s.dbCtx(), id, store.DeploymentUpdate{
		Status: store.Some(&status), Logs: store.Some(&logs), FinishedAt: store.Some(&now),
	})
	return err
}

func statusOf(ok bool) string {
	if ok {
		return deploy.StatusSuccess
	}
	return deploy.StatusFailed
}

// Stop tears the project down (compose down of every alias, verified by
// label probe), recording a "stop" row, and streams progress to out.
func (s *Service) Stop(ctx context.Context, projectID string, out io.Writer) (*Run, error) {
	unlock := s.startMu.lock(projectID)
	defer unlock()
	if s.rt.Tracker.IsDeploying(projectID) {
		return nil, ErrStopWhileDeploying
	}
	p, err := s.getProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.rt.ComposeFiles(projectID); err != nil {
		return nil, ErrNotDeployed
	}
	row, err := s.store.CreateDeployment(s.dbCtx(), store.DeploymentCreate{
		ProjectID: projectID, Status: store.Ptr(deploy.StatusInProgress), Trigger: store.Ptr("stop"),
		Logs: store.Ptr("🛑 Stopping project...\n"),
	})
	if err != nil {
		return nil, err
	}
	l := newDeployLog(out)
	run := newRun(row.ID)
	s.goRun(run, l, func() {
		if err := s.stop(p, row.ID, l); err != nil {
			s.log.Error("stop failed", "project", projectID, "error", err)
			l.line("\n❌ Error: " + err.Error() + "\n")
		}
	})
	return run, nil
}

func (s *Service) stop(p *store.Project, id string, l *deployLog) error {
	pid := p.ID
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	logf("\n%s\n🛑 STOPPING PROJECT\n📅 %s\n%s\n\n", bar50, isoNow(), bar50)

	if strVal(p.ProjectType) != "database" {
		l.line("🔌 Detaching linked databases from project network...\n")
		if err := s.rt.DisconnectLinkedDatabasesFromApp(s.bg, pid); err != nil {
			logf("⚠️ Link detach warning: %s\n", err.Error())
		}
	}
	l.line("🔌 Disconnecting edge proxy from project network...\n")
	s.rt.DisconnectProxyFromProjectNetwork(s.bg, pid)

	ok, err := s.teardown(p, l, false, true)
	if err != nil {
		return err
	}
	if !ok {
		s.restoreAfterFailedTeardown(p, l, "stop")
	}
	if ok {
		logf("\n%s\n✅ STOP SUCCESSFUL!\n%s\n", bar50, bar50)
	} else {
		logf("\n%s\n❌ STOP FAILED\n%s\n", bar50, bar50)
	}
	if err := s.finishRow(id, statusOf(ok), l.final()); err != nil {
		return err
	}
	if !ok {
		return nil
	}
	ctx := s.dbCtx()
	if err := s.rt.SetProjectAndServicesStatus(ctx, pid, deploy.StatusStopped); err != nil {
		return err
	}
	services, err := s.store.ListServices(ctx, store.ServiceFilter{ProjectID: &pid}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if err := s.edge.UpdateServiceDomain(s.bg, *svc, nginx.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// teardown runs compose down for every alias of the project and verifies
// each with the exact-label probe, retrying once after detaching the proxy.
// withFiles uses the runtime compose file for the current alias (stop
// requires it; cancel only when it exists); orphans adds --remove-orphans.
func (s *Service) teardown(p *store.Project, l *deployLog, orphans, retryOutput bool) (bool, error) {
	pid := p.ID
	current := deploy.ComposeProjectName(p.Name, pid)
	files, filesErr := s.rt.ComposeFiles(pid)
	// Never the checkout: compose without -f would load the repository's
	// own compose file and .env.
	dir, err := s.rt.ComposeWorkDir()
	if err != nil {
		return false, err
	}
	args := []string{"down"}
	if orphans {
		args = append(args, "--remove-orphans")
	}
	ok := true
	for _, alias := range deploy.ComposeProjectAliases(p.Name, pid) {
		c := dockerx.ComposeCmd{Project: alias, Dir: dir, Args: args}
		if alias == current {
			if filesErr != nil && !orphans {
				return false, filesErr
			}
			c.Files = files
		}
		l.line("🛑 compose down -p " + alias + "\n")
		down := s.docker.ComposeDown(s.bg, c)
		l.line(down.Stdout)
		l.line(down.Stderr)
		if !down.OK {
			s.rt.DisconnectProxyFromProjectNetwork(s.bg, pid)
			down = s.docker.ComposeDown(s.bg, c)
			if retryOutput {
				l.line(down.Stdout)
				l.line(down.Stderr)
			}
		}
		if !down.OK {
			ok = false
			if orphans {
				l.line("❌ Cancel teardown incomplete for \"" + alias + "\"\n")
			} else {
				l.line("❌ Teardown incomplete for \"" + alias + "\" — owned containers/networks still present\n")
			}
		} else {
			l.line("✅ Teardown verified for \"" + alias + "\"\n")
		}
	}
	return ok, nil
}

// restoreAfterFailedTeardown reattaches the proxy and linked databases when
// containers may still run.
func (s *Service) restoreAfterFailedTeardown(p *store.Project, l *deployLog, what string) {
	pid := p.ID
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	if err := s.rt.ConnectProxyToProjectNetwork(s.bg, pid); err != nil {
		logf("⚠️ Could not reconnect edge proxy: %s\n", err.Error())
	} else if what == "stop" {
		l.line("🔗 Reconnected edge proxy after failed stop (app may still be running)\n")
	} else {
		l.line("🔗 Reconnected edge proxy after failed cancel teardown\n")
	}
	if strVal(p.ProjectType) == "database" {
		return
	}
	okN, failed, err := s.rt.ReapplyDatabaseLinksForApp(s.bg, pid)
	switch {
	case err != nil:
		logf("⚠️ Could not re-attach linked databases: %s\n", err.Error())
	case failed == 0:
		logf("🔗 Re-attached %d linked database(s) after failed %s\n", okN, what)
	default:
		logf("⚠️ Re-attached %d linked database(s); %d failed after %s\n", okN, failed, what)
	}
}

func isoNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// Restart runs `docker compose restart`, recording a "restart" row.
func (s *Service) Restart(ctx context.Context, projectID string, out io.Writer) (*Run, error) {
	unlock := s.startMu.lock(projectID)
	defer unlock()
	if s.rt.Tracker.IsDeploying(projectID) {
		return nil, ErrRestartWhileDeploying
	}
	p, err := s.getProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	files, err := s.rt.ComposeFiles(projectID)
	if err != nil {
		return nil, ErrNotDeployed
	}
	dir, err := s.rt.ComposeWorkDir()
	if err != nil {
		return nil, err
	}
	dctx := s.dbCtx()
	row, err := s.store.CreateDeployment(dctx, store.DeploymentCreate{
		ProjectID: projectID, Status: store.Ptr(deploy.StatusInProgress), Trigger: store.Ptr("restart"),
		Logs: store.Ptr("🔄 Starting restart...\n"),
	})
	if err != nil {
		return nil, err
	}
	if err := s.rt.SetProjectAndServicesStatus(dctx, projectID, deploy.StatusBuilding); err != nil {
		s.restartFailed(projectID)
		return nil, err
	}
	l := newDeployLog(out)
	run := newRun(row.ID)
	s.goRun(run, l, func() {
		logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
		logf("\n%s\n🔄 RESTARTING PROJECT\n📅 %s\n%s\n\n", bar50, isoNow(), bar50)
		res, err := s.docker.Compose(s.bg, dockerx.ComposeCmd{
			Project: deploy.ComposeProjectName(p.Name, projectID), Files: files, Dir: dir,
			Args: []string{"restart"}, Output: l,
		})
		if err != nil {
			l.line("\n❌ Docker execution error: " + err.Error() + "\n")
			_ = s.finishRow(row.ID, deploy.StatusFailed, l.final())
		} else {
			ok := res.ExitCode == 0
			if ok {
				logf("\n%s\n✅ RESTART SUCCESSFUL!\n%s\n", bar50, bar50)
			} else {
				logf("\n%s\n❌ RESTART FAILED (code %d)\n%s\n", bar50, res.ExitCode, bar50)
			}
			if err := s.finishRow(row.ID, statusOf(ok), l.final()); err != nil {
				s.log.Error("restart failed", "project", projectID, "error", err)
				s.restartFailed(projectID)
				l.toClient("\n❌ Error: " + err.Error() + "\n")
				return
			}
		}
		if _, err := s.rt.SyncProjectStatus(s.bg, projectID); err != nil {
			s.log.Error("restart status sync failed", "project", projectID, "error", err)
		}
	})
	return run, nil
}

// restartFailed is the catch block of the restart route: clear the stuck
// "building" status and fail in-progress restart rows.
func (s *Service) restartFailed(projectID string) {
	ctx := s.dbCtx()
	_ = s.rt.SetProjectAndServicesStatus(ctx, projectID, deploy.StatusError)
	inProgress, trigger := deploy.StatusInProgress, "restart"
	now := db.Now()
	_, _ = s.store.UpdateDeployments(ctx, store.DeploymentFilter{ProjectID: &projectID, Status: &inProgress, Trigger: &trigger},
		store.DeploymentUpdate{Status: store.Some(store.Ptr(deploy.StatusFailed)), FinishedAt: store.Some(&now)})
}

// Cancel stops an in-flight deploy (killing its command) and tears the
// project down so it can start fresh. It holds the project lock for the
// whole teardown; an idle cancel leaves the deployment history unchanged.
func (s *Service) Cancel(ctx context.Context, projectID string, out io.Writer) (*Run, error) {
	unlock := s.startMu.lock(projectID)
	defer unlock()
	p, err := s.getProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	t := s.rt.Tracker
	if t.IsCancelling(projectID) {
		return nil, ErrCancelInProgress
	}
	hasRuntime := exists(s.rt.ComposePath(projectID))
	stolen, end := t.BeginCancel(projectID)
	if stolen != "" {
		t.MarkCancelled(stolen)
	}
	l := newDeployLog(out)
	run := newRun("")
	s.goRun(run, l, func() {
		defer func() {
			// Kill again and give the command a moment to abort before
			// unlocking, so a redeploy cannot overlap an in-flight up.
			t.KillActive(projectID, 500*time.Millisecond)
			end()
		}()
		if err := s.cancel(p, hasRuntime, l); err != nil {
			s.log.Error("cancel failed", "project", projectID, "error", err)
			l.line("\n❌ Cancel failed\n")
		}
	})
	return run, nil
}

func (s *Service) cancel(p *store.Project, hasRuntime bool, l *deployLog) error {
	pid := p.ID
	t := s.rt.Tracker
	ctx := s.dbCtx()
	l.line("❌ Cancelling — tearing down so you can start fresh...\n")
	_, active := t.ActiveDeployment(pid)
	if active {
		l.line("🛑 Stopping in-flight docker compose process...\n")
		t.KillActive(pid, 0)
	} else {
		l.line("ℹ️ No tracked build process — tearing down containers if present...\n")
	}

	status := deploy.StatusInProgress
	inProgress, err := s.store.ListDeployments(ctx, store.DeploymentFilter{ProjectID: &pid, Status: &status}, store.OrderNone, store.Page{})
	if err != nil {
		return err
	}
	for _, d := range inProgress {
		t.MarkCancelled(d.ID)
	}
	now := db.Now()
	if _, err := s.store.UpdateDeployments(ctx, store.DeploymentFilter{ProjectID: &pid, Status: &status},
		store.DeploymentUpdate{Status: store.Some(store.Ptr(deploy.StatusCancelled)), FinishedAt: store.Some(&now)}); err != nil {
		return err
	}
	if len(inProgress) == 0 && !active {
		l.line("ℹ️ No active deploy row — tearing down containers; history left unchanged.\n")
	}

	if strVal(p.ProjectType) != "database" {
		l.line("🔌 Detaching linked databases from project network...\n")
		if err := s.rt.DisconnectLinkedDatabasesFromApp(s.bg, pid); err != nil {
			l.line("⚠️ Link detach warning: " + err.Error() + "\n")
		}
	}
	s.rt.DisconnectProxyFromProjectNetwork(s.bg, pid)

	if exists(s.rt.ProjectPath(pid)) && hasRuntime {
		files, err := s.rt.ComposeFiles(pid)
		dir, dirErr := s.rt.ComposeWorkDir()
		if err == nil && dirErr == nil {
			_, _ = s.docker.Compose(s.bg, dockerx.ComposeCmd{
				Project: deploy.ComposeProjectName(p.Name, pid), Files: files, Dir: dir,
				Args: []string{"kill"}, Timeout: 30 * time.Second,
			})
		}
	}
	for _, d := range inProgress {
		t.ClearProcess(pid, d.ID)
	}

	ok, err := s.teardown(p, l, true, false)
	if err != nil {
		return err
	}
	if !ok {
		s.restoreAfterFailedTeardown(p, l, "cancel")
		l.line("❌ Compose teardown failed; project status unchanged\n")
		return nil
	}
	if err := s.rt.SetProjectAndServicesStatus(ctx, pid, deploy.StatusStopped); err != nil {
		return err
	}
	services, err := s.store.ListServices(ctx, store.ServiceFilter{ProjectID: &pid}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if err := s.edge.UpdateServiceDomain(s.bg, *svc, nginx.UpdateOptions{}); err != nil {
			s.log.Error("Cancel nginx cleanup failed", "service", svc.ID, "error", err)
		}
	}
	l.line("✅ Cancelled — containers down. Ready for a fresh deploy.\n")
	return nil
}
