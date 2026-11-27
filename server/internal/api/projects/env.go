package projects

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"docklift/internal/deploy"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

const (
	msgInvalidEnvKey = "Invalid env key. Use letters, digits, underscore; must start with a letter or _."
	msgEnvNotFound   = "Environment variable not found"
)

// managedKeys returns the credential env keys of a managed database
// project, which only the database routes may set (none for other projects
// or a missing project).
func (h *handlers) managedKeys(r *http.Request, projectID string) ([]string, error) {
	p, err := h.d.Store.GetProject(r.Context(), projectID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if p.ProjectType == nil || *p.ProjectType != "database" || p.DBEngine == nil || *p.DBEngine == "" {
		return nil, nil
	}
	return deploy.ManagedCredentialEnvKeys(*p.DBEngine), nil
}

// checkScope answers 400 for a service scope that is not a service of the
// project; the shared scope always exists. ok is false when a response was
// written.
func (h *handlers) checkScope(w http.ResponseWriter, r *http.Request, projectID, scope, failMsg string) bool {
	if scope == deploy.SharedEnvService {
		return true
	}
	_, err := h.d.Store.FindService(r.Context(), store.ServiceFilter{ProjectID: &projectID, Name: &scope}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusBadRequest, "Unknown service: "+scope)
		return false
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return false
	}
	return true
}

// envValue is normalizeEnvValue(v). Non-strings are converted with String()
// here: deploy.NormalizeEnvValue does not know json.Number.
func envValue(v val) string {
	if v.nullish() {
		return ""
	}
	return deploy.NormalizeEnvValue(v.jsString(), true)
}

// envScope is normalizeEnvServiceName(v): null or undefined is the shared
// scope, anything else String(v).trim().
func envScope(v val) string {
	if v.nullish() {
		return deploy.SharedEnvService
	}
	return jsTrim(v.jsString())
}

// flag is `v ?? def` for a boolean column: undefined and null take def, a
// boolean is kept, anything else is a client error (Prisma rejected it).
func flag(w http.ResponseWriter, v val, field string, def bool) (*bool, bool) {
	if v.nullish() {
		return &def, true
	}
	b, ok := v.v.(bool)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, field+" must be a boolean")
		return nil, false
	}
	return &b, true
}

// GET /:id/env — the project's env variables ordered by service then key;
// ?service=name (trimmed; empty for shared) keeps one scope.
func (h *handlers) listEnv(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	f := store.EnvFilter{ProjectID: &id}
	if q := r.URL.Query()["service"]; len(q) == 1 {
		scope := jsTrim(q[0])
		f.ServiceName = &scope
	}
	rows, err := h.d.Store.ListEnvVariables(r.Context(), f, store.OrderServiceKey)
	if err != nil {
		h.fail(w, "Failed to get environment variables", err)
		return
	}
	httpx.JSON(w, http.StatusOK, rows)
}

// POST /:id/env — create one variable. Defaults: shared scope, not a build
// arg, available at runtime, not secret.
func (h *handlers) createEnv(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to create environment variable"
	id := param(r, "id")
	b := jsonFields(httpx.BodyFrom(r))
	key, isStr := b.get("key").str()
	if !isStr || !deploy.IsValidEnvKey(key) {
		httpx.Error(w, http.StatusBadRequest, msgInvalidEnvKey)
		return
	}
	locked, err := h.managedKeys(r, id)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if slices.Contains(locked, key) {
		httpx.Error(w, http.StatusBadRequest, "Managed database credentials are set at create time. Official images only apply them on first volume init — recreate the database to rotate.")
		return
	}
	scope := envScope(b.get("service_name"))
	if !h.checkScope(w, r, id, scope, failMsg) {
		return
	}
	buildArg, ok := flag(w, b.get("is_build_arg"), "is_build_arg", false)
	if !ok {
		return
	}
	runtime, ok := flag(w, b.get("is_runtime"), "is_runtime", true)
	if !ok {
		return
	}
	row, err := h.d.Store.CreateEnvVariable(r.Context(), store.EnvCreate{
		ProjectID: id, ServiceName: scope, Key: key, Value: envValue(b.get("value")),
		IsBuildArg: buildArg, IsRuntime: runtime, IsSecret: store.Ptr(b.get("is_secret").truthy()),
	})
	if errors.Is(err, store.ErrConflict) {
		httpx.Error(w, http.StatusConflict, "Environment variable key already exists for this service scope")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, row)
}

