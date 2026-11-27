// Package ports serves /api/ports: the host-port pool with its allocations,
// the live projects that run without a host port, and manual release of an
// allocation (the port of backend/src/routes/ports.ts).
package ports

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

// ContainerChecker reports whether a container is running. Lookup errors
// (Docker unreachable, unknown container) report false, as
// dockerService.getContainerStatus did.
type ContainerChecker interface {
	Running(ctx context.Context, name string) bool
}

// engineChecker implements ContainerChecker with the Engine API: inspect by
// exact name, then a name-filtered list (partial match).
type engineChecker struct{ c *client.Client }

func (e engineChecker) Running(ctx context.Context, name string) bool {
	if e.c == nil {
		return false
	}
	if res, err := e.c.ContainerInspect(ctx, name, client.ContainerInspectOptions{}); err == nil {
		return res.Container.State != nil && res.Container.State.Running
	}
	list, err := e.c.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("name", name)})
	if err != nil || len(list.Items) == 0 {
		return false
	}
	return string(list.Items[0].State) == "running"
}

type handlers struct {
	d       *app.Deps
	running ContainerChecker
}

// Mount registers the /api/ports routes on r, which is already scoped to
// /api/ports and wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {
	MountWith(r, d, engineChecker{d.Docker})
}

// MountWith is Mount with an explicit container checker (tests).
func MountWith(r chi.Router, d *app.Deps, running ContainerChecker) {
	h := &handlers{d: d, running: running}
	r.Get("/", h.list)
	r.Delete("/{port}", h.release)
}

// freePort is a pool port without a row.
type freePort struct {
	Port      int     `json:"port"`
	ProjectID *string `json:"project_id"`
	IsLocked  bool    `json:"is_locked"`
}

type privateProject struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          *string `json:"status"`
	ProjectType     *string `json:"project_type"`
	DBEngine        *string `json:"db_engine"`
	PublishHostPort bool    `json:"publish_host_port"`
	Reason          string  `json:"reason"`
}

type pool struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type listBody struct {
	Ports          []any            `json:"ports"`
	PrivateRunning []privateProject `json:"private_running"`
	Pool           pool             `json:"pool"`
}

const (
	reasonDatabase = "Managed databases stay off the host — link them to apps over Docker DNS"
	reasonPublish  = "Publish host ports is on — redeploy to claim a pool port"
	reasonPrivate  = "Private by default — add a domain or enable Publish host ports + redeploy"
)

// GET / — one entry per pool port (the row with its project when allocated)
// plus the live projects running without a host port.
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.d.Store.ListPortsWithProject(ctx)
	if err != nil {
		h.fail(w, "Failed to list ports", err)
		return
	}
	byPort := make(map[int]*store.PortWithProject, len(rows))
	allocated := map[string]bool{}
	for _, p := range rows {
		if _, dup := byPort[p.Port.Port]; !dup {
			byPort[p.Port.Port] = p
		}
		if p.IsLocked != nil && *p.IsLocked && p.ProjectID != nil && *p.ProjectID != "" {
			allocated[*p.ProjectID] = true
		}
	}
	cfg := h.d.Config
	ports := []any{}
	for p := cfg.PortRangeStart; p <= cfg.PortRangeEnd; p++ {
		if row, ok := byPort[p]; ok {
			ports = append(ports, row)
		} else {
			ports = append(ports, freePort{Port: p})
		}
	}

	live, err := h.d.Store.ListActiveProjectsForPorts(ctx)
	if err != nil {
		h.fail(w, "Failed to list ports", err)
		return
	}
	private := []privateProject{}
	for _, p := range live {
		if allocated[p.ID] || hasHostPort(p.Services) {
			continue
		}
		reason := reasonPrivate
		switch {
		case p.ProjectType != nil && *p.ProjectType == "database":
			reason = reasonDatabase
		case p.PublishHostPort:
			reason = reasonPublish
		}
		private = append(private, privateProject{ID: p.ID, Name: p.Name, Status: p.Status, ProjectType: p.ProjectType,
			DBEngine: p.DBEngine, PublishHostPort: p.PublishHostPort, Reason: reason})
	}
	httpx.JSON(w, http.StatusOK, listBody{Ports: ports, PrivateRunning: private, Pool: pool{Start: cfg.PortRangeStart, End: cfg.PortRangeEnd}})
}

func hasHostPort(services []*store.ActiveProjectService) bool {
	for _, s := range services {
		if s.Port != nil && *s.Port > 0 {
			return true
		}
	}
	return false
}

var digits = regexp.MustCompile(`^[0-9]+$`)

// parsePort parses a port path parameter strictly: decimal digits only, so
// `77abc` and `-1` are rejected (TS parseInt accepted numeric prefixes).
func parsePort(s string) (int, bool) {
	if !digits.MatchString(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// DELETE /:port — release a port row unless a running container or a
// service/project still uses the port.
func (h *handlers) release(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to delete port"
	ctx := r.Context()
	port, ok := parsePort(chi.URLParam(r, "port"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid port")
		return
	}
	services, err := h.d.Store.ListServices(ctx, store.ServiceFilter{Port: &port}, store.OrderNone)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	projectUses, err := h.d.Store.ProjectUsesPort(ctx, port)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	for _, svc := range services {
		if svc.ContainerName == nil || *svc.ContainerName == "" {
			continue
		}
		if h.running.Running(ctx, *svc.ContainerName) {
			httpx.Error(w, http.StatusConflict, "Port is in use by a running container")
			return
		}
	}
	if len(services) > 0 || projectUses {
		httpx.Error(w, http.StatusConflict, "Port is still assigned to a service or project")
		return
	}
	if _, err := h.d.Store.DeletePort(ctx, port); err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error(msg, slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, msg)
}
