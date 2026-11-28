package store

import (
	"context"

	"docklift/internal/db"
)

// Project is a row of "projects".
type Project struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     *string  `json:"description"`
	SourceType      *string  `json:"source_type"`
	ProjectType     *string  `json:"project_type"`
	GithubURL       *string  `json:"github_url"`
	GithubBranch    *string  `json:"github_branch"`
	Domain          *string  `json:"domain"`
	Port            *int     `json:"port"`
	Status          *string  `json:"status"`
	ContainerName   *string  `json:"container_name"`
	AutoDeploy      *bool    `json:"auto_deploy"`
	WebhookSecret   *string  `json:"webhook_secret"`
	BuildType       string   `json:"build_type"`
	BaseDirectory   string   `json:"base_directory"`
	DockerfilePath  *string  `json:"dockerfile_path"`
	InternalPort    int      `json:"internal_port"`
	PublishHostPort bool     `json:"publish_host_port"`
	DBEngine        *string  `json:"db_engine"`
	CreatedAt       *db.Time `json:"created_at"`
	UpdatedAt       *db.Time `json:"updated_at"`
}

const projectCols = `"id", "name", "description", "source_type", "project_type", "github_url", "github_branch", "domain", "port", "status", "container_name", "auto_deploy", "webhook_secret", "build_type", "base_directory", "dockerfile_path", "internal_port", "publish_host_port", "db_engine", "created_at", "updated_at"`

