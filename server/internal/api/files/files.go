// Package files serves /api/files (the port of backend/src/routes/files.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/files.ts; until then Mount
// registers no routes and every request under /api/files answers 404 after the
// group's authentication middleware, which the router applies.
package files

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/files routes on r, which is already scoped to /api/files and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
