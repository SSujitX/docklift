package router

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"docklift/internal/api/openapi"
	"docklift/internal/app/apptest"
)

// notChiRoutes are table entries served outside the chi tree: the terminal
// upgrade is a root middleware.
var notChiRoutes = map[string]bool{"GET /ws/terminal": true}

// chiRoutes walks the router and returns "METHOD /path" for every endpoint
// except the dashboard fallback.
func chiRoutes(t *testing.T, h http.Handler) map[string]bool {
	t.Helper()
	routes, ok := h.(chi.Routes)
	if !ok {
		t.Fatalf("router is %T, not chi.Routes", h)
	}
	out := map[string]bool{}
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.ReplaceAll(route, "/*/", "/")
		if strings.HasSuffix(route, "/*") || route == "/*" {
			return nil // mount stubs and the SPA fallback
		}
		if len(route) > 1 {
			route = strings.TrimSuffix(route, "/")
		}
		out[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestOpenAPITableMatchesRouter fails when a route is registered without an
// entry in openapi.Routes, or the table lists a route that does not exist.
func TestOpenAPITableMatchesRouter(t *testing.T) {
	d := apptest.NewDeps(t)
	live := chiRoutes(t, New(d))
	table := map[string]bool{}
	for _, r := range openapi.Routes {
		key := r.Method + " " + r.Path
		if table[key] {
			t.Errorf("duplicate table entry %s", key)
		}
		table[key] = true
	}
	var missing, extra []string
	for k := range live {
		if !table[k] {
			missing = append(missing, k)
		}
	}
	for k := range table {
		if !live[k] && !notChiRoutes[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	for _, k := range missing {
		t.Errorf("route %s is not in openapi.Routes", k)
	}
	for _, k := range extra {
		t.Errorf("openapi.Routes lists %s, which the router does not serve", k)
	}
	t.Logf("router serves %d routes, table lists %d", len(live), len(table))
	if len(live) < 100 {
		t.Fatalf("walked only %d routes; the walk is broken", len(live))
	}
}
