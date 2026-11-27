// Package projects serves /api/projects: project CRUD with the Docker status
// sync on read, creation from a GitHub repository or a ZIP upload, build
// detection, persistent storage, env variables, auto-deploy settings and the
// full project teardown on delete (the port of backend/src/routes/projects.ts).
//
// Deleting a project is owned here and exported as Service.DeleteProject so
// other route groups can call it.
package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/store"
)

// GitHubApp is the GitHub App surface project creation uses to clone
// private repositories (getInstallationIdForRepo and getInstallationToken of
// routes/github.ts).
type GitHubApp interface {
	// InstallationIDForRepo returns the App installation that covers
	// owner/repo (falling back to the saved global installation id).
	InstallationIDForRepo(ctx context.Context, owner, repo string) (string, error)
	// InstallationToken mints an installation access token.
	InstallationToken(ctx context.Context, installationID string) (string, error)
}

// Options are the optional collaborators of MountWith.
type Options struct {
	// GitHub authenticates clones of private repositories. Nil clones
	// anonymously, which is what the TS code fell back to whenever the App
	// was not configured.
	GitHub GitHubApp
}

// Messages shared by several routes.
const (
	msgProjectNotFound = "Project not found"
	msgDomainFormat    = "Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com)."
	msgDockerDown      = "Docker is not available"
	webhookURL         = "/api/github/webhook"
	pingTimeout        = 5 * time.Second
)

// domainRe is DOMAIN_REGEX of projects.ts.
var domainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

// validDomainList is isValidDomainList: every non-empty comma-separated
// entry must match domainRe; the empty string is valid.
func validDomainList(s string) bool {
	for _, d := range strings.Split(s, ",") {
		if d = js.Trim(d); d != "" && !domainRe.MatchString(d) {
			return false
		}
	}
	return true
}

type handlers struct {
	d      *app.Deps
	svc    *Service
	github GitHubApp
}

// MountWith registers the /api/projects routes on r, which is already
// scoped to /api/projects and wrapped in the group's authentication
// middleware. Without o.GitHub, private GitHub repositories are cloned
// anonymously.
func MountWith(r chi.Router, d *app.Deps, o Options) {
	h := &handlers{d: d, svc: NewService(d), github: o.GitHub}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.remove)
	r.Get("/{id}/build/detect", h.detect)
	r.Get("/{id}/storage", h.listStorage)
	r.Post("/{id}/storage", h.createStorage)
	r.Delete("/{id}/storage/{storageId}", h.deleteStorage)
	r.Get("/{id}/env", h.listEnv)
	r.Post("/{id}/env", h.createEnv)
	r.Post("/{id}/env/bulk", h.bulkEnv)
	r.Delete("/{id}/env/{envId}", h.deleteEnv)
	r.Patch("/{id}/auto-deploy", h.setAutoDeploy)
	r.Get("/{id}/auto-deploy", h.getAutoDeploy)
}

// param is req.params[name]: chi matches on the escaped path when it has
// escapes, so decode once like Express.
func param(r *http.Request, name string) string {
	v := chi.URLParam(r, name)
	if r.URL.RawPath != "" {
		if decoded, err := url.PathUnescape(v); err == nil {
			return decoded
		}
	}
	return v
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error(msg, slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, msg)
}

// dockerAvailable answers 503 when the daemon cannot be reached and reports
// whether the request may go on.
func (h *handlers) dockerAvailable(w http.ResponseWriter, r *http.Request) bool {
	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()
	if err := h.d.DockerX.Ping(ctx); err != nil && dockerx.IsUnavailable(err) {
		httpx.Error(w, http.StatusServiceUnavailable, msgDockerDown)
		return false
	}
	return true
}