type envPair struct{ key, value string }

// parseDotenv reads KEY=VALUE lines: blank lines, comments and lines without
// '=' are skipped, the first occurrence of a key wins, values are
// normalized (trimmed, one pair of surrounding quotes removed). It returns
// the first invalid key, if any.
func parseDotenv(content string) ([]envPair, string, bool) {
	var out []envPair
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := jsTrim(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		k, v, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}
		key := jsTrim(k)
		if !deploy.IsValidEnvKey(key) {
			return nil, key, false
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, envPair{key, deploy.NormalizeEnvValue(v, true)})
	}
	return out, "", true
}

// POST /:id/env/bulk — import a .env text. Defaults: build arg and runtime.
// Variables are created one by one; a duplicate stops the import with 409
// and keeps those created before it.
func (h *handlers) bulkEnv(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to bulk import environment variables"
	id := param(r, "id")
	b := jsonFields(httpx.BodyFrom(r))
	scope := envScope(b.get("service_name"))
	content, isStr := b.get("content").str()
	if !isStr || content == "" {
		httpx.Error(w, http.StatusBadRequest, "Content is required")
		return
	}
	if !h.checkScope(w, r, id, scope, failMsg) {
		return
	}
	pairs, badKey, ok := parseDotenv(content)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid env key: "+badKey)
		return
	}
	if len(pairs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "No valid KEY=VALUE pairs found")
		return
	}
	locked, err := h.managedKeys(r, id)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	for _, p := range pairs {
		if slices.Contains(locked, p.key) {
			httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("Cannot import managed credential key %s. Recreate the database to rotate credentials.", p.key))
			return
		}
	}
	buildArg, ok := flag(w, b.get("is_build_arg"), "is_build_arg", true)
	if !ok {
		return
	}
	runtime, ok := flag(w, b.get("is_runtime"), "is_runtime", true)
	if !ok {
		return
	}
	created := 0
	for _, p := range pairs {
		_, err := h.d.Store.CreateEnvVariable(r.Context(), store.EnvCreate{
			ProjectID: id, ServiceName: scope, Key: p.key, Value: p.value, IsBuildArg: buildArg, IsRuntime: runtime,
		})
		if errors.Is(err, store.ErrConflict) {
			httpx.Error(w, http.StatusConflict, "Environment variable already exists: "+p.key)
			return
		}
		if err != nil {
			h.fail(w, failMsg, err)
			return
		}
		created++
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"count": created, "message": fmt.Sprintf("Added %d environment variable(s)", created)})
}

// DELETE /:id/env/:envId — delete one variable, unless it is a managed
// database credential or owned by a database link.
func (h *handlers) deleteEnv(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to delete environment variable"
	id, envID := param(r, "id"), param(r, "envId")
	existing, err := h.d.Store.FindEnvVariable(r.Context(), store.EnvFilter{ID: &envID, ProjectID: &id})
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgEnvNotFound)
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	locked, err := h.managedKeys(r, id)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if slices.Contains(locked, existing.Key) {
		httpx.Error(w, http.StatusBadRequest, "Cannot delete managed database credentials. Recreate the database to rotate passwords.")
		return
	}
	n, err := h.d.Store.CountLinks(r.Context(), store.LinkFilter{AppProjectID: &id, ServiceName: &existing.ServiceName, EnvKey: &existing.Key})
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if n > 0 {
		httpx.Error(w, http.StatusBadRequest, "This variable is owned by a managed database link. Unlink the database first.")
		return
	}
	if err := h.d.Store.DeleteEnvVariable(r.Context(), envID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, msgEnvNotFound)
			return
		}
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
