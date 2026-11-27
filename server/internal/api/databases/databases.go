// Package databases serves /api/databases (the port of backend/src/routes/databases.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/databases.ts; until then Mount
// registers no routes and every request under /api/databases answers 404 after the
// group's authentication middleware, which the router applies.
package databases

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/databases routes on r, which is already scoped to /api/databases and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
