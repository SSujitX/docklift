// Package backup serves /api/backup (the port of backend/src/routes/backup.ts):
// list, create (streamed progress), download, delete and upload of backup
// archives, the three restore entry points, and the restore-critical
// operator endpoints. The archive and restore logic lives in
// internal/backup.
package backup

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/auth"
	svc "docklift/internal/backup"
	"docklift/internal/httpx"
	"docklift/internal/pathsec"
)

// Options are the injectable collaborators of the route group. Zero values
// take the production implementations.
type Options struct {
	// Docker runs the restore's container commands (default: docker CLI).
	Docker svc.Docker
	// Nginx reloads nginx-proxy at the end of reconciliation (default d.Edge.Nginx).
	Nginx svc.Reloader
	// Restart ends the process after a committed restore so the container
	// restarts on the restored data (default: graceful self-termination).
	Restart func()
	// RestartDelay is how long after the response the restart happens (1 s).
	RestartDelay time.Duration
	// Now is the clock.
	Now func() time.Time
}

// restartDelay is the 1 s setTimeout before process.exit in backup.ts.
const restartDelay = time.Second

// Mount registers the /api/backup routes on r, which is already scoped to
// /api/backup and wrapped in the group's authentication middleware (session
// JWT, or the setup token for POST /restore-upload on a fresh install).
func Mount(r chi.Router, d *app.Deps) { MountWith(r, d, Options{}) }

// MountWith is Mount with explicit collaborators (tests).
func MountWith(r chi.Router, d *app.Deps, o Options) {
	paths := svc.PathsFrom(d.Config)
	if o.Docker == nil {
		o.Docker = svc.DockerCLI{X: d.DockerX, Runtime: d.Deploy, ProxyContainer: d.Config.NginxProxyContainer}
	}
	if o.Nginx == nil {
		o.Nginx = d.Edge.Nginx
	}
	if o.Restart == nil {
		o.Restart = func() { restartProcess(d.Log) }
	}
	if o.RestartDelay == 0 {
		o.RestartDelay = restartDelay
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	h := &handlers{
		d:     d,
		o:     o,
		paths: paths,
		creator: &svc.Creator{
			Paths: paths, DB: d.DB, Log: d.Log, Now: o.Now,
		},
		restorer: &svc.Restorer{
			Paths: paths, DB: d.DB, Store: d.Store, State: d.Maintenance,
			Docker: o.Docker, Nginx: o.Nginx, Setup: d.Auth.Bootstrap(), Log: d.Log, Now: o.Now,
		},
	}

	// Before the listener opens: seal an instance whose last restore was
	// interrupted, and remove work files of crashed backups/restores.
	d.Lifecycle.On(app.PhaseDockerNetwork, "backup-restore-recovery", func(context.Context) error {
		svc.RecoverInterrupted(paths, d.Maintenance, d.Log)
		svc.CleanupOnBoot(paths, d.Log)
		svc.TightenModes(paths, d.Log)
		return nil
	})

	r.Get("/", h.list)
	r.Get("/uploads", h.listUploads)
	r.Delete("/uploads/{filename}", h.deleteUpload)
	r.Post("/create", h.create)
	r.Post("/restore/{filename}", h.withRestoreLock("restore-from-backup", h.restoreFromBackup))
	r.Delete("/{filename}", h.deleteBackup)
	r.Get("/download/{filename}", h.download)
	r.Post("/upload", h.upload)
	r.Post("/restore-upload", h.withRestoreLock("restore-upload", h.restoreUpload))
	r.Post("/restore-from-upload/{filename}", h.withRestoreLock("restore-from-upload", h.restoreFromUpload))
	r.Post("/clear-critical-restore", h.clearCritical)
	r.Get("/critical-status", h.criticalStatus)
}

type handlers struct {
	d        *app.Deps
	o        Options
	paths    svc.Paths
	creator  *svc.Creator
	restorer *svc.Restorer
}

func (h *handlers) now() time.Time { return h.o.Now() }

// filenameParam is req.params.filename: decoded once, like Express.
func filenameParam(r *http.Request) string {
	v := chi.URLParam(r, "filename")
	if r.URL.RawPath != "" {
		if decoded, err := url.PathUnescape(v); err == nil {
			v = decoded
		}
	}
	return v
}

// contained resolves name inside dir and reports whether its real path
// (links followed) stays inside dir's real path. A path that does not exist
// passes, so the caller answers with its own "not found".
func contained(dir, name string) (string, bool) {
	p := filepath.Join(dir, name)
	real, err := pathsec.RealPath(p)
	if err != nil {
		return p, true
	}
	realDir, err := pathsec.RealPath(dir)
	if err != nil {
		return p, false
	}
	return p, real != realDir && pathsec.IsInside(realDir, real)
}

// fileEntry is one row of the backup and upload lists.
type fileEntry struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
	modified  time.Time
}

// listZips lists the *.zip entries of dir (newest first), skipping "skip".
func listZips(dir, skip string) ([]fileEntry, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []fileEntry{}
	for _, e := range entries {
		name := e.Name()
		if name == skip || !strings.HasSuffix(name, ".zip") {
			continue
		}
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		mod := st.ModTime().UTC().Truncate(time.Millisecond)
		out = append(out, fileEntry{Filename: name, Size: st.Size(), CreatedAt: svc.ISO(mod), modified: mod})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].modified.After(out[j].modified) })
	return out, nil
}

