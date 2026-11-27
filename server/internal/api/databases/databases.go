// Package databases serves /api/databases: the managed database engine
// catalog (with live Docker Hub versions), creation of a managed database
// project (credentials, data volume, service row), its listing and
// connection info, and links that inject a database's connection URL into an
// app project (the port of backend/src/routes/databases.ts).
//
// Starting, stopping, redeploying and deleting a managed database are not
// routes of this group: they go through /api/deployments and /api/projects
// like any other project, exactly as in the TS backend.
package databases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/store"
)

type handlers struct {
	d *app.Deps
}

// Mount registers the /api/databases routes on r, which is already scoped to
// /api/databases and wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {
	h := &handlers{d: d}
	r.Get("/engines", h.engines)
	r.Get("/", h.list)
	r.Get("/links/by-app/{appProjectId}", h.linksByApp)
	r.Post("/", h.create)
	r.Get("/{id}/connection", h.connection)
	r.Get("/{id}/links", h.links)
	r.Post("/{id}/links", h.createLink)
	r.Delete("/{id}/links/{linkId}", h.deleteLink)
}

// msgDockerUnavailable is the body of the deliberate 503 fix.
const msgDockerUnavailable = "Docker is not available"

// pingTimeout bounds the daemon check that classifies a failed volume create.
const pingTimeout = 5 * time.Second

// GET /engines — the catalog with live Docker Hub tags (cached; the static
// versions when Hub is unreachable).
func (h *handlers) engines(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, h.d.Deploy.Hub.ListEnginesWithLiveVersions(r.Context()))
}

// GET / — every managed database project, newest first, with its services
// and links (each with the linked app's {id, name, status}).
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	projects, err := h.d.Store.ListDatabaseProjects(r.Context())
	if err != nil {
		h.fail(w, "Failed to list databases", err)
		return
	}
	httpx.JSON(w, http.StatusOK, projects)
}

// GET /links/by-app/:appProjectId — the links targeting an app project,
// newest first, with the database's {id, name, status, db_engine}.
func (h *handlers) linksByApp(w http.ResponseWriter, r *http.Request) {
	links, err := h.d.Store.ListLinksForApp(r.Context(), chi.URLParam(r, "appProjectId"))
	if err != nil {
		h.fail(w, "Failed to list app database links", err)
		return
	}
	httpx.JSON(w, http.StatusOK, links)
}

// createdCredentials are returned once, at create, so the operator can copy
// the password.
type createdCredentials struct {
	Username *string `json:"username"`
	Database string  `json:"database"`
	Password string  `json:"password"`
}

// createdDatabase is the 201 body of POST /: the project row followed by the
// fields the TS route spread onto it.
type createdDatabase struct {
	store.Project
	Services          []*store.Service          `json:"services"`
	PersistentVolumes []*store.PersistentVolume `json:"persistent_volumes"`
	Engine            deploy.Engine             `json:"engine"`
	Image             string                    `json:"image"`
	Version           string                    `json:"version"`
	ConnectionURL     string                    `json:"connection_url"`
	Credentials       createdCredentials        `json:"credentials"`
}

// errCreateFailed is the generic 500 message of POST /.
const errCreateFailed = "Failed to create database"

// dataVolumeLabel is the display name of a managed database's data volume.
const dataVolumeLabel = "data"

