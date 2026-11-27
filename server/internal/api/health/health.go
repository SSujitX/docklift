// Package health serves GET /api/health (public): status, current time,
// release version and the per-process instance id the frontend uses to detect
// restarts.
package health

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/db"
	"docklift/internal/httpx"
	"docklift/internal/version"
)

type body struct {
	Status     string `json:"status"`
	Timestamp  string `json:"timestamp"`
	Version    string `json:"version"`
	InstanceID string `json:"instanceId"`
}

// Mount registers GET /health on r (scoped to /api).
func Mount(r chi.Router, d *app.Deps) {
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, body{
			Status:     "ok",
			Timestamp:  db.FormatISO(time.Now()),
			Version:    version.Version,
			InstanceID: d.InstanceID,
		})
	})
}
