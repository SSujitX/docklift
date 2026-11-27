package projects

import (
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"

	"docklift/internal/deploy"
	"docklift/internal/execx"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

// storageNameRe is STORAGE_NAME_REGEX.
var storageNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$`)

// blockedMountPaths are host-sensitive container paths storage may not
// shadow (nor anything below them).
var blockedMountPaths = []string{"/proc", "/sys", "/dev", "/etc", "/var/run", "/var/run/docker.sock"}

var (
	errMountNotAbsolute = errors.New("Mount path must be an absolute container path")
	errMountBlocked     = errors.New("This system mount path is not allowed")
)

// validateMountPath is validateMountPath of projects.ts: the trimmed path
// without trailing slashes must be absolute, without ':' or NUL, and not
// "/" or a blocked system path.
func validateMountPath(v val) (string, error) {
	s := ""
	if v.truthy() {
		s = v.jsString()
	}
	mount := strings.TrimRight(jsTrim(s), "/")
	if mount == "" {
		mount = "/"
	}
	if !strings.HasPrefix(mount, "/") || strings.ContainsAny(mount, ":\x00") {
		return "", errMountNotAbsolute
	}
	if mount == "/" {
		return "", errMountBlocked
	}
	for _, b := range blockedMountPaths {
		if mount == b || strings.HasPrefix(mount, b+"/") {
			return "", errMountBlocked
		}
	}
	return mount, nil
}

// trimmedString is String(v || ”).trim().
func trimmedString(v val) string {
	if !v.truthy() {
		return ""
	}
	return jsTrim(v.jsString())
}

// GET /:id/build/detect — resolve Auto/Dockerfile/Railpack without building.
func (h *handlers) detect(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Store.GetProject(r.Context(), param(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err != nil {
		h.fail(w, "Failed to detect build type", err)
		return
	}
	dir := h.d.Deploy.ProjectPath(p.ID)
	if _, err := os.Stat(dir); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Project files not found")
		return
	}
	resolved, err := deploy.ResolveProjectBuild(dir, deploy.BuildSettings{
		BuildType: p.BuildType, BaseDirectory: p.BaseDirectory, DockerfilePath: p.DockerfilePath, InternalPort: &p.InternalPort,
	})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		RequestedType  string   `json:"requestedType"`
		ResolvedType   string   `json:"resolvedType"`
		BaseDirectory  string   `json:"baseDirectory"`
		DockerfilePath *string  `json:"dockerfilePath"`
		Detected       string   `json:"detected"`
		Manifests      []string `json:"manifests"`
	}{resolved.RequestedType, resolved.ResolvedType, resolved.BaseDirectory, resolved.DockerfilePath, resolved.Detected, resolved.Manifests})
}

// GET /:id/storage — the project's persistent volumes, oldest first.
func (h *handlers) listStorage(w http.ResponseWriter, r *http.Request) {
	vols, err := h.d.Store.ListVolumes(r.Context(), param(r, "id"), store.OrderCreatedAsc)
	if err != nil {
		h.fail(w, "Failed to list persistent storage", err)
		return
	}
	httpx.JSON(w, http.StatusOK, vols)
}

// POST /:id/storage — attach a named Docker volume to a service mount path.
// The row is created first and removed again if `docker volume create`
// fails.
func (h *handlers) createStorage(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Store.GetProject(r.Context(), param(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msgProjectNotFound)
		return
	}
	if err != nil {
		h.fail(w, "Failed to create persistent storage", err)
		return
	}
	b := jsonFields(httpx.BodyFrom(r))
	serviceName := trimmedString(b.get("service_name"))
	label := trimmedString(b.get("name"))
	if !storageNameRe.MatchString(label) {
		httpx.Error(w, http.StatusBadRequest, "Storage name must use 1-32 letters, numbers, _ or -")
		return
	}
	if _, err := h.d.Store.FindService(r.Context(), store.ServiceFilter{ProjectID: &p.ID, Name: &serviceName}, store.OrderNone); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusBadRequest, "Select a valid project service")
			return
		}
		h.fail(w, "Failed to create persistent storage", err)
		return
	}
	mount, err := validateMountPath(b.get("mount_path"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.dockerAvailable(w, r) {
		return
	}
	// The label's hash keeps "a-b" and "a_b" (same slug) apart.
	volumeName := deploy.PersistentVolumeName(p.ID, label)
	created, err := h.d.Store.CreateVolume(r.Context(), store.VolumeCreate{
		ProjectID: p.ID, ServiceName: serviceName, Name: volumeName, DisplayName: label, MountPath: mount,
	})
	if errors.Is(err, store.ErrConflict) {
		httpx.Error(w, http.StatusConflict, "That storage name or mount path already exists")
		return
	}
	if err != nil {
		h.fail(w, "Failed to create persistent storage", err)
		return
	}
	if err := h.d.DockerX.VolumeCreate(r.Context(), volumeName, deploy.VolumeLabels(p.Name, p.ID)...); err != nil {
		_ = h.d.Store.DeleteVolume(h.d.Background, created.ID)
		// VolumeCreate's error is docker's stderr when there is one; a bare
		// exit, start or timeout error means docker said nothing.
		msg := err.Error()
		var exit *execx.ExitError
		var start *execx.StartError
		if errors.As(err, &exit) || errors.As(err, &start) || errors.Is(err, execx.ErrTimeout) || msg == "" {
			msg = "Failed to create Docker volume"
		}
		h.d.Log.Warn("docker volume create failed", "volume", volumeName, "error", err)
		httpx.Error(w, http.StatusInternalServerError, msg)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// DELETE /:id/storage/:storageId?removeVolume=true — remove the Docker
// volume (its data is gone for good), then the row.
func (h *handlers) deleteStorage(w http.ResponseWriter, r *http.Request) {
	if q := r.URL.Query()["removeVolume"]; len(q) != 1 || q[0] != "true" {
		httpx.Error(w, http.StatusBadRequest, "Confirm permanent data deletion with removeVolume=true")
		return
	}
	vol, err := h.d.Store.GetVolume(r.Context(), param(r, "id"), param(r, "storageId"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Persistent storage not found")
		return
	}
	if err != nil {
		h.fail(w, "Failed to delete persistent storage", err)
		return
	}
	if !h.dockerAvailable(w, r) {
		return
	}
	if res, err := h.d.DockerX.VolumeRemove(r.Context(), vol.Name); err != nil {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = "Stop the project before deleting storage"
		}
		httpx.Error(w, http.StatusConflict, msg)
		return
	}
	if err := h.d.Store.DeleteVolume(r.Context(), vol.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, "Failed to delete persistent storage", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