// POST / — create a managed database: project, README marker, service, the
// credential env, the data volume row and the Docker volume. The container
// is started by the deploy the dashboard triggers next. Nothing is published
// on a host port (publish_host_port false, service port null).
func (h *handlers) create(w http.ResponseWriter, r *http.Request) {
	body := httpx.BodyFrom(r)
	name := ""
	if s, ok := body.Get("name").String(); ok {
		name = js.Trim(s)
	}
	if name == "" || httpx.UTF16Len(name) > 120 {
		httpx.Error(w, http.StatusBadRequest, "Name is required (max 120 characters)")
		return
	}
	engineID, _ := body.Get("engine").String()
	engine, ok := deploy.GetEngine(engineID)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid engine. Use one of: "+strings.Join(deploy.EngineIDs, ", "))
		return
	}
	// The request context may end when the client goes away; once rows are
	// written the create (or its rollback) must run to completion.
	ctx := context.WithoutCancel(r.Context())

	// The live catalog of this engine only (TS refreshed all five and kept
	// this one; the selection is the same).
	liveEngine := engine
	liveEngine.Versions = h.d.Deploy.Hub.LiveVersions(r.Context(), engine.ID, engine.ImageRepo, engine.Versions)
	allowed := make([]string, 0, len(liveEngine.Versions))
	for _, v := range liveEngine.Versions {
		allowed = append(allowed, v.Tag)
	}
	resolved, err := deploy.ResolveEngineImage(liveEngine, requestedVersion(body), allowed)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	password, err := deploy.GenerateDatabasePassword()
	if err != nil {
		h.fail(w, errCreateFailed, err)
		return
	}
	creds := deploy.DefaultDatabaseCredentials(name, password)

	project, err := h.d.Store.CreateProject(ctx, store.ProjectCreate{
		Name:            name,
		Description:     store.Ptr(engine.Label + " " + resolved.Tag + " managed by DockLift"),
		SourceType:      store.Ptr("managed"),
		ProjectType:     store.Ptr("database"),
		DBEngine:        store.Ptr(engine.ID),
		Status:          store.Ptr("pending"),
		AutoDeploy:      store.Ptr(false),
		BuildType:       store.Ptr("dockerfile"),
		BaseDirectory:   store.Ptr("."),
		InternalPort:    store.Ptr(engine.Port),
		PublishHostPort: store.Ptr(false),
	})
	if err != nil {
		h.fail(w, errCreateFailed, err)
		return
	}
	projectPath := h.d.Deploy.ProjectPath(project.ID)
	rollback := func() {
		if err := h.d.Store.DeleteProject(ctx, project.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			h.d.Log.Error("[databases] rollback: delete project", "project", project.ID, slog.Any("error", err))
		}
		if err := os.RemoveAll(projectPath); err != nil {
			h.d.Log.Warn("[databases] rollback: remove project directory", "project", project.ID, slog.Any("error", err))
		}
	}

	service, volume, err := h.createRows(ctx, project, engine, resolved.Image, creds)
	if err != nil {
		rollback()
		h.fail(w, errCreateFailed, err)
		return
	}

	labels := deploy.VolumeLabels(project.Name, project.ID, deploy.ManagedDatabaseVolumeLabel)
	if err := h.d.DockerX.VolumeCreate(ctx, volume.Name, labels...); err != nil {
		rollback()
		h.volumeFailed(ctx, w, err)
		return
	}

	shownEngine := engine
	shownEngine.Image = resolved.Image
	var username *string
	if creds.Username != "" {
		username = &creds.Username
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusCreated, createdDatabase{
		Project:           *project,
		Services:          []*store.Service{service},
		PersistentVolumes: []*store.PersistentVolume{volume},
		Engine:            shownEngine,
		Image:             resolved.Image,
		Version:           resolved.Tag,
		ConnectionURL:     deploy.ConnectionURLFor(engine, *service.ContainerName, creds),
		Credentials:       createdCredentials{Username: username, Database: creds.Database, Password: creds.Password},
	})
}

// requestedVersion is body.version, else body.tag, else body.image (the
// first that is a string), or nil for the recommended version.
func requestedVersion(body httpx.Body) *string {
	for _, key := range []string{"version", "tag", "image"} {
		if s, ok := body.Get(key).String(); ok {
			return &s
		}
	}
	return nil
}

