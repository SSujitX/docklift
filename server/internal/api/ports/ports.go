// Package ports serves /api/ports (the port of backend/src/routes/ports.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/ports.ts; until then Mount
// registers no routes and every request under /api/ports answers 404 after the
// group's authentication middleware, which the router applies.
package ports

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/ports routes on r, which is already scoped to /api/ports and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
