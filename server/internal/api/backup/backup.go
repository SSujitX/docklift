// Package backup serves /api/backup (the port of backend/src/routes/backup.ts).
//
// The route handlers are owned by the agent porting backend/src/routes/backup.ts; until then Mount
// registers no routes and every request under /api/backup answers 404 after the
// group's authentication middleware, which the router applies.
package backup

import (
	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
)

// Mount registers the /api/backup routes on r, which is already scoped to /api/backup and
// wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {}
