// Package github serves /api/github (the port of backend/src/routes/github.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/github.ts; until then Mount
// registers no routes and every request under /api/github answers 404 after the
// group's authentication middleware, which the router applies.
package github

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/github routes on r, which is already scoped to /api/github and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