// createRows writes the README marker, the service, the credential env and
// the data volume row of a new managed database.
func (h *handlers) createRows(ctx context.Context, project *store.Project, engine deploy.Engine, image string, creds deploy.Credentials) (*store.Service, *store.PersistentVolume, error) {
	projectPath := h.d.Deploy.ProjectPath(project.ID)
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create project directory: %w", err)
	}
	readme := "# " + project.Name + "\n\nManaged " + engine.Label + " (" + image + ").\nDo not replace with application source.\n"
	if err := os.WriteFile(filepath.Join(projectPath, "README.docklift-db.md"), []byte(readme), 0o644); err != nil {
		return nil, nil, fmt.Errorf("write README: %w", err)
	}

	containerName := deploy.ServiceContainerName(project.Name, project.ID, engine.ServiceName)
	service, err := h.d.Store.CreateService(ctx, store.ServiceCreate{
		ProjectID:      project.ID,
		Name:           engine.ServiceName,
		DockerfilePath: deploy.ManagedServiceMarker(engine.ID, image),
		ContainerName:  &containerName,
		InternalPort:   store.Ptr(engine.Port),
		Status:         store.Ptr("pending"),
	})
	if err != nil {
		return nil, nil, err
	}

	t, f := true, false
	for _, kv := range deploy.EngineRuntimeEnv(engine, creds) {
		secret := strings.Contains(strings.ToLower(kv.Key), "password")
		if _, err := h.d.Store.CreateEnvVariable(ctx, store.EnvCreate{
			ProjectID: project.ID, ServiceName: "", Key: kv.Key, Value: kv.Value,
			IsRuntime: &t, IsBuildArg: &f, IsSecret: &secret,
		}); err != nil {
			return nil, nil, fmt.Errorf("create env %s: %w", kv.Key, err)
		}
	}

	volume, err := h.d.Store.CreateVolume(ctx, store.VolumeCreate{
		ProjectID:   project.ID,
		ServiceName: engine.ServiceName,
		Name:        deploy.PersistentVolumeName(project.ID, dataVolumeLabel),
		DisplayName: dataVolumeLabel,
		MountPath:   deploy.VolumeMountForEngine(engine, image),
	})
	if err != nil {
		return nil, nil, err
	}
	return service, volume, nil
}

// volumeFailed answers a failed `docker volume create`: 503 when the daemon
// is unreachable (deliberate fix; the dashboard only shows the message),
// else 500 with docker's stderr or the TS fallback message.
func (h *handlers) volumeFailed(ctx context.Context, w http.ResponseWriter, err error) {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if pingErr := h.d.DockerX.Ping(pingCtx); dockerx.IsUnavailable(pingErr) {
		h.d.Log.Warn("[databases] volume create failed: Docker unavailable", slog.Any("error", err))
		httpx.Error(w, http.StatusServiceUnavailable, msgDockerUnavailable)
		return
	}
	h.d.Log.Error("[databases] volume create failed", slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, volumeErrorMessage(err))
}

// volumeErrorMessage is docker's stderr (dockerx.VolumeCreate returns it as
// the error text) or, when the CLI did not start, timed out or wrote nothing,
// the TS fallback message.
func volumeErrorMessage(err error) string {
	var startErr *execx.StartError
	var exitErr *execx.ExitError
	if errors.As(err, &startErr) || errors.As(err, &exitErr) || errors.Is(err, execx.ErrTimeout) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.TrimSpace(err.Error()) == "" {
		return "Failed to create Docker volume for database"
	}
	return strings.TrimSpace(err.Error())
}

// connectionInfo is the body of GET /:id/connection.
type connectionInfo struct {
	Engine              deploy.Engine `json:"engine"`
	Host                string        `json:"host"`
	Port                int           `json:"port"`
	Username            *string       `json:"username"`
	Database            string        `json:"database"`
	Password            string        `json:"password"`
	ConnectionURL       string        `json:"connection_url"`
	InternalOnly        bool          `json:"internal_only"`
	PublishHostPort     bool          `json:"publish_host_port"`
	HostPort            *int          `json:"host_port"`
	CredentialsInitOnly bool          `json:"credentials_init_only"`
	Note                string        `json:"note"`
}