func scanProject(r rowScanner) (*Project, error) {
	var p Project
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.SourceType, &p.ProjectType, &p.GithubURL, &p.GithubBranch,
		&p.Domain, &p.Port, &p.Status, &p.ContainerName, &p.AutoDeploy, &p.WebhookSecret, &p.BuildType,
		&p.BaseDirectory, &p.DockerfilePath, &p.InternalPort, &p.PublishHostPort, &p.DBEngine, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ProjectFilter selects projects; zero fields are ignored and set fields are
// AND-ed, like a Prisma `where`.
type ProjectFilter struct {
	ID             *string
	ProjectType    *string
	ProjectTypeNot *string // Prisma `not`; also excludes NULL project_type
	SourceType     *string
	AutoDeploy     *bool
	StatusIn       []string
	// GithubURLIn matches any of these github_url values (Prisma OR of equals).
	GithubURLIn []string
}

func (f ProjectFilter) where() where {
	var w where
	if f.ID != nil {
		w.eq(`"id"`, *f.ID)
	}
	if f.ProjectType != nil {
		w.eq(`"project_type"`, *f.ProjectType)
	}
	if f.ProjectTypeNot != nil {
		w.ne(`"project_type"`, *f.ProjectTypeNot)
	}
	if f.SourceType != nil {
		w.eq(`"source_type"`, *f.SourceType)
	}
	if f.AutoDeploy != nil {
		w.eq(`"auto_deploy"`, *f.AutoDeploy)
	}
	if f.StatusIn != nil {
		w.in(`"status"`, f.StatusIn)
	}
	if f.GithubURLIn != nil {
		w.in(`"github_url"`, f.GithubURLIn)
	}
	return w
}

// GetProject returns the project with this id.
func (s *Store) GetProject(ctx context.Context, id string) (*Project, error) {
	return queryOne(ctx, s.reader(), "get project", `SELECT `+projectCols+` FROM "projects" WHERE "id" = ?`, []any{id}, scanProject)
}

// FindProject returns the first project matching f.
func (s *Store) FindProject(ctx context.Context, f ProjectFilter) (*Project, error) {
	w := f.where()
	return queryOne(ctx, s.reader(), "find project", `SELECT `+projectCols+` FROM "projects"`+w.clause()+` LIMIT 1`, w.args, scanProject)
}

// ListProjects returns the projects matching f in the given order.
func (s *Store) ListProjects(ctx context.Context, f ProjectFilter, order Order) ([]*Project, error) {
	w := f.where()
	return queryList(ctx, s.reader(), "list projects", `SELECT `+projectCols+` FROM "projects"`+w.clause()+order.clause(), w.args, scanProject)
}

// ProjectCreate holds a new project's fields. Nil pointers for columns with a
// schema default take that default (auto_deploy false, build_type "auto",
// base_directory ".", internal_port 3000, publish_host_port false).
type ProjectCreate struct {
	Name            string
	Description     *string
	SourceType      *string
	ProjectType     *string
	GithubURL       *string
	GithubBranch    *string
	Domain          *string
	Port            *int
	Status          *string
	ContainerName   *string
	AutoDeploy      *bool
	WebhookSecret   *string
	BuildType       *string
	BaseDirectory   *string
	DockerfilePath  *string
	InternalPort    *int
	PublishHostPort *bool
	DBEngine        *string
}

// CreateProject inserts a project and returns the stored row.
func (s *Store) CreateProject(ctx context.Context, in ProjectCreate) (*Project, error) {
	now := db.Now()
	p := &Project{
		ID: db.NewID(), Name: in.Name, Description: in.Description, SourceType: in.SourceType,
		ProjectType: in.ProjectType, GithubURL: in.GithubURL, GithubBranch: in.GithubBranch, Domain: in.Domain,
		Port: in.Port, Status: in.Status, ContainerName: in.ContainerName, AutoDeploy: in.AutoDeploy,
		WebhookSecret: in.WebhookSecret, BuildType: "auto", BaseDirectory: ".", DockerfilePath: in.DockerfilePath,
		InternalPort: 3000, DBEngine: in.DBEngine, CreatedAt: &now, UpdatedAt: &now,
	}
	if p.AutoDeploy == nil {
		p.AutoDeploy = Ptr(false)
	}
	if in.BuildType != nil {
		p.BuildType = *in.BuildType
	}
	if in.BaseDirectory != nil {
		p.BaseDirectory = *in.BaseDirectory
	}
	if in.InternalPort != nil {
		p.InternalPort = *in.InternalPort
	}
	if in.PublishHostPort != nil {
		p.PublishHostPort = *in.PublishHostPort
	}
	_, err := execAffected(ctx, s.writer(), "create project",
		`INSERT INTO "projects" (`+projectCols+`) VALUES (`+placeholders(21)+`)`,
		p.ID, p.Name, p.Description, p.SourceType, p.ProjectType, p.GithubURL, p.GithubBranch, p.Domain, p.Port,
		p.Status, p.ContainerName, p.AutoDeploy, p.WebhookSecret, p.BuildType, p.BaseDirectory, p.DockerfilePath,
		p.InternalPort, p.PublishHostPort, p.DBEngine, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ProjectUpdate lists the project columns to change. updated_at is always
// bumped (Prisma @updatedAt).
type ProjectUpdate struct {
	Name            Opt[string]
	Description     Opt[*string]
	SourceType      Opt[*string]
	ProjectType     Opt[*string]
	GithubURL       Opt[*string]
	GithubBranch    Opt[*string]
	Domain          Opt[*string]
	Port            Opt[*int]
	Status          Opt[*string]
	ContainerName   Opt[*string]
	AutoDeploy      Opt[*bool]
	WebhookSecret   Opt[*string]
	BuildType       Opt[string]
	BaseDirectory   Opt[string]
	DockerfilePath  Opt[*string]
	InternalPort    Opt[int]
	PublishHostPort Opt[bool]
	DBEngine        Opt[*string]
}

// UpdateProject applies the set fields and returns the updated row.
func (s *Store) UpdateProject(ctx context.Context, id string, in ProjectUpdate) (*Project, error) {
	var set sets
	setOpt(&set, `"name"`, in.Name)
	setOpt(&set, `"description"`, in.Description)
	setOpt(&set, `"source_type"`, in.SourceType)
	setOpt(&set, `"project_type"`, in.ProjectType)
	setOpt(&set, `"github_url"`, in.GithubURL)
	setOpt(&set, `"github_branch"`, in.GithubBranch)
	setOpt(&set, `"domain"`, in.Domain)
	setOpt(&set, `"port"`, in.Port)
	setOpt(&set, `"status"`, in.Status)
	setOpt(&set, `"container_name"`, in.ContainerName)
	setOpt(&set, `"auto_deploy"`, in.AutoDeploy)
	setOpt(&set, `"webhook_secret"`, in.WebhookSecret)
	setOpt(&set, `"build_type"`, in.BuildType)
	setOpt(&set, `"base_directory"`, in.BaseDirectory)
	setOpt(&set, `"dockerfile_path"`, in.DockerfilePath)
	setOpt(&set, `"internal_port"`, in.InternalPort)
	setOpt(&set, `"publish_host_port"`, in.PublishHostPort)
	setOpt(&set, `"db_engine"`, in.DBEngine)
	set.add(`"updated_at"`, db.Now())
	return queryOne(ctx, s.writer(), "update project",
		`UPDATE "projects" SET `+set.clause()+` WHERE "id" = ? RETURNING `+projectCols,
		append(set.args, id), scanProject)
}

// DeleteProject deletes a project; foreign keys cascade to its deployments,
// services, env variables, ports, volumes and database links.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	return execOne(ctx, s.writer(), "delete project", `DELETE FROM "projects" WHERE "id" = ?`, id)
}

// ProjectRef is the {id, name, status} projection used by relation includes.
type ProjectRef struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Status *string `json:"status"`
}

// ProjectWithServices is a project with all of its services included.
type ProjectWithServices struct {
	Project
	Services []*Service `json:"services"`
}

// GetProjectWithServices returns the project matching f (first match) with
// its services (`include: { services: true }`), or only the first service
// when firstServiceOnly is set (`services: { take: 1 }`).
func (s *Store) GetProjectWithServices(ctx context.Context, f ProjectFilter, firstServiceOnly bool) (*ProjectWithServices, error) {
	p, err := s.FindProject(ctx, f)
	if err != nil {
		return nil, err
	}
	services, err := s.ListServices(ctx, ServiceFilter{ProjectID: &p.ID}, OrderNone)
	if err != nil {
		return nil, err
	}
	if firstServiceOnly && len(services) > 1 {
		services = services[:1]
	}
	return &ProjectWithServices{Project: *p, Services: services}, nil
}

// DatabaseProject is a managed database project as listed by GET
// /api/databases: all project columns, its services, and its links with the
// linked app's {id, name, status}.
type DatabaseProject struct {
	Project
	Services          []*Service             `json:"services"`
	DatabaseLinksAsDb []*DatabaseLinkWithApp `json:"databaseLinksAsDb"`
}

// ListDatabaseProjects returns projects with project_type "database", newest
// first, with services and databaseLinksAsDb included.
func (s *Store) ListDatabaseProjects(ctx context.Context) ([]*DatabaseProject, error) {
	projects, err := s.ListProjects(ctx, ProjectFilter{ProjectType: Ptr("database")}, OrderCreatedDesc)
	if err != nil {
		return nil, err
	}
	out := make([]*DatabaseProject, 0, len(projects))
	for _, p := range projects {
		services, err := s.ListServices(ctx, ServiceFilter{ProjectID: &p.ID}, OrderNone)
		if err != nil {
			return nil, err
		}
		links, err := s.listLinksWithApp(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, &DatabaseProject{Project: *p, Services: services, DatabaseLinksAsDb: links})
	}
	return out, nil
}

// ActiveProjectService is the {id, port, name} service projection of
// ListActiveProjectsForPorts.
type ActiveProjectService struct {
	ID   string `json:"id"`
	Port *int   `json:"port"`
	Name string `json:"name"`
}

// ActiveProject is the projection GET /api/ports builds its usage view from.
type ActiveProject struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	Status          *string                 `json:"status"`
	ProjectType     *string                 `json:"project_type"`
	DBEngine        *string                 `json:"db_engine"`
	PublishHostPort bool                    `json:"publish_host_port"`
	Services        []*ActiveProjectService `json:"services"`
}

// ListActiveProjectsForPorts returns projects whose status is running,
// degraded, building or pending, most recently updated first, with their
// services' {id, port, name}.
func (s *Store) ListActiveProjectsForPorts(ctx context.Context) ([]*ActiveProject, error) {
	projects, err := s.ListProjects(ctx, ProjectFilter{StatusIn: []string{"running", "degraded", "building", "pending"}}, OrderUpdatedDesc)
	if err != nil {
		return nil, err
	}
	out := make([]*ActiveProject, 0, len(projects))
	for _, p := range projects {
		services, err := s.ListServices(ctx, ServiceFilter{ProjectID: &p.ID}, OrderNone)
		if err != nil {
			return nil, err
		}
		ap := &ActiveProject{ID: p.ID, Name: p.Name, Status: p.Status, ProjectType: p.ProjectType, DBEngine: p.DBEngine,
			PublishHostPort: p.PublishHostPort, Services: make([]*ActiveProjectService, 0, len(services))}
		for _, svc := range services {
			ap.Services = append(ap.Services, &ActiveProjectService{ID: svc.ID, Port: svc.Port, Name: svc.Name})
		}
		out = append(out, ap)
	}
	return out, nil
}
