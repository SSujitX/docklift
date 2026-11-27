package deploy

import (
	"context"
	"errors"
	"fmt"

	"docklift/internal/db"
	"docklift/internal/dockerx"
	"docklift/internal/store"
)

// Deployment row statuses.
const (
	StatusInProgress = "in_progress"
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

// Project and service statuses.
const (
	StatusBuilding = "building"
	StatusRunning  = "running"
	StatusStopped  = "stopped"
	StatusError    = "error"
	StatusDegraded = "degraded"
	StatusPending  = "pending"
)

// interruptedMessage is appended to deployments cut off by a restart.
const interruptedMessage = "interrupted by restart"

// DockerStatusToServiceStatus maps a container status to a service status:
// running -> running, dead -> error, everything else (incl. not_found) ->
// stopped.
func DockerStatusToServiceStatus(cs dockerx.ContainerStatus) string {
	switch {
	case cs.Running:
		return StatusRunning
	case cs.Status == dockerx.StatusNotFound:
		return StatusStopped
	case cs.Status == "dead":
		return StatusError
	}
	return StatusStopped
}

// AggregateProjectStatus folds service statuses into the project status:
// none -> stopped, all running -> running, any error -> error, some running
// -> degraded (never a full "running"), else stopped.
func AggregateProjectStatus(statuses []string) string {
	if len(statuses) == 0 {
		return StatusStopped
	}
	all, anyErr, anyRunning := true, false, false
	for _, s := range statuses {
		all = all && s == StatusRunning
		anyErr = anyErr || s == StatusError
		anyRunning = anyRunning || s == StatusRunning
	}
	switch {
	case all:
		return StatusRunning
	case anyErr:
		return StatusError
	case anyRunning:
		return StatusDegraded
	}
	return StatusStopped
}

// SyncProjectStatus inspects every service container, writes changed
// service statuses and the aggregate project status, and returns the latter
// (syncProjectStatusFromContainers). Services without a container name count
// as stopped.
func (r *Runtime) SyncProjectStatus(ctx context.Context, projectID string) (string, error) {
	services, err := r.Store.ListServices(ctx, store.ServiceFilter{ProjectID: &projectID}, store.OrderNone)
	if err != nil {
		return "", err
	}
	statuses := make([]string, 0, len(services))
	for _, svc := range services {
		status := StatusStopped
		if svc.ContainerName != nil && *svc.ContainerName != "" {
			status = DockerStatusToServiceStatus(r.Docker.ContainerStatus(ctx, *svc.ContainerName))
		}
		if svc.Status == nil || *svc.Status != status {
			if _, err := r.Store.UpdateService(ctx, svc.ID, store.ServiceUpdate{Status: store.Some(store.Ptr(status))}); err != nil {
				return "", err
			}
		}
		statuses = append(statuses, status)
	}
	agg := AggregateProjectStatus(statuses)
	if _, err := r.Store.UpdateProject(ctx, projectID, store.ProjectUpdate{Status: store.Some(store.Ptr(agg))}); err != nil {
		return "", err
	}
	return agg, nil
}

// SetProjectAndServicesStatus writes status to the project and all of its
// services (the paired project.update + service.updateMany of the routes).
func (r *Runtime) SetProjectAndServicesStatus(ctx context.Context, projectID, status string) error {
	if _, err := r.Store.UpdateProject(ctx, projectID, store.ProjectUpdate{Status: store.Some(store.Ptr(status))}); err != nil {
		return err
	}
	_, err := r.Store.UpdateServices(ctx, store.ServiceFilter{ProjectID: &projectID}, store.ServiceUpdate{Status: store.Some(store.Ptr(status))})
	return err
}

// RecoverOnBoot repairs state left by a restart (recoverDeploymentStateOnBoot):
// every in_progress deployment becomes failed with "❌ interrupted by
// restart" appended to its logs; every project stuck in "building" is
// re-synced from Docker when any of its containers runs, else it and its
// services are set to "error". Run it once at startup, before serving
// deploys.
func (r *Runtime) RecoverOnBoot(ctx context.Context) error {
	inProgress := StatusInProgress
	rows, err := r.Store.ListDeployments(ctx, store.DeploymentFilter{Status: &inProgress}, store.OrderNone, store.Page{})
	if err != nil {
		return fmt.Errorf("list in-progress deployments: %w", err)
	}
	for _, d := range rows {
		logs := ""
		if d.Logs != nil {
			logs = *d.Logs
		}
		now := db.Now()
		_, err := r.Store.UpdateDeployment(ctx, d.ID, store.DeploymentUpdate{
			Status:     store.Some(store.Ptr(StatusFailed)),
			FinishedAt: store.Some(&now),
			Logs:       store.Some(store.Ptr(logs + "\n❌ " + interruptedMessage + "\n")),
		})
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("fail interrupted deployment %s: %w", d.ID, err)
		}
	}

	projects, err := r.Store.ListProjects(ctx, store.ProjectFilter{StatusIn: []string{StatusBuilding}}, store.OrderNone)
	if err != nil {
		return fmt.Errorf("list building projects: %w", err)
	}
	for _, p := range projects {
		services, err := r.Store.ListServices(ctx, store.ServiceFilter{ProjectID: &p.ID}, store.OrderNone)
		if err != nil {
			return err
		}
		anyRunning := false
		for _, svc := range services {
			if svc.ContainerName == nil || *svc.ContainerName == "" {
				continue
			}
			if r.Docker.ContainerStatus(ctx, *svc.ContainerName).Running {
				anyRunning = true
			}
		}
		if anyRunning {
			if _, err := r.SyncProjectStatus(ctx, p.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			continue
		}
		if err := r.SetProjectAndServicesStatus(ctx, p.ID, StatusError); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return nil
}

// FailDeployment marks a deployment failed (with logs when non-nil) unless it
// was cancelled meanwhile — an intentional cancel is never overwritten — and
// then sets the project and its services to "error" (failDeploymentState).
// Errors are swallowed; the result
// reports whether the row was marked failed.
func (r *Runtime) FailDeployment(ctx context.Context, projectID, deploymentID string, logs *string) bool {
	now := db.Now()
	upd := store.DeploymentUpdate{Status: store.Some(store.Ptr(StatusFailed)), FinishedAt: store.Some(&now)}
	if logs != nil {
		upd.Logs = store.Some(logs)
	}
	cancelled := StatusCancelled
	n, err := r.Store.UpdateDeployments(ctx, store.DeploymentFilter{ID: &deploymentID, StatusNot: &cancelled}, upd)
	if err != nil || n == 0 {
		return false
	}
	status := store.Some(store.Ptr(StatusError))
	_, _ = r.Store.UpdateProject(ctx, projectID, store.ProjectUpdate{Status: status})
	_, _ = r.Store.UpdateServices(ctx, store.ServiceFilter{ProjectID: &projectID}, store.ServiceUpdate{Status: status})
	return true
}
