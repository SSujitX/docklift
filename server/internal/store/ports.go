package store

import (
	"context"
)

// Port is a row of "ports" (an allocated host port).
type Port struct {
	Port      int     `json:"port"`
	ProjectID *string `json:"project_id"`
	IsLocked  *bool   `json:"is_locked"`
}

// PortWithProject is a port with its project's {id, name, status} (null when
// the port belongs to no project), as GET /api/ports returns it.
type PortWithProject struct {
	Port
	Project *ProjectRef `json:"project"`
}

func scanPort(r rowScanner) (*Port, error) {
	var p Port
	if err := r.Scan(&p.Port, &p.ProjectID, &p.IsLocked); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPortsWithProject returns every port row with its project included.
func (s *Store) ListPortsWithProject(ctx context.Context) ([]*PortWithProject, error) {
	return queryList(ctx, s.reader(), "list ports",
		`SELECT p."port", p."project_id", p."is_locked", pr."id", pr."name", pr."status"
		 FROM "ports" p LEFT JOIN "projects" pr ON pr."id" = p."project_id"`, nil,
		func(r rowScanner) (*PortWithProject, error) {
			var out PortWithProject
			var id, name, status *string
			if err := r.Scan(&out.Port.Port, &out.ProjectID, &out.IsLocked, &id, &name, &status); err != nil {
				return nil, err
			}
			if id != nil {
				out.Project = &ProjectRef{ID: *id, Name: deref(name), Status: status}
			}
			return &out, nil
		})
}

// ListLockedPorts returns the port numbers with is_locked = true.
func (s *Store) ListLockedPorts(ctx context.Context) ([]int, error) {
	return queryList(ctx, s.reader(), "list locked ports", `SELECT "port" FROM "ports" WHERE "is_locked" = 1`, nil,
		func(r rowScanner) (int, error) {
			var p int
			err := r.Scan(&p)
			return p, err
		})
}

// GetPort returns the row for a port number.
func (s *Store) GetPort(ctx context.Context, port int) (*Port, error) {
	return queryOne(ctx, s.reader(), "get port", `SELECT "port", "project_id", "is_locked" FROM "ports" WHERE "port" = ?`,
		[]any{port}, scanPort)
}

// ClaimPort assigns a free port row (not locked, no project) to projectID and
// locks it. It reports whether the row was claimed.
func (s *Store) ClaimPort(ctx context.Context, port int, projectID string) (bool, error) {
	n, err := execAffected(ctx, s.writer(), "claim port",
		`UPDATE "ports" SET "project_id" = ?, "is_locked" = 1 WHERE "port" = ? AND "is_locked" <> 1 AND "project_id" IS NULL`,
		projectID, port)
	return n == 1, err
}

// InsertLockedPort inserts a locked port row for projectID. It reports false
// (without error) when the port row already exists.
func (s *Store) InsertLockedPort(ctx context.Context, port int, projectID string) (bool, error) {
	n, err := execAffected(ctx, s.writer(), "insert port",
		`INSERT INTO "ports" ("port", "project_id", "is_locked") VALUES (?, ?, 1) ON CONFLICT ("port") DO NOTHING`,
		port, projectID)
	return n == 1, err
}

// ReleasePort clears the project and lock of a project's port row.
func (s *Store) ReleasePort(ctx context.Context, projectID string, port int) (int64, error) {
	return execAffected(ctx, s.writer(), "release port",
		`UPDATE "ports" SET "project_id" = NULL, "is_locked" = 0 WHERE "project_id" = ? AND "port" = ?`, projectID, port)
}

// DeletePort deletes the row of a port number.
func (s *Store) DeletePort(ctx context.Context, port int) (int64, error) {
	return execAffected(ctx, s.writer(), "delete port", `DELETE FROM "ports" WHERE "port" = ?`, port)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
