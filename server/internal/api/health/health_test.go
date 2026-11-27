package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/go-chi/chi/v5"

	"docklift/internal/api/health"
	"docklift/internal/app/apptest"
	"docklift/internal/version"
)

func TestHealth(t *testing.T) {
	d := apptest.NewDeps(t)
	r := chi.NewRouter()
	health.Mount(r, d)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || body["status"] != "ok" || body["version"] != version.Version || body["instanceId"] != d.InstanceID || len(body) != 4 {
		t.Errorf("health = %d %v", rec.Code, body)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`).MatchString(body["timestamp"]) {
		t.Errorf("timestamp %q is not toISOString format", body["timestamp"])
	}
}
