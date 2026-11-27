package deploy

import (
	"context"
	"errors"
	"fmt"

	"docklift/internal/store"
)

// LoadedDatabase is a managed database's engine, credentials and Docker DNS
// host (its first service's container name).
type LoadedDatabase struct {
	Engine Engine
	Creds  Credentials
	Host   string
}

// LoadDatabaseCredentials reads a managed database project's engine,
// credentials and host (loadDatabaseCredentials). ok=false when the project
// is missing, not a database, has an unknown engine, incomplete credentials
// or no container yet.
func (r *Runtime) LoadDatabaseCredentials(ctx context.Context, databaseProjectID string) (LoadedDatabase, bool, error) {
	p, err := r.Store.GetProject(ctx, databaseProjectID)
	if errors.Is(err, store.ErrNotFound) {
		return LoadedDatabase{}, false, nil
	}
	if err != nil {
		return LoadedDatabase{}, false, err
	}
	if p.ProjectType == nil || *p.ProjectType != "database" || p.DBEngine == nil || *p.DBEngine == "" {
		return LoadedDatabase{}, false, nil
	}
	engine, ok := GetEngine(*p.DBEngine)
	if !ok {
		return LoadedDatabase{}, false, nil
	}
	rows, err := r.Store.ListEnvVariables(ctx, store.EnvFilter{ProjectID: &p.ID}, store.OrderNone)
	if err != nil {
		return LoadedDatabase{}, false, err
	}
	env := make(map[string]string, len(rows))
	for _, v := range rows {
		env[v.Key] = v.Value
	}
	creds, ok := CredentialsFromEnv(engine, env)
	if !ok {
		return LoadedDatabase{}, false, nil
	}
	svc, err := r.firstService(ctx, p.ID)
	if err != nil || svc == nil || svc.ContainerName == nil || *svc.ContainerName == "" {
		return LoadedDatabase{}, false, err
	}
	return LoadedDatabase{Engine: engine, Creds: creds, Host: *svc.ContainerName}, true, nil
}

// firstService is the oldest service of a project (nil when none).
func (r *Runtime) firstService(ctx context.Context, projectID string) (*store.Service, error) {
	svc, err := r.Store.FindService(ctx, store.ServiceFilter{ProjectID: &projectID}, store.OrderCreatedAsc)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return svc, err
}

// ErrDatabaseNotReady is returned when a managed database has no container yet.
var ErrDatabaseNotReady = errors.New("Database container is not ready yet — deploy the database first")

// EnsureDBOnAppNetwork attaches the database container to the app's project
// network.
func (r *Runtime) EnsureDBOnAppNetwork(ctx context.Context, databaseProjectID, appProjectID string) error {
	svc, err := r.firstService(ctx, databaseProjectID)
	if err != nil {
		return err
	}
	if svc == nil || svc.ContainerName == nil || *svc.ContainerName == "" {
		return ErrDatabaseNotReady
	}
	return r.ConnectContainerToProjectNetwork(ctx, appProjectID, *svc.ContainerName)
}

// MaybeDisconnectDBFromAppNetwork detaches the database container from the
// app network once no link between the two remains.
func (r *Runtime) MaybeDisconnectDBFromAppNetwork(ctx context.Context, databaseProjectID, appProjectID string) error {
	n, err := r.Store.CountLinks(ctx, store.LinkFilter{DatabaseProjectID: &databaseProjectID, AppProjectID: &appProjectID})
	if err != nil || n > 0 {
		return err
	}
	svc, err := r.firstService(ctx, databaseProjectID)
	if err != nil || svc == nil || svc.ContainerName == nil || *svc.ContainerName == "" {
		return err
	}
	r.DisconnectContainerFromProjectNetwork(ctx, appProjectID, *svc.ContainerName)
	return nil
}

// LinkConflictError is an env-key ownership conflict; Status is the HTTP
// status to answer (409) and Error() the message.
type LinkConflictError struct {
	Status  int
	Message string
}

func (e *LinkConflictError) Error() string { return e.Message }

// EnvKeyRequest identifies an env key on an app project/service scope that
// a database link wants to own.
type EnvKeyRequest struct {
	AppProjectID      string
	ServiceName       string
	EnvKey            string
	DatabaseProjectID string
	Overwrite         bool
}

