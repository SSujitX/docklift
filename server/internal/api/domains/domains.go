// Package domains serves /api/domains (the port of backend/src/routes/domains.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/domains.ts; until then Mount
// registers no routes and every request under /api/domains answers 404 after the
// group's authentication middleware, which the router applies.
package domains

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/domains routes on r, which is already scoped to /api/domains and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
