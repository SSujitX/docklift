// Package deployments serves /api/deployments (the port of backend/src/routes/deployments.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/deployments.ts; until then Mount
// registers no routes and every request under /api/deployments answers 404 after the
// group's authentication middleware, which the router applies.
package deployments

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/deployments routes on r, which is already scoped to /api/deployments and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