// GET / — server-created backups (the uploads directory is excluded).
func (h *handlers) list(w http.ResponseWriter, _ *http.Request) {
	list, err := listZips(h.paths.BackupDir, "uploads")
	if err != nil {
		h.d.Log.Error("List backups error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to list backups")
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

// GET /uploads — uploaded restore files.
func (h *handlers) listUploads(w http.ResponseWriter, _ *http.Request) {
	list, err := listZips(h.paths.UploadsDir(), "")
	if err != nil {
		h.d.Log.Error("List uploads error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to list uploaded files")
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

type messageBody struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// removeFile is fsp.access + fsp.unlink: errNotFound when missing; a
// directory cannot be unlinked.
func removeFile(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return errors.New("is a directory")
	}
	return os.Remove(p)
}

// DELETE /uploads/:filename
func (h *handlers) deleteUpload(w http.ResponseWriter, r *http.Request) {
	name := filenameParam(r)
	if !svc.ValidFilename(name) {
		httpx.Error(w, http.StatusBadRequest, "Invalid filename")
		return
	}
	p, ok := contained(h.paths.UploadsDir(), name)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid filename")
		return
	}
	if err := removeFile(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			httpx.Error(w, http.StatusNotFound, "File not found")
			return
		}
		h.d.Log.Error("Delete upload error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to delete file")
		return
	}
	httpx.JSON(w, http.StatusOK, messageBody{Success: true, Message: "File deleted successfully"})
}

// DELETE /:filename
func (h *handlers) deleteBackup(w http.ResponseWriter, r *http.Request) {
	name := filenameParam(r)
	if !svc.ValidFilename(name) {
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	p, ok := contained(h.paths.BackupDir, name)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	if err := removeFile(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			httpx.Error(w, http.StatusNotFound, "Backup not found")
			return
		}
		h.d.Log.Error("Delete backup error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to delete backup")
		return
	}
	httpx.JSON(w, http.StatusOK, messageBody{Success: true, Message: "Backup deleted successfully"})
}

// GET /download/:filename — the archive, streamed from disk.
func (h *handlers) download(w http.ResponseWriter, r *http.Request) {
	name := filenameParam(r)
	if !svc.ValidFilename(name) {
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	p, ok := contained(h.paths.BackupDir, name)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			httpx.Error(w, http.StatusNotFound, "Backup not found")
			return
		}
		h.d.Log.Error("Download backup error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to download backup")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		h.d.Log.Error("Download stream error", "path", p, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to download backup")
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "application/zip")
	hd.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	hd.Set("Content-Length", formatInt(st.Size()))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		h.d.Log.Warn("Download stream error", "error", err)
	}
}

// POST /create — streamed backup log.
func (h *handlers) create(w http.ResponseWriter, r *http.Request) {
	label, nameErr := backupLabel(httpx.BodyFrom(r).Get("name"))
	p := startProgress(w)
	if nameErr != nil {
		svc.LogFailure(p.write, nameErr)
		return
	}
	// Like 2.1.6, a client disconnect does not stop the backup.
	h.creator.Create(h.d.Background, label, p.write)
}

// backupLabel is `name ? name.replace(...).slice(0, 50) : 'docklift'`.
func backupLabel(v httpx.Value) (string, error) {
	if !v.Truthy() {
		return svc.DefaultLabel, nil
	}
	s, ok := v.String()
	if !ok {
		return "", errors.New("name must be a string")
	}
	return svc.SanitizeLabel(s), nil
}

// POST /upload — store an uploaded backup without restoring it.
func (h *handlers) upload(w http.ResponseWriter, r *http.Request) {
	file, _, uerr := h.readUpload(w, r)
	if uerr != nil {
		h.uploadFailed(w, uerr)
		return
	}
	if file == nil {
		httpx.Error(w, http.StatusBadRequest, "No backup file uploaded")
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Success  bool   `json:"success"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Message  string `json:"message"`
	}{true, file.name, file.size, "Backup uploaded successfully"})
}

// uploadFailed answers a rejected multipart body. 2.1.6 sent multer errors
// of POST /upload to the generic 500 handler; client mistakes are 400 here
// (deliberate fix 1).
func (h *handlers) uploadFailed(w http.ResponseWriter, uerr *uploadError) {
	if uerr.status >= 500 {
		h.d.Log.Error("Upload backup error", "error", uerr)
	}
	httpx.Error(w, uerr.status, uerr.msg)
}

// critical-status / clear-critical-restore

type criticalStatusBody struct {
	Critical bool `json:"critical"`
	Seal     any  `json:"seal"`
}

// GET /critical-status
func (h *handlers) criticalStatus(w http.ResponseWriter, _ *http.Request) {
	var seal any
	if p := h.d.Maintenance.ReadCritical(); p != nil {
		seal = p
	}
	httpx.JSON(w, http.StatusOK, criticalStatusBody{Critical: h.d.Maintenance.Critical(), Seal: seal})
}

// POST /clear-critical-restore — operator acknowledgement after manual repair.
func (h *handlers) clearCritical(w http.ResponseWriter, r *http.Request) {
	if !h.d.Maintenance.Critical() {
		httpx.Error(w, http.StatusBadRequest, "No critical restore seal is active")
		return
	}
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !ok {
		return
	}
	prev, err := h.d.Maintenance.ClearCritical()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	var previous any
	if prev != nil {
		previous = prev
	}
	httpx.JSON(w, http.StatusOK, struct {
		Success  bool   `json:"success"`
		Message  string `json:"message"`
		Previous any    `json:"previous"`
	}{true, "Critical restore seal cleared", previous})
}

// setupAuthorized reports whether the request came in on the setup token.
func setupAuthorized(r *http.Request) bool { return auth.SetupTokenAuthorized(r.Context()) }