// GET /:id/connection — host, credentials and connection URL of a managed
// database (credentials are read from its env; rotating means recreating).
func (h *handlers) connection(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to load connection info"
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	project, err := h.d.Store.GetProjectWithServices(ctx, store.ProjectFilter{ID: &id, ProjectType: store.Ptr("database")}, true)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, failMsg, err)
		return
	}
	loaded, ok, err := h.d.Deploy.LoadDatabaseCredentials(ctx, id)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if !ok || project == nil {
		httpx.Error(w, http.StatusNotFound, "Managed database not found or credentials missing")
		return
	}
	publish := project.PublishHostPort
	var hostPort *int
	if publish && len(project.Services) > 0 && project.Services[0].Port != nil && *project.Services[0].Port > 0 {
		hostPort = project.Services[0].Port
	}
	note := "Reachable from linked apps on the app Docker network via the host above. Prefer linking over publishing host ports."
	if hostPort != nil {
		note = fmt.Sprintf("Host port %d is published — this database may be reachable on the server IP. Prefer linking apps over sharing IP:port.", *hostPort)
	}
	var username *string
	if loaded.Creds.Username != "" {
		username = &loaded.Creds.Username
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, connectionInfo{
		Engine:              loaded.Engine,
		Host:                loaded.Host,
		Port:                loaded.Engine.Port,
		Username:            username,
		Database:            loaded.Creds.Database,
		Password:            loaded.Creds.Password,
		ConnectionURL:       deploy.ConnectionURLFor(loaded.Engine, loaded.Host, loaded.Creds),
		InternalOnly:        hostPort == nil,
		PublishHostPort:     publish,
		HostPort:            hostPort,
		CredentialsInitOnly: true,
		Note:                note,
	})
}

// findDatabase returns the managed database project with this id, or nil.
func (h *handlers) findDatabase(ctx context.Context, id string) (*store.Project, error) {
	p, err := h.d.Store.FindProject(ctx, store.ProjectFilter{ID: &id, ProjectType: store.Ptr("database")})
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return p, err
}

