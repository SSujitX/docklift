package httpx

import (
	"context"
	"net/http"
	"strings"
)

// GateState is what the maintenance gate needs to know (implemented by
// *maintenance.State).
type GateState interface {
	Critical() bool
	Active() bool
	Reason() string
	CriticalMarkerPath() string
}

type criticalBody struct {
	Error       string `json:"error"`
	Maintenance bool   `json:"maintenance"`
	Critical    bool   `json:"critical"`
	SealPath    string `json:"sealPath"`
}

type maintenanceBody struct {
	Error       string `json:"error"`
	Maintenance bool   `json:"maintenance"`
}

// MaintenanceGate is the restore gate. While the restore-critical
// seal exists only /api/health and the two operator recovery endpoints pass;
// during plain maintenance /api/health and the active restore stream
// (/api/backup/restore*) pass. Everything else gets 503.
func MaintenanceGate(state GateState) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := RoutePath(r)
			if state.Critical() {
				if p == "/api/health" || p == "/api/backup/clear-critical-restore" || p == "/api/backup/critical-status" {
					next.ServeHTTP(w, r)
					return
				}
				JSON(w, http.StatusServiceUnavailable, criticalBody{
					Error: state.Reason(), Maintenance: true, Critical: true, SealPath: state.CriticalMarkerPath(),
				})
				return
			}
			if !state.Active() || p == "/api/health" || strings.HasPrefix(p, "/api/backup/restore") {
				next.ServeHTTP(w, r)
				return
			}
			JSON(w, http.StatusServiceUnavailable, maintenanceBody{Error: state.Reason(), Maintenance: true})
		})
	}
}

// RoutePath is the request path as routing sees it: the escaped form when it
// differs from the decoded one (chi matches on it), like 2.x.
func RoutePath(r *http.Request) string {
	if r.URL.RawPath != "" {
		return r.URL.RawPath
	}
	return r.URL.Path
}

type headKey struct{}

// HeadAsGet serves HEAD requests with the GET handlers, as 2.x routes did;
// net/http discards the body of HEAD responses. OriginalMethod still reports
// HEAD for middleware that branched on req.method in 2.x.
func HeadAsGet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			r = r.Clone(context.WithValue(r.Context(), headKey{}, true))
			r.Method = http.MethodGet
		}
		next.ServeHTTP(w, r)
	})
}

// OriginalMethod is the method the client sent (before HeadAsGet).
func OriginalMethod(r *http.Request) string {
	if head, _ := r.Context().Value(headKey{}).(bool); head {
		return http.MethodHead
	}
	return r.Method
}
