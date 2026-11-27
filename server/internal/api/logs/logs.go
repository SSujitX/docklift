// Package logs serves /api/logs (the port of backend/src/routes/logs.ts):
// the containers of a project with their live state, and the SSE stream of a
// container's logs. Stream is also used by /api/system/logs/:service.
package logs

import (
	"context"
	"errors"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/dockerx"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/store"
)

// Docker is the part of the Engine API client these routes use;
// *client.Client implements it.
type Docker interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
}

// ErrDockerNotConfigured is returned when the process has no Docker client.
var ErrDockerNotConfigured = errors.New("docker client is not configured")

// DockerUnavailableMessage is the 503 body of deliberate fix 9.
const DockerUnavailableMessage = "Docker is not available"

// DockerFrom returns d.Docker as a Docker, or nil when there is none (a nil
// *client.Client must not become a non-nil interface).
func DockerFrom(d *app.Deps) Docker {
	if d.Docker == nil {
		return nil
	}
	return d.Docker
}

// IsDockerUnavailable reports an error meaning the daemon cannot be reached
// (as opposed to an error answered by the daemon, such as "no such container").
// It also recognises dockerx.ErrUnavailable from the Deps.DockerX wrapper.
func IsDockerUnavailable(err error) bool {
	return errors.Is(err, ErrDockerNotConfigured) || dockerx.IsUnavailable(err)
}

type handlers struct {
	d      *app.Deps
	docker Docker
}

// Mount registers the /api/logs routes on r, which is already scoped to
// /api/logs and wrapped in the group's authentication middleware (SSE query
// token for GET */stream/*, session JWT otherwise).
func Mount(r chi.Router, d *app.Deps) {
	MountWith(r, d, DockerFrom(d))
}

// MountWith is Mount with an explicit Docker client (nil: none configured).
func MountWith(r chi.Router, d *app.Deps, docker Docker) {
	h := &handlers{d: d, docker: docker}
	r.Get("/{projectId}/containers", h.containers)
	r.Get("/{projectId}/stream/{containerName}", h.stream)
}

type containerInfo struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ContainerName  *string `json:"container_name"`
	Port           *int    `json:"port"`
	InternalPort   *int    `json:"internal_port"`
	DockerfilePath string  `json:"dockerfile_path"`
	Status         string  `json:"status"`
	Running        bool    `json:"running"`
}

// GET /:projectId/containers — the project's services with live container state.
func (h *handlers) containers(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	if _, err := h.d.Store.GetProject(r.Context(), projectID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Project not found")
			return
		}
		h.fail(w, "Failed to list containers", err)
		return
	}
	services, err := h.d.Store.ListServices(r.Context(), store.ServiceFilter{ProjectID: &projectID}, store.OrderNone)
	if err != nil {
		h.fail(w, "Failed to list containers", err)
		return
	}
	out := make([]containerInfo, 0, len(services))
	for _, svc := range services {
		status, running := "unknown", false
		if svc.ContainerName != nil && *svc.ContainerName != "" {
			status, running, err = ContainerStatus(r.Context(), h.docker, *svc.ContainerName)
			if err != nil {
				h.d.Log.Warn("Failed to list containers: Docker unavailable", "error", err)
				httpx.Error(w, http.StatusServiceUnavailable, DockerUnavailableMessage)
				return
			}
		}
		out = append(out, containerInfo{ID: svc.ID, Name: svc.Name, ContainerName: svc.ContainerName, Port: svc.Port,
			InternalPort: svc.InternalPort, DockerfilePath: svc.DockerfilePath, Status: status, Running: running})
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ContainerStatus is getContainerStatus of services/docker.ts: the state of
// the container with this exact name, else of the first container whose name
// matches the filter, else "not_found". The error is non-nil only when the
// daemon cannot be reached (deliberate fix 9; TypeScript reported
// "not_found" then).
func ContainerStatus(ctx context.Context, docker Docker, name string) (string, bool, error) {
	if docker == nil {
		return "", false, ErrDockerNotConfigured
	}
	info, err := docker.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err == nil && info.Container.State != nil {
		return string(info.Container.State.Status), info.Container.State.Running, nil
	}
	if err != nil && client.IsErrConnectionFailed(err) {
		return "", false, err
	}
	list, err := docker.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("name", name)})
	if err != nil {
		if client.IsErrConnectionFailed(err) {
			return "", false, err
		}
		return "not_found", false, nil
	}
	if len(list.Items) > 0 {
		state := string(list.Items[0].State)
		return state, state == "running", nil
	}
	return "not_found", false, nil
}

// GET /:projectId/stream/:containerName — SSE log stream of a container that
// belongs to the project.
func (h *handlers) stream(w http.ResponseWriter, r *http.Request) {
	projectID, name := chi.URLParam(r, "projectId"), chi.URLParam(r, "containerName")
	_, err := h.d.Store.FindService(r.Context(), store.ServiceFilter{ProjectID: &projectID, ContainerName: &name}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Container not found for this project")
		return
	}
	if err != nil {
		h.fail(w, "Failed to stream logs", err)
		return
	}
	Stream(w, r, h.docker, name, Tail(r))
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error(msg, "error", err)
	httpx.Error(w, http.StatusInternalServerError, msg)
}

// Tail is `parseInt(req.query.tail) || 200` as the decimal string dockerode
// sent; negative values pass through (the daemon reads them as "all").
func Tail(r *http.Request) string {
	n := js.ParseInt(r.URL.Query().Get("tail"))
	if math.IsNaN(n) || n == 0 {
		return "200"
	}
	return js.NumberString(n)
}
