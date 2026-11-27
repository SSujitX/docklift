// Package system serves /api/system (the port of backend/src/routes/system.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/system.ts; until then Mount
// registers no routes and every request under /api/system answers 404 after the
// group's authentication middleware, which the router applies.
package system

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/system routes on r, which is already scoped to /api/system and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