// GET /:id/links — the database's links, newest first, with the linked app
// and its services.
func (h *handlers) links(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to list links"
	ctx := r.Context()
	project, err := h.findDatabase(ctx, chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if project == nil {
		httpx.Error(w, http.StatusNotFound, "Database not found")
		return
	}
	links, err := h.d.Store.ListLinksForDatabase(ctx, project.ID)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, links)
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// linkCreated is the 201 body of POST /:id/links.
type linkCreated struct {
	Link            *store.DatabaseLink `json:"link"`
	EnvKey          string              `json:"env_key"`
	ServiceName     *string             `json:"service_name"`
	NetworkAttached bool                `json:"network_attached"`
	NetworkError    *string             `json:"network_error"`
	Note            string              `json:"note"`
}

// POST /:id/links — link a running managed database to an app project (or
// one of its services): own the env key, inject the connection URL as a
// runtime secret and attach the database to the app network. The app picks
// the env up on its next deploy; nothing is redeployed here.
func (h *handlers) createLink(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to link database"
	ctx := context.WithoutCancel(r.Context())
	dbProject, err := h.findDatabase(ctx, chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if dbProject == nil || dbProject.DBEngine == nil || *dbProject.DBEngine == "" {
		httpx.Error(w, http.StatusNotFound, "Database not found")
		return
	}
	engine, ok := deploy.GetEngine(*dbProject.DBEngine)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Unknown database engine")
		return
	}

	body := httpx.BodyFrom(r)
	appProjectID := ""
	if s, ok := body.Get("app_project_id").String(); ok {
		appProjectID = js.Trim(s)
	}
	if appProjectID == "" {
		httpx.Error(w, http.StatusBadRequest, "app_project_id is required")
		return
	}
	appProject, err := h.d.Store.GetProjectWithServices(ctx, store.ProjectFilter{ID: &appProjectID, ProjectTypeNot: store.Ptr("database")}, false)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "App project not found")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}

	serviceName := ""
	if s, ok := body.Get("service_name").String(); ok {
		serviceName = js.Trim(s)
	}
	if serviceName != "" && !hasService(appProject.Services, serviceName) {
		httpx.Error(w, http.StatusBadRequest, fmt.Sprintf(`Service "%s" not found on that project`, serviceName))
		return
	}

	envKey := engine.DefaultEnvKey
	if s, ok := body.Get("env_key").String(); ok && js.Trim(s) != "" {
		envKey = js.Trim(s)
	}
	if !envKeyRe.MatchString(envKey) || len(envKey) > 64 {
		httpx.Error(w, http.StatusBadRequest, "Invalid env_key")
		return
	}

	loaded, ok, err := h.d.Deploy.LoadDatabaseCredentials(ctx, dbProject.ID)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Database credentials missing — deploy the database first")
		return
	}
	if dbProject.Status == nil || (*dbProject.Status != "running" && *dbProject.Status != "degraded") {
		httpx.Error(w, http.StatusConflict, "Database must be running before linking. Deploy it first.")
		return
	}

	overwrite, _ := body.Get("overwrite").Bool()
	err = h.d.Deploy.AssertEnvKeyAvailable(ctx, deploy.EnvKeyRequest{
		AppProjectID: appProject.ID, ServiceName: serviceName, EnvKey: envKey,
		DatabaseProjectID: dbProject.ID, Overwrite: overwrite,
	})
	var conflict *deploy.LinkConflictError
	if errors.As(err, &conflict) {
		httpx.Error(w, conflict.Status, conflict.Message)
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}

	url := deploy.ConnectionURLFor(loaded.Engine, loaded.Host, loaded.Creds)

	// Link row first (ownership), then env — avoids an orphan env if the
	// create fails mid-way.
	link, err := h.d.Store.UpsertLink(ctx, store.LinkCreate{
		DatabaseProjectID: dbProject.ID, AppProjectID: appProject.ID, ServiceName: serviceName, EnvKey: envKey,
	})
	if errors.Is(err, store.ErrConflict) {
		httpx.Error(w, http.StatusConflict, "This link already exists")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if err := h.d.Deploy.UpsertLinkedEnv(ctx, deploy.LinkedEnv{
		AppProjectID: appProject.ID, ServiceName: serviceName, EnvKey: envKey, Value: url,
	}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.Error(w, http.StatusConflict, "This link already exists")
			return
		}
		h.fail(w, failMsg, err)
		return
	}

	out := linkCreated{Link: link, EnvKey: envKey, NetworkAttached: true,
		Note: "Connection URL injected as a runtime secret. Redeploy the app for containers to pick up the new env."}
	if serviceName != "" {
		out.ServiceName = &serviceName
	}
	if err := h.d.Deploy.EnsureDBOnAppNetwork(ctx, dbProject.ID, appProject.ID); err != nil {
		msg := err.Error()
		if dockerx.IsUnavailable(err) {
			msg = msgDockerUnavailable
		}
		h.d.Log.Warn("[databases] attach database to app network", "database", dbProject.ID, "app", appProject.ID, slog.Any("error", err))
		out.NetworkAttached = false
		out.NetworkError = &msg
		out.Note = "Connection URL saved. Network attach pending (" + msg + "). Redeploy the app (and ensure the database is running) to join networks."
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func hasService(services []*store.Service, name string) bool {
	for _, s := range services {
		if s.Name == name {
			return true
		}
	}
	return false
}

// DELETE /:id/links/:linkId — remove a link, its injected env (when no other
// link owns the key) and the network attachment (when no link between the
// two projects remains).
func (h *handlers) deleteLink(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to unlink database"
	ctx := context.WithoutCancel(r.Context())
	linkID, dbID := chi.URLParam(r, "linkId"), chi.URLParam(r, "id")
	link, err := h.d.Store.FindLink(ctx, store.LinkFilter{ID: &linkID, DatabaseProjectID: &dbID})
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Link not found")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if err := h.d.Deploy.UnlinkDatabaseLink(ctx, link.ID); err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "unlinked",
		"note":   "Env removed if no other database owns that key. Redeploy the app to drop it from running containers.",
	})
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error(msg, slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, msg)
}
