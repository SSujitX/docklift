// Package deployments serves /api/deployments (the port of
// backend/src/routes/deployments.ts) and owns "deploy a project": the
// Service type runs the deploy pipeline, stop, restart, cancel and rollback
// in the background and queues auto-deploys per project, for this route
// group and for the github and databases groups.
//
// Progress streams (deploy, redeploy, stop, restart, cancel, rollback) are
// plain text. The operation runs on the process background context and
// writes to a non-blocking buffer that the handler copies to the client;
// a client that disconnects stops receiving output but never aborts the
// operation.
package deployments

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/dockerx"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

type handlers struct {
	d   *app.Deps
	svc *Service
}

// Mount registers the /api/deployments routes on r, which is already scoped
// to /api/deployments and wrapped in the group's authentication middleware.
// It uses the process-wide Service (For).
func Mount(r chi.Router, d *app.Deps) { MountWith(r, d, For(d)) }

// MountWith is Mount with an explicit Service (tests).
func MountWith(r chi.Router, d *app.Deps, svc *Service) {
	h := &handlers{d: d, svc: svc}
	r.Get("/{projectId}", h.list)
	r.Get("/{projectId}/services", h.services)
	r.Get("/{projectId}/services/{serviceId}/ssl", h.serviceSSL)
	r.Post("/{projectId}/services/{serviceId}/dns-check", h.dnsCheck)
	r.Post("/{projectId}/services/{serviceId}/ssl/retry", h.sslRetry)
	r.Put("/{projectId}/services/{serviceId}", h.updateService)
	r.Post("/{projectId}/deploy", h.deploy)
	r.Post("/{projectId}/redeploy", h.deploy)
	r.Post("/{projectId}/stop", h.stop)
	r.Post("/{projectId}/restart", h.restart)
	r.Post("/{projectId}/rollback", h.rollback)
	r.Post("/{projectId}/cancel", h.cancel)
	r.Get("/{projectId}/logs", h.logs)
	r.Get("/{projectId}/stats", h.stats)
}

func projectID(r *http.Request) string { return chi.URLParam(r, "projectId") }

// list returns a project's deployments, newest first: a JSON array, or
// {items, total} with ?meta=1|true. limit/offset use parseInt semantics
// (limit falls back to 10); status filters when non-empty after trimming.
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 10
	if s, ok := queryString(q["limit"]); ok {
		if n, ok := jsParseInt(s); ok && n != 0 {
			limit = n
		}
	}
	offset := 0
	if s, ok := queryString(q["offset"]); ok {
		if n, ok := jsParseInt(s); ok {
			offset = n
		}
	}
	meta := len(q["meta"]) == 1 && (q.Get("meta") == "1" || q.Get("meta") == "true")
	pid := projectID(r)
	f := store.DeploymentFilter{ProjectID: &pid}
	if st := q["status"]; len(st) == 1 && strings.TrimSpace(st[0]) != "" {
		trimmed := strings.TrimSpace(st[0])
		f.Status = &trimmed
	}
	fail := func(err error) {
		h.d.Log.Error("list deployments", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to list deployments")
	}
	if offset < 0 {
		fail(errors.New("negative offset"))
		return
	}
	rows, err := h.listPage(r.Context(), f, limit, offset)
	if err != nil {
		fail(err)
		return
	}
	if !meta {
		httpx.JSON(w, http.StatusOK, rows)
		return
	}
	total, err := h.d.Store.CountDeployments(r.Context(), f)
	if err != nil {
		fail(err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Items []*store.Deployment `json:"items"`
		Total int                 `json:"total"`
	}{rows, total})
}