// AssertEnvKeyAvailable enforces one link per (app, service, env key): a
// key owned by another database, or an existing unlinked env value without
// Overwrite, is a *LinkConflictError (assertEnvKeyAvailable).
func (r *Runtime) AssertEnvKeyAvailable(ctx context.Context, q EnvKeyRequest) error {
	other, err := r.Store.FindLinkOwningEnvKey(ctx, q.AppProjectID, q.ServiceName, q.EnvKey, q.DatabaseProjectID)
	if err == nil {
		return &LinkConflictError{Status: 409, Message: fmt.Sprintf(`"%s" on this project/service is already linked to database "%s". Unlink that first.`, q.EnvKey, other.DatabaseProjectName)}
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = r.Store.FindEnvVariable(ctx, store.EnvFilter{ProjectID: &q.AppProjectID, ServiceName: &q.ServiceName, Key: &q.EnvKey})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = r.Store.FindLink(ctx, store.LinkFilter{DatabaseProjectID: &q.DatabaseProjectID, AppProjectID: &q.AppProjectID, ServiceName: &q.ServiceName, EnvKey: &q.EnvKey})
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if !q.Overwrite {
		return &LinkConflictError{Status: 409, Message: fmt.Sprintf(`"%s" already exists on this project/service. Pass overwrite=true to replace it with the managed database URL.`, q.EnvKey)}
	}
	return nil
}

// DisconnectLinkedDatabasesFromApp detaches every linked database container
// from the app network (before compose down).
func (r *Runtime) DisconnectLinkedDatabasesFromApp(ctx context.Context, appProjectID string) error {
	dbIDs, err := r.linkedIDs(ctx, store.LinkFilter{AppProjectID: &appProjectID}, linkDatabase)
	if err != nil {
		return err
	}
	for _, dbID := range dbIDs {
		svc, err := r.firstService(ctx, dbID)
		if err != nil {
			return err
		}
		if svc == nil || svc.ContainerName == nil || *svc.ContainerName == "" {
			continue
		}
		r.DisconnectContainerFromProjectNetwork(ctx, appProjectID, *svc.ContainerName)
	}
	return nil
}

// linkedIDs returns pick(link) for the links matching f, deduplicated in
// first-seen order.
func (r *Runtime) linkedIDs(ctx context.Context, f store.LinkFilter, pick func(*store.DatabaseLink) string) ([]string, error) {
	links, err := r.Store.ListLinks(ctx, f, store.OrderNone)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, l := range links {
		if id := pick(l); !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// LinkedEnv is a connection URL env var injected into an app.
type LinkedEnv struct {
	AppProjectID string
	ServiceName  string
	EnvKey       string
	Value        string
}

// UpsertLinkedEnv writes the connection URL as a runtime secret env var on
// the app (scoped or shared), creating or updating it.
func (r *Runtime) UpsertLinkedEnv(ctx context.Context, e LinkedEnv) error {
	t, f := true, false
	existing, err := r.Store.FindEnvVariable(ctx, store.EnvFilter{ProjectID: &e.AppProjectID, ServiceName: &e.ServiceName, Key: &e.EnvKey})
	if err == nil {
		_, err = r.Store.UpdateEnvVariable(ctx, existing.ID, store.EnvUpdate{
			Value: store.Some(e.Value), IsRuntime: store.Some(&t), IsBuildArg: store.Some(&f), IsSecret: store.Some(&t),
		})
		return err
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = r.Store.CreateEnvVariable(ctx, store.EnvCreate{
		ProjectID: e.AppProjectID, ServiceName: e.ServiceName, Key: e.EnvKey, Value: e.Value,
		IsRuntime: &t, IsBuildArg: &f, IsSecret: &t,
	})
	return err
}

// DeleteLinkedEnvIfUnowned removes the injected env var unless a remaining
// link (of any database) still owns that app/service/key; it reports
// whether it deleted.
func (r *Runtime) DeleteLinkedEnvIfUnowned(ctx context.Context, appProjectID, serviceName, envKey string) (bool, error) {
	n, err := r.Store.CountLinks(ctx, store.LinkFilter{AppProjectID: &appProjectID, ServiceName: &serviceName, EnvKey: &envKey})
	if err != nil || n > 0 {
		return false, err
	}
	if _, err := r.Store.DeleteEnvVariables(ctx, store.EnvFilter{ProjectID: &appProjectID, ServiceName: &serviceName, Key: &envKey}); err != nil {
		return false, err
	}
	return true, nil
}

// UnlinkDatabaseLink deletes one link, then its env var when unowned, then
// the network attachment when no link between the projects remains. A
// missing link is not an error.
func (r *Runtime) UnlinkDatabaseLink(ctx context.Context, linkID string) error {
	link, err := r.Store.FindLink(ctx, store.LinkFilter{ID: &linkID})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.dropLink(ctx, link)
}

func (r *Runtime) dropLink(ctx context.Context, link *store.DatabaseLink) error {
	if err := r.Store.DeleteLink(ctx, link.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if _, err := r.DeleteLinkedEnvIfUnowned(ctx, link.AppProjectID, link.ServiceName, link.EnvKey); err != nil {
		return err
	}
	return r.MaybeDisconnectDBFromAppNetwork(ctx, link.DatabaseProjectID, link.AppProjectID)
}

// CleanupLinksForDatabaseProject removes every link of a managed database
// (before deleting it), cleaning the peer apps' env and networks.
func (r *Runtime) CleanupLinksForDatabaseProject(ctx context.Context, databaseProjectID string) error {
	links, err := r.Store.ListLinks(ctx, store.LinkFilter{DatabaseProjectID: &databaseProjectID}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := r.dropLink(ctx, l); err != nil {
			return err
		}
	}
	return nil
}

// RefreshLinkedEnvForDatabase rewrites the connection URL on every app
// linked to the database (after a redeploy).
func (r *Runtime) RefreshLinkedEnvForDatabase(ctx context.Context, databaseProjectID string) error {
	loaded, ok, err := r.LoadDatabaseCredentials(ctx, databaseProjectID)
	if err != nil || !ok {
		return err
	}
	url := ConnectionURLFor(loaded.Engine, loaded.Host, loaded.Creds)
	links, err := r.Store.ListLinks(ctx, store.LinkFilter{DatabaseProjectID: &databaseProjectID}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := r.UpsertLinkedEnv(ctx, LinkedEnv{AppProjectID: l.AppProjectID, ServiceName: l.ServiceName, EnvKey: l.EnvKey, Value: url}); err != nil {
			return err
		}
	}
	return nil
}

// ReapplyDatabaseLinksForDatabase refreshes injected URLs and re-attaches the
// database to every linked app network (after the database deployed);
// attach failures are logged, not returned.
func (r *Runtime) ReapplyDatabaseLinksForDatabase(ctx context.Context, databaseProjectID string) error {
	if err := r.RefreshLinkedEnvForDatabase(ctx, databaseProjectID); err != nil {
		return err
	}
	appIDs, err := r.linkedIDs(ctx, store.LinkFilter{DatabaseProjectID: &databaseProjectID}, linkApp)
	if err != nil {
		return err
	}
	for _, appID := range appIDs {
		if err := r.EnsureDBOnAppNetwork(ctx, databaseProjectID, appID); err != nil {
			r.Log.Warn("[databaseLinks] failed to attach database to app", "database", databaseProjectID, "app", appID, "error", err)
		}
	}
	return nil
}

// ReapplyDatabaseLinksForApp re-attaches every linked database to the app
// network and reports how many attaches succeeded and failed (logged); err
// is a failure to read the links.
func (r *Runtime) ReapplyDatabaseLinksForApp(ctx context.Context, appProjectID string) (ok, failed int, err error) {
	dbIDs, err := r.linkedIDs(ctx, store.LinkFilter{AppProjectID: &appProjectID}, linkDatabase)
	if err != nil {
		return 0, 0, err
	}
	for _, dbID := range dbIDs {
		if err := r.EnsureDBOnAppNetwork(ctx, dbID, appProjectID); err != nil {
			failed++
			r.Log.Warn("[databaseLinks] failed to attach database to app", "database", dbID, "app", appProjectID, "error", err)
			continue
		}
		ok++
	}
	return ok, failed, nil
}

func linkDatabase(l *store.DatabaseLink) string { return l.DatabaseProjectID }

func linkApp(l *store.DatabaseLink) string { return l.AppProjectID }
