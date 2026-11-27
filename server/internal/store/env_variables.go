package store

import (
	"context"

	"docklift/internal/db"
)

// EnvVariable is a row of "env_variables". ServiceName "" is the shared scope.
type EnvVariable struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	ServiceName string   `json:"service_name"`
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	IsBuildArg  *bool    `json:"is_build_arg"`
	IsRuntime   *bool    `json:"is_runtime"`
	IsSecret    *bool    `json:"is_secret"`
	CreatedAt   *db.Time `json:"created_at"`
}

const envCols = `"id", "project_id", "service_name", "key", "value", "is_build_arg", "is_runtime", "is_secret", "created_at"`

func scanEnv(r rowScanner) (*EnvVariable, error) {
	var e EnvVariable
	if err := r.Scan(&e.ID, &e.ProjectID, &e.ServiceName, &e.Key, &e.Value, &e.IsBuildArg, &e.IsRuntime,
		&e.IsSecret, &e.CreatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

// EnvFilter selects env variables; set fields are AND-ed.
type EnvFilter struct {
	ID          *string
	IDIn        []string
	ProjectID   *string
	ServiceName *string
	Key         *string
}

func (f EnvFilter) where() where {
	var w where
	if f.ID != nil {
		w.eq(`"id"`, *f.ID)
	}
	if f.IDIn != nil {
		w.in(`"id"`, f.IDIn)
	}
	if f.ProjectID != nil {
		w.eq(`"project_id"`, *f.ProjectID)
	}
	if f.ServiceName != nil {
		w.eq(`"service_name"`, *f.ServiceName)
	}
	if f.Key != nil {
		w.eq(`"key"`, *f.Key)
	}
	return w
}

// FindEnvVariable returns the first env variable matching f.
func (s *Store) FindEnvVariable(ctx context.Context, f EnvFilter) (*EnvVariable, error) {
	w := f.where()
	return queryOne(ctx, s.reader(), "find env variable", `SELECT `+envCols+` FROM "env_variables"`+w.clause()+` LIMIT 1`, w.args, scanEnv)
}

// ListEnvVariables returns env variables matching f in the given order.
func (s *Store) ListEnvVariables(ctx context.Context, f EnvFilter, order Order) ([]*EnvVariable, error) {
	w := f.where()
	return queryList(ctx, s.reader(), "list env variables", `SELECT `+envCols+` FROM "env_variables"`+w.clause()+order.clause(), w.args, scanEnv)
}

// EnvCreate holds a new env variable's fields. IsSecret nil takes the schema
// default false; IsBuildArg/IsRuntime nil stay NULL.
type EnvCreate struct {
	ProjectID   string
	ServiceName string
	Key         string
	Value       string
	IsBuildArg  *bool
	IsRuntime   *bool
	IsSecret    *bool
}

// CreateEnvVariable inserts an env variable; a duplicate
// (project_id, service_name, key) is ErrConflict.
func (s *Store) CreateEnvVariable(ctx context.Context, in EnvCreate) (*EnvVariable, error) {
	now := db.Now()
	e := &EnvVariable{ID: db.NewID(), ProjectID: in.ProjectID, ServiceName: in.ServiceName, Key: in.Key,
		Value: in.Value, IsBuildArg: in.IsBuildArg, IsRuntime: in.IsRuntime, IsSecret: in.IsSecret, CreatedAt: &now}
	if e.IsSecret == nil {
		e.IsSecret = Ptr(false)
	}
	_, err := execAffected(ctx, s.writer(), "create env variable",
		`INSERT INTO "env_variables" (`+envCols+`) VALUES (`+placeholders(9)+`)`,
		e.ID, e.ProjectID, e.ServiceName, e.Key, e.Value, e.IsBuildArg, e.IsRuntime, e.IsSecret, e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// EnvUpdate lists the env variable columns to change.
type EnvUpdate struct {
	ServiceName Opt[string]
	Key         Opt[string]
	Value       Opt[string]
	IsBuildArg  Opt[*bool]
	IsRuntime   Opt[*bool]
	IsSecret    Opt[*bool]
}

// UpdateEnvVariable applies the set fields and returns the updated row.
func (s *Store) UpdateEnvVariable(ctx context.Context, id string, in EnvUpdate) (*EnvVariable, error) {
	var set sets
	setOpt(&set, `"service_name"`, in.ServiceName)
	setOpt(&set, `"key"`, in.Key)
	setOpt(&set, `"value"`, in.Value)
	setOpt(&set, `"is_build_arg"`, in.IsBuildArg)
	setOpt(&set, `"is_runtime"`, in.IsRuntime)
	setOpt(&set, `"is_secret"`, in.IsSecret)
	if len(set.cols) == 0 {
		return s.FindEnvVariable(ctx, EnvFilter{ID: &id})
	}
	return queryOne(ctx, s.writer(), "update env variable",
		`UPDATE "env_variables" SET `+set.clause()+` WHERE "id" = ? RETURNING `+envCols,
		append(set.args, id), scanEnv)
}

// DeleteEnvVariable deletes the env variable with this id.
func (s *Store) DeleteEnvVariable(ctx context.Context, id string) error {
	return execOne(ctx, s.writer(), "delete env variable", `DELETE FROM "env_variables" WHERE "id" = ?`, id)
}

// DeleteEnvVariables deletes every env variable matching f and returns the
// count (prisma.envVariable.deleteMany). An empty filter is refused.
func (s *Store) DeleteEnvVariables(ctx context.Context, f EnvFilter) (int64, error) {
	w := f.where()
	if len(w.parts) == 0 {
		return 0, errEmptyFilter("delete env variables")
	}
	return execAffected(ctx, s.writer(), "delete env variables", `DELETE FROM "env_variables"`+w.clause(), w.args...)
}

// DedupeEnvVariables keeps the newest row per (project_id, service_name, key)
// and returns how many duplicates were removed.
func (s *Store) DedupeEnvVariables(ctx context.Context) (int, error) {
	return db.DedupeEnvVariables(ctx, s.writer())
}
