package store

import (
	"context"
	"errors"

	"docklift/internal/db"
)

// DatabaseLink is a row of "database_links": a managed database project
// linked to an app project (ServiceName "" = every service of the app).
type DatabaseLink struct {
	ID                string   `json:"id"`
	DatabaseProjectID string   `json:"database_project_id"`
	AppProjectID      string   `json:"app_project_id"`
	ServiceName       string   `json:"service_name"`
	EnvKey            string   `json:"env_key"`
	CreatedAt         *db.Time `json:"created_at"`
}

const linkCols = `"id", "database_project_id", "app_project_id", "service_name", "env_key", "created_at"`

func scanLink(r rowScanner) (*DatabaseLink, error) {
	var l DatabaseLink
	if err := r.Scan(&l.ID, &l.DatabaseProjectID, &l.AppProjectID, &l.ServiceName, &l.EnvKey, &l.CreatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

// LinkFilter selects database links; set fields are AND-ed.
type LinkFilter struct {
	ID                   *string
	DatabaseProjectID    *string
	DatabaseProjectIDNot *string
	AppProjectID         *string
	ServiceName          *string
	EnvKey               *string
}

func (f LinkFilter) where() where {
	var w where
	if f.ID != nil {
		w.eq(`"id"`, *f.ID)
	}
	if f.DatabaseProjectID != nil {
		w.eq(`"database_project_id"`, *f.DatabaseProjectID)
	}
	if f.DatabaseProjectIDNot != nil {
		w.ne(`"database_project_id"`, *f.DatabaseProjectIDNot)
	}
	if f.AppProjectID != nil {
		w.eq(`"app_project_id"`, *f.AppProjectID)
	}
	if f.ServiceName != nil {
		w.eq(`"service_name"`, *f.ServiceName)
	}
	if f.EnvKey != nil {
		w.eq(`"env_key"`, *f.EnvKey)
	}
	return w
}

// CountLinks counts links matching f.
func (s *Store) CountLinks(ctx context.Context, f LinkFilter) (int, error) {
	return count(ctx, s.reader(), "count database links", `"database_links"`, f.where())
}

// FindLink returns the first link matching f.
func (s *Store) FindLink(ctx context.Context, f LinkFilter) (*DatabaseLink, error) {
	w := f.where()
	return queryOne(ctx, s.reader(), "find database link", `SELECT `+linkCols+` FROM "database_links"`+w.clause()+` LIMIT 1`, w.args, scanLink)
}

// ListLinks returns links matching f in the given order.
func (s *Store) ListLinks(ctx context.Context, f LinkFilter, order Order) ([]*DatabaseLink, error) {
	w := f.where()
	return queryList(ctx, s.reader(), "list database links", `SELECT `+linkCols+` FROM "database_links"`+w.clause()+order.clause(), w.args, scanLink)
}

// LinkConflict is a link that already owns an env key, with the owning
// database project's name (include database_project: {select: {name}}).
type LinkConflict struct {
	DatabaseLink
	DatabaseProjectName string
}

// FindLinkOwningEnvKey returns the link of another database project that owns
// (app, service, envKey), if any.
func (s *Store) FindLinkOwningEnvKey(ctx context.Context, appProjectID, serviceName, envKey, exceptDatabaseProjectID string) (*LinkConflict, error) {
	return queryOne(ctx, s.reader(), "find conflicting database link",
		`SELECT l."id", l."database_project_id", l."app_project_id", l."service_name", l."env_key", l."created_at", p."name"
		 FROM "database_links" l JOIN "projects" p ON p."id" = l."database_project_id"
		 WHERE l."app_project_id" = ? AND l."service_name" = ? AND l."env_key" = ? AND l."database_project_id" <> ? LIMIT 1`,
		[]any{appProjectID, serviceName, envKey, exceptDatabaseProjectID},
		func(r rowScanner) (*LinkConflict, error) {
			var c LinkConflict
			if err := r.Scan(&c.ID, &c.DatabaseProjectID, &c.AppProjectID, &c.ServiceName, &c.EnvKey, &c.CreatedAt, &c.DatabaseProjectName); err != nil {
				return nil, err
			}
			return &c, nil
		})
}

// LinkCreate identifies a link by its (database, app, service, env key) unique key.
type LinkCreate struct {
	DatabaseProjectID string
	AppProjectID      string
	ServiceName       string
	EnvKey            string
}

// UpsertLink returns the link with this exact unique key, creating it if
// absent (prisma upsert with an empty update). A clash on the other unique
// key (app, service, env key) is ErrConflict.
func (s *Store) UpsertLink(ctx context.Context, in LinkCreate) (*DatabaseLink, error) {
	var out *DatabaseLink
	err := s.InTx(ctx, func(tx *Store) error {
		existing, err := queryOne(ctx, tx.writer(), "find database link",
			`SELECT `+linkCols+` FROM "database_links" WHERE "database_project_id" = ? AND "app_project_id" = ? AND "service_name" = ? AND "env_key" = ?`,
			[]any{in.DatabaseProjectID, in.AppProjectID, in.ServiceName, in.EnvKey}, scanLink)
		if err == nil {
			out = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		now := db.Now()
		l := &DatabaseLink{ID: db.NewID(), DatabaseProjectID: in.DatabaseProjectID, AppProjectID: in.AppProjectID,
			ServiceName: in.ServiceName, EnvKey: in.EnvKey, CreatedAt: &now}
		if _, err := execAffected(ctx, tx.writer(), "create database link",
			`INSERT INTO "database_links" (`+linkCols+`) VALUES (?, ?, ?, ?, ?, ?)`,
			l.ID, l.DatabaseProjectID, l.AppProjectID, l.ServiceName, l.EnvKey, l.CreatedAt); err != nil {
			return err
		}
		out = l
		return nil
	})
	return out, err
}

// DeleteLink deletes the link with this id.
func (s *Store) DeleteLink(ctx context.Context, id string) error {
	return execOne(ctx, s.writer(), "delete database link", `DELETE FROM "database_links" WHERE "id" = ?`, id)
}

// DatabaseRef is the {id, name, status, db_engine} projection of a linked
// database project.
type DatabaseRef struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Status   *string `json:"status"`
	DBEngine *string `json:"db_engine"`
}

// DatabaseLinkWithDatabase is a link with database_project included.
type DatabaseLinkWithDatabase struct {
	DatabaseLink
	DatabaseProject DatabaseRef `json:"database_project"`
}

// ListLinksForApp returns an app project's links, newest first, with the
// linked database project's {id, name, status, db_engine}.
func (s *Store) ListLinksForApp(ctx context.Context, appProjectID string) ([]*DatabaseLinkWithDatabase, error) {
	return queryList(ctx, s.reader(), "list app database links",
		`SELECT l."id", l."database_project_id", l."app_project_id", l."service_name", l."env_key", l."created_at",
		        p."id", p."name", p."status", p."db_engine"
		 FROM "database_links" l JOIN "projects" p ON p."id" = l."database_project_id"
		 WHERE l."app_project_id" = ? ORDER BY l."created_at" DESC`,
		[]any{appProjectID},
		func(r rowScanner) (*DatabaseLinkWithDatabase, error) {
			var o DatabaseLinkWithDatabase
			if err := r.Scan(&o.ID, &o.DatabaseProjectID, &o.AppProjectID, &o.ServiceName, &o.EnvKey, &o.CreatedAt,
				&o.DatabaseProject.ID, &o.DatabaseProject.Name, &o.DatabaseProject.Status, &o.DatabaseProject.DBEngine); err != nil {
				return nil, err
			}
			return &o, nil
		})
}

// DatabaseLinkWithApp is a link with app_project's {id, name, status} included.
type DatabaseLinkWithApp struct {
	DatabaseLink
	AppProject ProjectRef `json:"app_project"`
}

func (s *Store) listLinksWithApp(ctx context.Context, databaseProjectID string) ([]*DatabaseLinkWithApp, error) {
	return queryList(ctx, s.reader(), "list database links with app",
		`SELECT l."id", l."database_project_id", l."app_project_id", l."service_name", l."env_key", l."created_at",
		        p."id", p."name", p."status"
		 FROM "database_links" l JOIN "projects" p ON p."id" = l."app_project_id"
		 WHERE l."database_project_id" = ?`,
		[]any{databaseProjectID},
		func(r rowScanner) (*DatabaseLinkWithApp, error) {
			var o DatabaseLinkWithApp
			if err := r.Scan(&o.ID, &o.DatabaseProjectID, &o.AppProjectID, &o.ServiceName, &o.EnvKey, &o.CreatedAt,
				&o.AppProject.ID, &o.AppProject.Name, &o.AppProject.Status); err != nil {
				return nil, err
			}
			return &o, nil
		})
}

// ServiceRef is the {id, name, status} projection of a service.
type ServiceRef struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Status *string `json:"status"`
}

// AppRefWithServices is a linked app project's {id, name, status, services}.
type AppRefWithServices struct {
	ProjectRef
	Services []*ServiceRef `json:"services"`
}

// DatabaseLinkWithAppServices is a link with app_project and its services included.
type DatabaseLinkWithAppServices struct {
	DatabaseLink
	AppProject AppRefWithServices `json:"app_project"`
}

// ListLinksForDatabase returns a database project's links, newest first, with
// the linked app's {id, name, status, services[{id, name, status}]}.
func (s *Store) ListLinksForDatabase(ctx context.Context, databaseProjectID string) ([]*DatabaseLinkWithAppServices, error) {
	links, err := queryList(ctx, s.reader(), "list database links",
		`SELECT l."id", l."database_project_id", l."app_project_id", l."service_name", l."env_key", l."created_at",
		        p."id", p."name", p."status"
		 FROM "database_links" l JOIN "projects" p ON p."id" = l."app_project_id"
		 WHERE l."database_project_id" = ? ORDER BY l."created_at" DESC`,
		[]any{databaseProjectID},
		func(r rowScanner) (*DatabaseLinkWithAppServices, error) {
			var o DatabaseLinkWithAppServices
			if err := r.Scan(&o.ID, &o.DatabaseProjectID, &o.AppProjectID, &o.ServiceName, &o.EnvKey, &o.CreatedAt,
				&o.AppProject.ID, &o.AppProject.Name, &o.AppProject.Status); err != nil {
				return nil, err
			}
			return &o, nil
		})
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		services, err := s.ListServices(ctx, ServiceFilter{ProjectID: &l.AppProjectID}, OrderNone)
		if err != nil {
			return nil, err
		}
		l.AppProject.Services = make([]*ServiceRef, 0, len(services))
		for _, svc := range services {
			l.AppProject.Services = append(l.AppProject.Services, &ServiceRef{ID: svc.ID, Name: svc.Name, Status: svc.Status})
		}
	}
	return links, nil
}
