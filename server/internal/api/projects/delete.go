package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"docklift/internal/app"
	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/store"
)

// ErrNotFound is returned by DeleteProject for a project that does not exist.
var ErrNotFound = errors.New("Project not found")

// ErrDockerUnavailable is returned by DeleteProject when the Docker daemon
// cannot be reached, before anything is torn down.
var ErrDockerUnavailable = errors.New("Docker is not available")

// ConflictError is a delete refused because the project is busy or Docker
// could not tear it down; nothing has been deleted from the database and
// Message is the response text.
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

const (
	msgDeployInProgress = "Cannot delete project while a deployment is in progress"
	// deleteLockPrefix marks a project lock held by a delete.
	deleteLockPrefix = "delete:"
	// composeDownTimeout is the per-alias compose down limit.
	composeDownTimeout = 60 * time.Second
)

var noSuchVolumeRe = regexp.MustCompile(`(?i)no such volume`)

// Service owns the project operations other route groups call.
type Service struct {
	d *app.Deps
}

// NewService returns the project service over d.
func NewService(d *app.Deps) *Service { return &Service{d: d} }

// DeleteProject tears a project down and deletes it (DELETE
// /api/projects/:id): linked databases and the edge proxy are detached,
// every compose project name the project used is brought down with its
// images and verified gone, the project network and persistent volumes are
// removed, its checkout and runtime state are deleted, managed-database
// links are cleaned (for a database project), nginx confs of its services
// are removed, and finally the row is deleted (cascading to deployments,
// services, env variables, ports, volumes and links).
//
// The project's deploy lock is held for the whole teardown, so no deploy
// can start while it runs; a deploy that holds it (or an in_progress
// deployment row) makes the delete fail with a *ConflictError. A teardown
// failure also returns a *ConflictError after restoring the proxy and
// database attachments, and leaves every row in place. Other errors:
// ErrNotFound, ErrDockerUnavailable, or a wrapped internal error.
func (s *Service) DeleteProject(ctx context.Context, projectID string) error {
	rt := s.d.Deploy
	release, ok := rt.Tracker.Acquire(projectID, deleteLockPrefix+db.NewID())
	if !ok {
		return &ConflictError{Message: msgDeployInProgress}
	}
	defer release()
	inProgress := deploy.StatusInProgress
	active, err := s.d.Store.CountDeployments(ctx, store.DeploymentFilter{ProjectID: &projectID, Status: &inProgress})
	if err != nil {
		return err
	}
	if active > 0 {
		return &ConflictError{Message: msgDeployInProgress}
	}
	project, err := s.d.Store.GetProject(ctx, projectID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	volumes, err := s.d.Store.ListVolumes(ctx, projectID, store.OrderNone)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err = s.d.DockerX.Ping(pingCtx)
	cancel()
	if err != nil && dockerx.IsUnavailable(err) {
		return ErrDockerUnavailable
	}

	isDatabase := project.ProjectType != nil && *project.ProjectType == "database"
	// Linked database containers hold the app network open.
	if !isDatabase {
		if err := rt.DisconnectLinkedDatabasesFromApp(ctx, projectID); err != nil {
			return err
		}
	}
	// The proxy must leave the project network before compose down can remove it.
	rt.DisconnectProxyFromProjectNetwork(ctx, projectID)
	if err := s.composeDown(ctx, project, isDatabase); err != nil {
		return err
	}
	rt.TeardownProjectNetwork(ctx, projectID)

	for _, v := range volumes {
		res, err := s.d.DockerX.VolumeRemove(ctx, v.Name)
		var exit *execx.ExitError
		if err != nil && (!errors.As(err, &exit) || !noSuchVolumeRe.Match(append(res.Stdout, res.Stderr...))) {
			return &ConflictError{Message: fmt.Sprintf(`Could not remove persistent volume "%s". Detach any containers using it, then retry project deletion.`, v.DisplayName)}
		}
		if err := s.d.Store.DeleteVolume(ctx, v.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}

	for _, dir := range []string{rt.ProjectPath(projectID), rt.StatePath(projectID)} {
		if err := os.RemoveAll(dir); err != nil {
			s.d.Log.Warn("File cleanup warned for project "+projectID, "error", err)
		}
	}
	// Only after Docker and volume teardown succeeded: strip peer apps' env
	// and links of a managed database.
	if isDatabase {
		if err := rt.CleanupLinksForDatabaseProject(ctx, projectID); err != nil {
			return err
		}
	}
	services, err := s.d.Store.ListServices(ctx, store.ServiceFilter{ProjectID: &projectID}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, svc := range services {
		s.d.Edge.Nginx.CleanupServiceDomain(ctx, svc.ID)
	}
	if err := s.d.Store.DeleteProject(ctx, projectID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// composeDown brings down every compose project name the project may have
// used. The current name uses the runtime compose file when it exists.
// Each must be verified gone; on the first failure the proxy and linked
// databases are re-attached (best-effort) and a *ConflictError returned.
//
// Compose runs in rt.ComposeWorkDir, never in the checkout (TS ran it there,
// so a legacy alias without -f loaded the repository's own compose file and
// .env, and `--rmi all` removed every image that file named). Images are
// removed afterwards by Docklift tag only: docklift-<project8>-* tags, never
// upstream images such as a managed database's postgres image.
//
// --volumes removes the anonymous volumes of the project's containers (an
// image VOLUME no Docklift volume is mounted on, such as mongo's
// /data/configdb). TS left them behind on every delete (found on a real
// daemon); the persistent volumes are external in the runtime compose, so
// compose never removes them — they are removed by name after this.
func (s *Service) composeDown(ctx context.Context, project *store.Project, isDatabase bool) error {
	rt := s.d.Deploy
	dir, err := rt.ComposeWorkDir()
	if err != nil {
		return err
	}
	runtimeCompose := rt.ComposePath(project.ID)
	_, statErr := os.Stat(runtimeCompose)
	primary := deploy.ComposeProjectName(project.Name, project.ID)
	for _, alias := range deploy.ComposeProjectAliases(project.Name, project.ID) {
		cmd := dockerx.ComposeCmd{Project: alias, Dir: dir, Args: []string{"down", "--remove-orphans", "--volumes"}, Timeout: composeDownTimeout}
		if alias == primary && statErr == nil {
			cmd.Files = []string{runtimeCompose}
		}
		down := s.d.DockerX.ComposeDown(ctx, cmd)
		if down.OK {
			continue
		}
		if err := rt.ConnectProxyToProjectNetwork(ctx, project.ID); err != nil {
			s.d.Log.Warn("restore proxy attachment after failed teardown", "project", project.ID, "error", err)
		}
		if !isDatabase {
			if _, _, err := rt.ReapplyDatabaseLinksForApp(ctx, project.ID); err != nil {
				s.d.Log.Warn("restore database links after failed teardown", "project", project.ID, "error", err)
			}
		}
		return &ConflictError{Message: fmt.Sprintf(`Cannot delete project: Docker teardown failed for "%s" (owned containers/networks still present). %s`, alias, down.Detail())}
	}
	res, err := rt.RemoveUnusedDockliftImages(ctx, project.ID, nil, nil)
	if err != nil || len(res.Errors) > 0 {
		s.d.Log.Warn("Image cleanup warned for project "+project.ID, "error", err, "failures", res.Errors)
	}
	return nil
}
