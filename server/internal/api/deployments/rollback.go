package deployments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/gitops"
	"docklift/internal/store"
)

// Rollback restores the images of a previous successful deployment without
// rebuilding ("Restore previous"): it resets the checkout to the target's
// commit, rewrites the runtime compose with the stored image tags and runs
// compose up. Only a success that is not the current one, has stored tags
// and whose images all still exist locally can be restored. The caller has
// already verified the step-up password.
func (s *Service) Rollback(ctx context.Context, projectID, targetID string, out io.Writer) (*Run, error) {
	unlock := s.startMu.lock(projectID)
	defer unlock()
	p, err := s.getProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if strVal(p.ProjectType) == "database" {
		return nil, reqErr(http.StatusBadRequest, "Rollback is not supported for managed databases")
	}
	t := s.rt.Tracker
	if t.IsCancelling(projectID) || t.IsDeploying(projectID) {
		return nil, reqErr(http.StatusConflict, "A deployment or cancel is already in progress")
	}
	if busy, err := s.inProgress(ctx, projectID); err != nil {
		return nil, err
	} else if busy {
		return nil, ErrBusy
	}

	success := deploy.StatusSuccess
	latestID := ""
	latest, err := s.store.FindDeployment(ctx, store.DeploymentFilter{ProjectID: &projectID, Status: &success}, store.OrderCreatedDesc)
	switch {
	case err == nil:
		latestID = latest.ID
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	target, err := s.store.FindDeployment(ctx, store.DeploymentFilter{ID: &targetID, ProjectID: &projectID}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		return nil, reqErr(http.StatusNotFound, "Target successful deployment not found")
	}
	if err != nil {
		return nil, err
	}
	tags, _ := deploy.ParseImageTags(target.ImageTags)
	if g := deploy.RollbackTargetGuard(targetID, latestID, strVal(target.Status), tags); !g.OK {
		return nil, reqErr(g.Status, g.Error)
	}
	projectPath := s.rt.ProjectPath(projectID)
	if !exists(projectPath) {
		return nil, ErrFilesNotFound
	}
	if err := s.docker.Ping(ctx); dockerx.IsUnavailable(err) {
		return nil, ErrDockerUnavailable
	}

	msg := "Rollback to " + targetID[:min(8, len(targetID))]
	if strVal(target.CommitMessage) != "" {
		msg = "Rollback: " + *target.CommitMessage
	}
	dctx := s.dbCtx()
	row, err := s.store.CreateDeployment(dctx, store.DeploymentCreate{
		ProjectID: projectID, Status: store.Ptr(deploy.StatusInProgress), Trigger: store.Ptr("rollback"),
		CommitMessage: &msg, CommitSHA: target.CommitSHA, ImageTags: tags.JSON(), Logs: store.Ptr("⏪ Starting rollback...\n"),
	})
	if err != nil {
		return nil, err
	}
	release, ok := t.Acquire(projectID, row.ID)
	if !ok {
		if err := s.finishRow(row.ID, deploy.StatusFailed, "❌ Another deployment is already running\n"); err != nil {
			return nil, err
		}
		return nil, ErrBusy
	}
	// The lock is held before the image checks so a system purge cannot
	// remove an image mid-verify.
	for _, svc := range tags.Services() {
		tag, _ := tags.Get(svc)
		if !s.docker.ImageExists(ctx, tag) {
			err := s.finishRow(row.ID, deploy.StatusFailed, fmt.Sprintf("❌ Image missing for %s: %s\n", svc, tag))
			release()
			if err != nil {
				return nil, err
			}
			return nil, reqErr(http.StatusConflict, fmt.Sprintf("Image missing for %s: %s. Redeploy from git instead.", svc, tag))
		}
	}

	l := newDeployLog(out)
	run := newRun(row.ID)
	s.goRun(run, l, func() {
		defer func() {
			t.ClearProcess(projectID, row.ID)
			release()
		}()
		s.rollback(p, row.ID, target, tags, l)
	})
	return run, nil
}

func (s *Service) rollback(p *store.Project, id string, target *store.Deployment, tags deploy.ImageTags, l *deployLog) {
	pid := p.ID
	ctx := s.dbCtx()
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	projectPath := s.rt.ProjectPath(pid)
	targetSHA := strVal(target.CommitSHA)
	logf("⏪ Restoring deployment %s...\n", target.ID[:min(8, len(target.ID))])

	previousHead := ""
	if targetSHA != "" && gitops.IsRepository(projectPath) {
		previousHead, _ = s.git.CommitSHA(ctx, projectPath)
	}
	didReset, composeOK := false, false

	err := func() error {
		if targetSHA != "" && gitops.IsRepository(projectPath) {
			logf("📌 Resetting git to %s...\n", targetSHA[:min(12, len(targetSHA))])
			if err := s.git.ResetToCommit(ctx, projectPath, targetSHA); err != nil {
				return err
			}
			didReset = true
			l.line("   ✅ Git reset complete\n")
		} else {
			l.line("⚠️ No commit SHA stored — leaving working tree unchanged\n")
		}
		services, err := s.store.ListServices(ctx, store.ServiceFilter{ProjectID: &pid}, store.OrderCreatedAsc)
		if err != nil {
			return err
		}
		volumes, err := s.store.ListVolumes(ctx, pid, store.OrderCreatedAsc)
		if err != nil {
			return err
		}
		envRows, err := s.store.ListEnvVariables(ctx, store.EnvFilter{ProjectID: &pid}, store.OrderNone)
		if err != nil {
			return err
		}
		var runtime []deploy.RuntimeService
		for _, svc := range services {
			image, ok := tags.Get(svc.Name)
			if !ok {
				return fmt.Errorf("No image tag stored for service %s", svc.Name)
			}
			if strVal(svc.ContainerName) == "" {
				return fmt.Errorf("Service %s has no container name", svc.Name)
			}
			port := 3000
			if svc.InternalPort != nil && *svc.InternalPort != 0 {
				port = *svc.InternalPort
			}
			runtime = append(runtime, deploy.RuntimeService{
				Name: svc.Name, Image: image, InternalPort: port, Port: svc.Port, ContainerName: *svc.ContainerName,
				Volumes: runtimeVolumes(volumes, svc.Name),
			})
		}
		if len(runtime) == 0 {
			return errors.New("No services to restore")
		}
		composePath := s.rt.ComposePath(pid)
		l.line("📝 Rewriting runtime compose with previous images...\n")
		if err := deploy.WriteRuntimeCompose(composePath, runtime, deploy.EnvVarsFromStore(envRows),
			deploy.RuntimeComposeOptions{ProjectID: pid, PublishHostPort: p.PublishHostPort}); err != nil {
			return err
		}
		l.line("🚀 Starting containers from previous images...\n")
		var res struct{ code int }
		err = s.process(pid, id, func(pctx context.Context) error {
			r, err := s.docker.Compose(pctx, dockerx.ComposeCmd{
				Project: deploy.ComposeProjectName(p.Name, pid), Files: []string{composePath}, Dir: projectPath,
				Args: []string{"up", "-d", "--remove-orphans"}, Output: l,
			})
			res.code = r.ExitCode
			return err
		})
		l.final()
		if err != nil {
			return err
		}
		if res.code != 0 {
			return fmt.Errorf("compose up exited %d", res.code)
		}
		composeOK = true
		if !s.rt.Tracker.Owns(pid, id) {
			return errors.New("Rollback cancelled")
		}
		if err := s.rt.ConnectProxyToProjectNetwork(s.bg, pid); err != nil {
			logf("⚠️ Proxy attach warning: %s\n", err.Error())
		} else {
			l.line("🔗 Edge proxy attached\n")
		}
		if _, _, err := s.rt.ReapplyDatabaseLinksForApp(s.bg, pid); err != nil {
			logf("⚠️ Link re-apply warning: %s\n", err.Error())
		}
		if _, err := s.rt.SyncProjectStatus(s.bg, pid); err != nil {
			return err
		}
		l.line("\n✅ ROLLBACK SUCCESSFUL\n")
		return s.finishRollback(id, deploy.StatusSuccess, l.final(), &tags, target.CommitSHA)
	}()
	if err == nil {
		return
	}
	if composeOK {
		logf("\n⚠️ Post-compose warning: %s\n", err.Error())
		l.line("⚠️ Containers were updated; leaving git at restore target despite post-compose error\n")
		l.line("\n✅ ROLLBACK SUCCESSFUL\n")
		_ = s.finishRollback(id, deploy.StatusSuccess, l.final(), &tags, target.CommitSHA)
		return
	}
	logf("\n[ERROR] Rollback failed: %s\n", err.Error())
	if didReset && previousHead != "" && previousHead != targetSHA {
		logf("📌 Restoring git HEAD to %s...\n", previousHead[:min(12, len(previousHead))])
		if gerr := s.git.ResetToCommit(ctx, projectPath, previousHead); gerr != nil {
			logf("   ! Failed to restore git HEAD: %s\n", gerr.Error())
		} else {
			l.line("   ✅ Git HEAD restored after failed rollback\n")
		}
	}
	_ = s.finishRollback(id, deploy.StatusFailed, l.final(), nil, nil)
}

// finishRollback writes the rollback row's final state; a success also
// stores the restored tags and commit.
func (s *Service) finishRollback(id, status, logs string, tags *deploy.ImageTags, sha *string) error {
	now := db.Now()
	upd := store.DeploymentUpdate{Status: store.Some(&status), Logs: store.Some(&logs), FinishedAt: store.Some(&now)}
	if tags != nil {
		upd.ImageTags = store.Some(tags.JSON())
		upd.CommitSHA = store.Some(sha)
	}
	_, err := s.store.UpdateDeployment(s.dbCtx(), id, upd)
	return err
}
