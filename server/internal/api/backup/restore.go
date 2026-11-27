package backup

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	svc "docklift/internal/backup"
	"docklift/internal/httpx"
)

// Maintenance reasons shown by the gate while a restore runs.
const (
	reasonRestoreBackup = "Restoring from backup"
	reasonRestoreUpload = "Restoring from uploaded backup"
)

// msgRestoreInProgress answers a restore attempt while another one holds the
// lock. 2.1.6 returned the lock's internal label ("restore-upload");
// deliberate fix 7 returns a sentence.
const msgRestoreInProgress = "A restore is already in progress. Wait for it to finish before starting another one."

type lockedBody struct {
	Error  string `json:"error"`
	Locked bool   `json:"locked"`
}

// withRestoreLock is requireRestoreLock: the global restore lock is taken
// before any upload byte is read; a second restore gets 409.
func (h *handlers) withRestoreLock(why string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.d.Maintenance.TryAcquireRestoreLock(why) {
			msg := msgRestoreInProgress
			if h.d.Maintenance.Critical() {
				msg = h.d.Maintenance.RestoreLockReason()
			}
			drain(w, r, http.NewResponseController(w))
			httpx.JSON(w, http.StatusConflict, lockedBody{Error: msg, Locked: true})
			return
		}
		next(w, r)
	}
}

func (h *handlers) release() { h.d.Maintenance.ReleaseRestoreLock() }

// stepUp runs the step-up password check; on refusal (or a database error)
// it has answered, released the lock and returns false.
func (h *handlers) stepUp(w http.ResponseWriter, r *http.Request) bool {
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		h.release()
		httpx.InternalError(w, r, err)
		return false
	}
	if !ok {
		h.release()
	}
	return ok
}

// POST /restore/:filename — restore a server-created backup.
func (h *handlers) restoreFromBackup(w http.ResponseWriter, r *http.Request) {
	if !h.stepUp(w, r) {
		return
	}
	name := filenameParam(r)
	if !svc.ValidFilename(name) {
		h.release()
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	p, inside := contained(h.paths.BackupDir, name)
	if _, err := os.Stat(p); err != nil {
		h.release()
		httpx.Error(w, http.StatusNotFound, "Backup file not found")
		return
	}
	if !inside {
		h.release()
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	h.runRestore(w, svc.Request{Kind: svc.KindBackup, ArchivePath: p, Filename: name}, reasonRestoreBackup)
}

// POST /restore-from-upload/:filename — restore a previously uploaded file.
func (h *handlers) restoreFromUpload(w http.ResponseWriter, r *http.Request) {
	if !h.stepUp(w, r) {
		return
	}
	name := filenameParam(r)
	if !svc.ValidFilename(name) {
		h.release()
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	p, inside := contained(h.paths.UploadsDir(), name)
	st, err := os.Stat(p)
	if err != nil {
		h.release()
		httpx.Error(w, http.StatusNotFound, "Uploaded file not found")
		return
	}
	if !inside {
		h.release()
		httpx.Error(w, http.StatusBadRequest, "Invalid backup filename")
		return
	}
	h.runRestore(w, svc.Request{Kind: svc.KindStoredUpload, ArchivePath: p, Filename: name, Size: st.Size()}, reasonRestoreUpload)
}

// POST /restore-upload — upload and restore in one request. Authorised by
// the session (with the step-up password in the form) or, on a fresh
// install, by the setup token; the setup token and bootstrap secret are
// consumed only after a committed restore.
func (h *handlers) restoreUpload(w http.ResponseWriter, r *http.Request) {
	file, fields, uerr := h.readUpload(w, r)
	if uerr != nil {
		h.release()
		h.uploadFailed(w, uerr)
		return
	}
	discard := func() {
		if file != nil {
			_ = os.Remove(file.path)
		}
	}
	if fields != nil {
		r = withFields(r, fields)
	}
	if setupAuthorized(r) {
		// Re-checked after the upload (the router checked before it): an
		// admin may have registered meanwhile. A failed count refuses.
		admins, err := h.d.Store.CountUsersByRole(r.Context(), "admin")
		if err != nil {
			h.release()
			discard()
			httpx.InternalError(w, r, err)
			return
		}
		if admins > 0 {
			h.release()
			discard()
			httpx.Error(w, http.StatusForbidden, "Setup-token restore is only allowed before the first admin exists")
			return
		}
	} else if !h.stepUp(w, r) {
		discard()
		return
	}
	if file == nil {
		h.release()
		httpx.Error(w, http.StatusBadRequest, "No backup file uploaded")
		return
	}
	h.runRestore(w, svc.Request{
		Kind: svc.KindUpload, ArchivePath: file.path, Filename: file.name, Size: file.size, SetupAuth: setupAuthorized(r),
	}, reasonRestoreUpload)
}

// runRestore streams one restore (the lock is held) and applies its outcome
// to maintenance mode and the lock.
func (h *handlers) runRestore(w http.ResponseWriter, req svc.Request, reason string) {
	h.d.Maintenance.Enter(reason)
	p := startProgress(w)
	// The restore outlives the client connection (as in 2.1.6); only a
	// shutdown cancels it, and only before its point of no return.
	switch h.restorer.Run(h.d.Background, req, p.write) {
	case svc.OutcomeCommitted:
		// Maintenance and the lock stay on until the process restarts.
		time.AfterFunc(h.o.RestartDelay, h.o.Restart)
	case svc.OutcomeCritical:
		// The seal holds maintenance and the lock across restarts.
	default:
		h.d.Maintenance.Exit()
		h.release()
	}
}

// progress is the streamed text/plain log of create and restore.
type progress struct {
	w    http.ResponseWriter
	rc   *http.ResponseController
	dead bool
}

func startProgress(w http.ResponseWriter) *progress {
	hd := w.Header()
	hd.Set("Content-Type", "text/plain; charset=utf-8")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "no-cache, no-transform")
	w.WriteHeader(http.StatusOK)
	p := &progress{w: w, rc: http.NewResponseController(w)}
	_ = p.rc.Flush()
	return p
}

// write sends one log chunk; a gone client is ignored (the work goes on).
func (p *progress) write(s string) {
	if p.dead {
		return
	}
	if _, err := io.WriteString(p.w, s); err != nil {
		p.dead = true
		return
	}
	_ = p.rc.Flush()
}

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }
