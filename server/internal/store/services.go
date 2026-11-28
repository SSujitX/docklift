package store

import (
	"context"

	"docklift/internal/db"
)

// Service is a row of "services".
type Service struct {
	ID             string   `json:"id"`
	ProjectID      string   `json:"project_id"`
	Name           string   `json:"name"`
	DockerfilePath string   `json:"dockerfile_path"`
	ContainerName  *string  `json:"container_name"`
	Port           *int     `json:"port"`
	InternalPort   *int     `json:"internal_port"`
	Domain         *string  `json:"domain"`
	Status         *string  `json:"status"`
	CreatedAt      *db.Time `json:"created_at"`
}

const serviceCols = `"id", "project_id", "name", "dockerfile_path", "container_name", "port", "internal_port", "domain", "status", "created_at"`

func scanService(r rowScanner) (*Service, error) {
	var s Service
	if err := r.Scan(&s.ID, &s.ProjectID, &s.Name, &s.DockerfilePath, &s.ContainerName, &s.Port, &s.InternalPort,
		&s.Domain, &s.Status, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// ServiceFilter selects services; set fields are AND-ed.
type ServiceFilter struct {
	ID            *string
	ProjectID     *string
	Name          *string
	NameNotIn     []string
	ContainerName *string
	Port          *int
	// HasDomain / HasContainerName filter on IS NOT NULL (true) or IS NULL (false).
	HasDomain        *bool
	HasContainerName *bool
	StatusIn         []string
}

func (f ServiceFilter) where() where {
	var w where
	if f.ID != nil {
		w.eq(`"id"`, *f.ID)
	}
	if f.ProjectID != nil {
		w.eq(`"project_id"`, *f.ProjectID)
	}
	if f.Name != nil {
		w.eq(`"name"`, *f.Name)
	}
	if f.NameNotIn != nil {
		w.notIn(`"name"`, f.NameNotIn)
	}
	if f.ContainerName != nil {
		w.eq(`"container_name"`, *f.ContainerName)
	}
	if f.Port != nil {
		w.eq(`"port"`, *f.Port)
	}
	if f.HasDomain != nil {
		w.null(`"domain"`, !*f.HasDomain)
	}
	if f.HasContainerName != nil {
		w.null(`"container_name"`, !*f.HasContainerName)
	}
	if f.StatusIn != nil {
		w.in(`"status"`, f.StatusIn)
	}
	return w
}

// GetService returns the service with this id.
func (s *Store) GetService(ctx context.Context, id string) (*Service, error) {
	return queryOne(ctx, s.reader(), "get service", `SELECT `+serviceCols+` FROM "services" WHERE "id" = ?`, []any{id}, scanService)
}

// FindService returns the first service matching f in the given order.
func (s *Store) FindService(ctx context.Context, f ServiceFilter, order Order) (*Service, error) {
	w := f.where()
	return queryOne(ctx, s.reader(), "find service", `SELECT `+serviceCols+` FROM "services"`+w.clause()+order.clause()+` LIMIT 1`, w.args, scanService)
}

// ListServices returns services matching f in the given order.
func (s *Store) ListServices(ctx context.Context, f ServiceFilter, order Order) ([]*Service, error) {
	w := f.where()
	return queryList(ctx, s.reader(), "list services", `SELECT `+serviceCols+` FROM "services"`+w.clause()+order.clause(), w.args, scanService)
}

// ServiceCreate holds a new service's fields.
type ServiceCreate struct {
	ProjectID      string
	Name           string
	DockerfilePath string
	ContainerName  *string
	Port           *int
	InternalPort   *int
	Domain         *string
	Status         *string
}

// CreateService inserts a service and returns the stored row.
func (s *Store) CreateService(ctx context.Context, in ServiceCreate) (*Service, error) {
	now := db.Now()
	svc := &Service{ID: db.NewID(), ProjectID: in.ProjectID, Name: in.Name, DockerfilePath: in.DockerfilePath,
		ContainerName: in.ContainerName, Port: in.Port, InternalPort: in.InternalPort, Domain: in.Domain,
		Status: in.Status, CreatedAt: &now}
	_, err := execAffected(ctx, s.writer(), "create service",
		`INSERT INTO "services" (`+serviceCols+`) VALUES (`+placeholders(10)+`)`,
		svc.ID, svc.ProjectID, svc.Name, svc.DockerfilePath, svc.ContainerName, svc.Port, svc.InternalPort,
		svc.Domain, svc.Status, svc.CreatedAt)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// ServiceUpdate lists the service columns to change.
type ServiceUpdate struct {
	Name           Opt[string]
	DockerfilePath Opt[string]
	ContainerName  Opt[*string]
	Port           Opt[*int]
	InternalPort   Opt[*int]
	Domain         Opt[*string]
	Status         Opt[*string]
}

func (in ServiceUpdate) sets() sets {
	var set sets
	setOpt(&set, `"name"`, in.Name)
	setOpt(&set, `"dockerfile_path"`, in.DockerfilePath)
	setOpt(&set, `"container_name"`, in.ContainerName)
	setOpt(&set, `"port"`, in.Port)
	setOpt(&set, `"internal_port"`, in.InternalPort)
	setOpt(&set, `"domain"`, in.Domain)
	setOpt(&set, `"status"`, in.Status)
	return set
}

// UpdateService applies the set fields and returns the updated row.
func (s *Store) UpdateService(ctx context.Context, id string, in ServiceUpdate) (*Service, error) {
	set := in.sets()
	if len(set.cols) == 0 {
		return s.GetService(ctx, id)
	}
	return queryOne(ctx, s.writer(), "update service",
		`UPDATE "services" SET `+set.clause()+` WHERE "id" = ? RETURNING `+serviceCols,
		append(set.args, id), scanService)
}

// UpdateServices applies the set fields to every service matching f and
// returns how many rows changed (prisma.service.updateMany).
func (s *Store) UpdateServices(ctx context.Context, f ServiceFilter, in ServiceUpdate) (int64, error) {
	set := in.sets()
	if len(set.cols) == 0 {
		return 0, nil
	}
	w := f.where()
	return execAffected(ctx, s.writer(), "update services",
		`UPDATE "services" SET `+set.clause()+w.clause(), append(set.args, w.args...)...)
}

// DeleteService deletes the service with this id.
func (s *Store) DeleteService(ctx context.Context, id string) error {
	return execOne(ctx, s.writer(), "delete service", `DELETE FROM "services" WHERE "id" = ?`, id)
}