func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// syncOnRead is the status/branch refresh GET / and GET /:id run on every
// project: a GitHub project without a recorded branch takes the checkout's
// current branch, and a project with a container (its own or its first
// service's) that is not building takes the status aggregated from Docker.
// The returned row carries the refreshed fields over the values read.
func (h *handlers) syncOnRead(ctx context.Context, p *store.Project) error {
	if p.SourceType != nil && *p.SourceType == "github" && (p.GithubBranch == nil || *p.GithubBranch == "") {
		dir := h.d.Deploy.ProjectPath(p.ID)
		if _, err := os.Stat(dir); err == nil {
			if branch, ok := h.d.Git.CurrentBranch(ctx, dir); ok && branch != "" {
				if _, err := h.d.Store.UpdateProject(ctx, p.ID, store.ProjectUpdate{GithubBranch: store.Some(&branch)}); err != nil {
					return err
				}
				p.GithubBranch = &branch
			}
		}
	}
	container := p.ContainerName
	if container == nil || *container == "" {
		svc, err := h.d.Store.FindService(ctx, store.ServiceFilter{ProjectID: &p.ID}, store.OrderNone)
		switch {
		case err == nil:
			container = svc.ContainerName
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
	}
	if container != nil && *container != "" && (p.Status == nil || *p.Status != deploy.StatusBuilding) {
		status, err := h.d.Deploy.SyncProjectStatus(ctx, p.ID)
		if err != nil {
			return err
		}
		p.Status = &status
	}
	return nil
}

// GET / — every project, newest first, synced with Git and Docker.
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	projects, err := h.d.Store.ListProjects(r.Context(), store.ProjectFilter{}, store.OrderCreatedDesc)
	if err != nil {
		h.fail(w, "Failed to list projects", err)
		return
	}
	for _, p := range projects {
		if err := h.syncOnRead(r.Context(), p); err != nil {
			h.fail(w, "Failed to list projects", err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, projects)
}

// GET /:id — one project, synced with Git and Docker.
func (h *handlers) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Store.GetProject(r.Context(), param(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err == nil {
		err = h.syncOnRead(r.Context(), p)
	}
	if err != nil {
		h.fail(w, "Failed to get project", err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// stringOrNull checks a field that is written when defined: it must be a
// string or null. ok is false (and the response written) otherwise.
func stringOrNull(w http.ResponseWriter, v val, field string) (*string, bool) {
	if v.isNull() {
		return nil, true
	}
	s, isStr := v.str()
	if !isStr {
		httpx.Error(w, http.StatusBadRequest, field+" must be a string")
		return nil, false
	}
	return &s, true
}

// truthyString checks a field that is used only when truthy: a truthy value
// must be a string.
func truthyString(w http.ResponseWriter, v val, field string) (string, bool) {
	if !v.truthy() {
		return "", true
	}
	s, isStr := v.str()
	if !isStr {
		httpx.Error(w, http.StatusBadRequest, field+" must be a string")
		return "", false
	}
	return s, true
}

// checkDomain validates a domain field the way `domain &&
// !isValidDomainList(domain)` did; a truthy non-string (which made the TS
// code throw) is the same format error.
func checkDomain(w http.ResponseWriter, v val) bool {
	if !v.truthy() {
		return true
	}
	if s, ok := v.str(); ok && validDomainList(s) {
		return true
	}
	httpx.Error(w, http.StatusBadRequest, msgDomainFormat)
	return false
}

// PATCH /:id — update the editable project fields. Only these fields are
// written: name and project_type when truthy; description, github_url,
// domain, build_type, base_directory, dockerfile_path, internal_port and
// publish_host_port when present.
func (h *handlers) update(w http.ResponseWriter, r *http.Request) {
	b := jsonFields(httpx.BodyFrom(r))
	domain := b.get("domain")
	if !checkDomain(w, domain) {
		return
	}
	var upd store.ProjectUpdate
	name, ok := truthyString(w, b.get("name"), "name")
	if !ok {
		return
	}
	if name != "" {
		upd.Name = store.Some(name)
	}
	projectType, ok := truthyString(w, b.get("project_type"), "project_type")
	if !ok {
		return
	}
	if projectType != "" {
		upd.ProjectType = store.Some(&projectType)
	}
	for _, f := range []struct {
		key string
		dst *store.Opt[*string]
	}{{"description", &upd.Description}, {"github_url", &upd.GithubURL}, {"domain", &upd.Domain}} {
		v := b.get(f.key)
		if !v.defined {
			continue
		}
		s, ok := stringOrNull(w, v, f.key)
		if !ok {
			return
		}
		*f.dst = store.Some(s)
	}
	// github_url is validated as on create (TS stored any string, so a PATCH
	// could point the next deploy's token at another host); "" still clears.
	if u := upd.GithubURL; u.Set && u.Value != nil && js.Trim(*u.Value) != "" {
		trimmed := js.Trim(*u.Value)
		if !validGithubRepoURL(trimmed) || h.d.Git.ValidateURL(trimmed) != nil {
			httpx.Error(w, http.StatusBadRequest, msgGithubURL)
			return
		}
		upd.GithubURL = store.Some(&trimmed)
	}
	if v := b.get("build_type"); v.defined {
		upd.BuildType = store.Some(deploy.NormalizeBuildType(v.v))
	}
	if v := b.get("base_directory"); v.defined {
		upd.BaseDirectory = store.Some(baseDirectory(v))
	}
	if v := b.get("dockerfile_path"); v.defined {
		upd.DockerfilePath = store.Some(dockerfilePath(v))
	}
	if v := b.get("internal_port"); v.defined {
		upd.InternalPort = store.Some(clampPort(v))
	}
	if v := b.get("publish_host_port"); v.defined {
		upd.PublishHostPort = store.Some(v.truthy())
	}
	p, err := h.d.Store.UpdateProject(r.Context(), param(r, "id"), upd)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err != nil {
		h.fail(w, "Failed to update project", err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// DELETE /:id — tear the project down and delete it (Service.DeleteProject).
func (h *handlers) remove(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	// Teardown runs to completion even if the client goes away, so Docker,
	// files and rows are never left half-removed by a disconnect.
	err := h.svc.DeleteProject(h.d.Background, id)
	var conflict *ConflictError
	switch {
	case err == nil:
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	case errors.As(err, &conflict):
		httpx.Error(w, http.StatusConflict, conflict.Message)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
	case errors.Is(err, ErrDockerUnavailable):
		httpx.Error(w, http.StatusServiceUnavailable, msgDockerDown)
	default:
		h.d.Log.Error("Failed to delete project "+id, slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "Failed to delete project")
	}
}

type autoDeployBody struct {
	AutoDeploy    *bool   `json:"auto_deploy"`
	WebhookSecret *string `json:"webhook_secret"`
	WebhookURL    *string `json:"webhook_url"`
}

type autoDeployStatus struct {
	autoDeployBody
	Available bool `json:"available"`
}

func webhookURLIf(on bool) *string {
	if !on {
		return nil
	}
	u := webhookURL
	return &u
}

// PATCH /:id/auto-deploy — toggle auto-deploy of a GitHub project, creating
// its webhook secret on first enable.
func (h *handlers) setAutoDeploy(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	enabled := jsonFields(httpx.BodyFrom(r)).get("enabled")
	p, err := h.d.Store.GetProject(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err != nil {
		h.fail(w, "Failed to update auto-deploy settings", err)
		return
	}
	if p.SourceType == nil || *p.SourceType != "github" {
		httpx.Error(w, http.StatusBadRequest, "Auto-deploy is only available for GitHub projects")
		return
	}
	var autoDeploy store.Opt[*bool]
	switch b, isBool := enabled.v.(bool); {
	case !enabled.defined:
	case enabled.isNull():
		autoDeploy = store.Some[*bool](nil)
	case isBool:
		autoDeploy = store.Some(&b)
	default:
		httpx.Error(w, http.StatusBadRequest, "enabled must be a boolean")
		return
	}
	secret := p.WebhookSecret
	if enabled.truthy() && (secret == nil || *secret == "") {
		s, err := generateWebhookSecret()
		if err != nil {
			h.fail(w, "Failed to update auto-deploy settings", err)
			return
		}
		secret = &s
	}
	updated, err := h.d.Store.UpdateProject(r.Context(), id, store.ProjectUpdate{AutoDeploy: autoDeploy, WebhookSecret: store.Some(secret)})
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err != nil {
		h.fail(w, "Failed to update auto-deploy settings", err)
		return
	}
	// GitHub App delivers every repo push to this one URL; projects are
	// matched by github_url.
	httpx.JSON(w, http.StatusOK, autoDeployBody{AutoDeploy: updated.AutoDeploy, WebhookSecret: updated.WebhookSecret, WebhookURL: webhookURLIf(enabled.truthy())})
}

// GET /:id/auto-deploy — auto-deploy status; the webhook secret is only
// revealed while auto-deploy is on.
func (h *handlers) getAutoDeploy(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Store.GetProject(r.Context(), param(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err != nil {
		h.fail(w, "Failed to get auto-deploy status", err)
		return
	}
	on := p.AutoDeploy != nil && *p.AutoDeploy
	out := autoDeployStatus{autoDeployBody: autoDeployBody{AutoDeploy: &on, WebhookURL: webhookURLIf(on)},
		Available: p.SourceType != nil && *p.SourceType == "github"}
	if on {
		out.WebhookSecret = p.WebhookSecret
	}
	httpx.JSON(w, http.StatusOK, out)
}