// listPage is findMany({orderBy: created_at desc, take, skip}); a negative
// take reads from the other end (the oldest rows) and keeps the descending
// order, as Prisma does.
func (h *handlers) listPage(ctx context.Context, f store.DeploymentFilter, limit, offset int) ([]*store.Deployment, error) {
	if limit > 0 {
		return h.d.Store.ListDeployments(ctx, f, store.OrderCreatedDesc, store.Page{Limit: limit, Offset: offset})
	}
	rows, err := h.d.Store.ListDeployments(ctx, f, store.OrderCreatedAsc, store.Page{Limit: -limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}

func (h *handlers) services(w http.ResponseWriter, r *http.Request) {
	pid := projectID(r)
	rows, err := h.d.Store.ListServices(r.Context(), store.ServiceFilter{ProjectID: &pid}, store.OrderNone)
	if err != nil {
		h.d.Log.Error("list services", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to list services")
		return
	}
	httpx.JSON(w, http.StatusOK, rows)
}

// containerServices lists the project's services with a container name and
// reports whether Docker is unreachable while there are any (deliberate fix
// 9: 503 instead of empty logs / null stats).
func (h *handlers) containerServices(w http.ResponseWriter, r *http.Request, failMsg string) ([]*store.Service, bool) {
	pid := projectID(r)
	rows, err := h.d.Store.ListServices(r.Context(), store.ServiceFilter{ProjectID: &pid}, store.OrderNone)
	if err != nil {
		h.d.Log.Error(failMsg, "error", err)
		httpx.Error(w, http.StatusInternalServerError, failMsg)
		return nil, false
	}
	var out []*store.Service
	for _, s := range rows {
		if strVal(s.ContainerName) != "" {
			out = append(out, s)
		}
	}
	if len(out) > 0 {
		if err := h.svc.docker.Ping(r.Context()); dockerx.IsUnavailable(err) {
			httpx.Error(w, http.StatusServiceUnavailable, ErrDockerUnavailable.Message)
			return nil, false
		}
	}
	return out, true
}

// containerLogTail is getContainerLogs' default tail.
const containerLogTail = 100

func (h *handlers) logs(w http.ResponseWriter, r *http.Request) {
	services, ok := h.containerServices(w, r, "Failed to get logs")
	if !ok {
		return
	}
	var logs jsObject[string]
	for _, s := range services {
		logs.set(s.Name, h.svc.docker.ContainerLogs(r.Context(), *s.ContainerName, containerLogTail))
	}
	httpx.JSON(w, http.StatusOK, struct {
		Logs jsObject[string] `json:"logs"`
	}{logs})
}

func (h *handlers) stats(w http.ResponseWriter, r *http.Request) {
	services, ok := h.containerServices(w, r, "Failed to get stats")
	if !ok {
		return
	}
	var stats jsObject[*dockerx.ContainerStats]
	for _, s := range services {
		stats.set(s.Name, h.svc.docker.ContainerStats(r.Context(), *s.ContainerName))
	}
	httpx.JSON(w, http.StatusOK, stats)
}

// stream answers a started operation: headers now, then its output until it
// finishes or the client leaves.
func stream(w http.ResponseWriter, r *http.Request, c *clientStream, run *Run, keepAlive bool) {
	streamHeaders(w, keepAlive)
	pump(w, r, c, run.Done())
}

// refuse answers an operation that did not start: its RequestError, or the
// route's generic failure.
func (h *handlers) refuse(w http.ResponseWriter, err error, status int, msg string) {
	var re *RequestError
	if errors.As(err, &re) {
		httpx.Error(w, re.Status, re.Message)
		return
	}
	h.d.Log.Error(msg, "error", err)
	httpx.Error(w, status, msg)
}

// bodyString reads an optional string field: absent or falsy values are "";
// another type is a 400 (deliberate fix 3).
func bodyString(r *http.Request, key string) (string, error) {
	v := httpx.BodyFrom(r).Get(key)
	if !v.Truthy() {
		return "", nil
	}
	s, ok := v.String()
	if !ok {
		return "", reqErr(http.StatusBadRequest, key+" must be a string")
	}
	return s, nil
}

func requestHost(r *http.Request) string {
	host, _, _ := strings.Cut(r.Host, ":")
	return host
}

func (h *handlers) deploy(w http.ResponseWriter, r *http.Request) {
	trigger, err := bodyString(r, "trigger")
	if err != nil {
		h.refuse(w, err, 0, "")
		return
	}
	msg, err := bodyString(r, "commit_message")
	if err != nil {
		h.refuse(w, err, 0, "")
		return
	}
	c := newClientStream()
	run, err := h.svc.StartDeploy(r.Context(), projectID(r), DeployOptions{Trigger: trigger, CommitMessage: msg, Host: requestHost(r), Output: c})
	if err != nil {
		h.refuse(w, err, http.StatusInternalServerError, "Deployment failed")
		return
	}
	stream(w, r, c, run, true)
}

// startLifecycle runs stop or restart. A failure before the row exists is
// reported like the TS routes did (stop: a 200 text error; restart: 500).
func (h *handlers) startLifecycle(w http.ResponseWriter, r *http.Request,
	start func(context.Context, string, io.Writer) (*Run, error), failText bool, failMsg string) {
	c := newClientStream()
	run, err := start(r.Context(), projectID(r), c)
	var re *RequestError
	switch {
	case err == nil:
		stream(w, r, c, run, false)
	case errors.As(err, &re) || !failText:
		h.refuse(w, err, http.StatusInternalServerError, failMsg)
	default:
		h.d.Log.Error(failMsg, "error", err)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "\n❌ Error: "+err.Error()+"\n")
	}
}

func (h *handlers) stop(w http.ResponseWriter, r *http.Request) {
	h.startLifecycle(w, r, h.svc.Stop, true, "Stop failed")
}

func (h *handlers) restart(w http.ResponseWriter, r *http.Request) {
	h.startLifecycle(w, r, h.svc.Restart, false, "Restart failed")
}

func (h *handlers) cancel(w http.ResponseWriter, r *http.Request) {
	c := newClientStream()
	run, err := h.svc.Cancel(r.Context(), projectID(r), c)
	if err != nil {
		h.refuse(w, err, http.StatusInternalServerError, "Failed to cancel build")
		return
	}
	stream(w, r, c, run, false)
}

func (h *handlers) rollback(w http.ResponseWriter, r *http.Request) {
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		h.d.Log.Error("Rollback error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		return
	}
	target := ""
	if s, isStr := httpx.BodyFrom(r).Get("deploymentId").String(); isStr {
		target = strings.TrimSpace(s)
	}
	if target == "" {
		httpx.Error(w, http.StatusBadRequest, "deploymentId is required")
		return
	}
	c := newClientStream()
	run, err := h.svc.Rollback(r.Context(), projectID(r), target, c)
	if err != nil {
		var re *RequestError
		if errors.As(err, &re) {
			httpx.Error(w, re.Status, re.Message)
			return
		}
		h.d.Log.Error("Rollback error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	stream(w, r, c, run, false)
}

// serviceOf returns the service of the project, or nil (404) when missing.
func (h *handlers) serviceOf(ctx context.Context, r *http.Request) (*store.Service, error) {
	pid, sid := projectID(r), chi.URLParam(r, "serviceId")
	svc, err := h.d.Store.FindService(ctx, store.ServiceFilter{ID: &sid, ProjectID: &pid}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return svc, err
}
