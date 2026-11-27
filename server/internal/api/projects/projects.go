// Package projects serves /api/projects (the port of backend/src/routes/projects.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/projects.ts; until then Mount
// registers no routes and every request under /api/projects answers 404 after the
// group's authentication middleware, which the router applies.
package projects

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/projects routes on r, which is already scoped to /api/projects and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
