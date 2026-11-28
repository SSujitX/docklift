package store

import (
	"context"

	"docklift/internal/db"
)

// PersistentVolume is a row of "persistent_volumes".
type PersistentVolume struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	ServiceName string   `json:"service_name"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	MountPath   string   `json:"mount_path"`
	CreatedAt   *db.Time `json:"created_at"`
}

const volumeCols = `"id", "project_id", "service_name", "name", "display_name", "mount_path", "created_at"`

func scanVolume(r rowScanner) (*PersistentVolume, error) {
	var v PersistentVolume
	if err := r.Scan(&v.ID, &v.ProjectID, &v.ServiceName, &v.Name, &v.DisplayName, &v.MountPath, &v.CreatedAt); err != nil {
		return nil, err
	}
	return &v, nil
}

// GetVolume returns the volume with this id belonging to projectID.
func (s *Store) GetVolume(ctx context.Context, projectID, id string) (*PersistentVolume, error) {
	return queryOne(ctx, s.reader(), "get volume",
		`SELECT `+volumeCols+` FROM "persistent_volumes" WHERE "id" = ? AND "project_id" = ? LIMIT 1`,
		[]any{id, projectID}, scanVolume)
}

// ListVolumes returns a project's volumes in the given order.
func (s *Store) ListVolumes(ctx context.Context, projectID string, order Order) ([]*PersistentVolume, error) {
	return queryList(ctx, s.reader(), "list volumes",
		`SELECT `+volumeCols+` FROM "persistent_volumes" WHERE "project_id" = ?`+order.clause(),
		[]any{projectID}, scanVolume)
}

// VolumeCreate holds a new volume's fields.
type VolumeCreate struct {
	ProjectID   string
	ServiceName string
	Name        string
	DisplayName string
	MountPath   string
}

// CreateVolume inserts a volume; a duplicate (project_id, name) or
// (project_id, service_name, mount_path) is ErrConflict.
func (s *Store) CreateVolume(ctx context.Context, in VolumeCreate) (*PersistentVolume, error) {
	now := db.Now()
	v := &PersistentVolume{ID: db.NewID(), ProjectID: in.ProjectID, ServiceName: in.ServiceName, Name: in.Name,
		DisplayName: in.DisplayName, MountPath: in.MountPath, CreatedAt: &now}
	_, err := execAffected(ctx, s.writer(), "create volume",
		`INSERT INTO "persistent_volumes" (`+volumeCols+`) VALUES (`+placeholders(7)+`)`,
		v.ID, v.ProjectID, v.ServiceName, v.Name, v.DisplayName, v.MountPath, v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// UpdateVolumeMountPath changes a volume's mount path and returns the row.
func (s *Store) UpdateVolumeMountPath(ctx context.Context, id, mountPath string) (*PersistentVolume, error) {
	return queryOne(ctx, s.writer(), "update volume",
		`UPDATE "persistent_volumes" SET "mount_path" = ? WHERE "id" = ? RETURNING `+volumeCols,
		[]any{mountPath, id}, scanVolume)
}

// DeleteVolume deletes the volume with this id.
func (s *Store) DeleteVolume(ctx context.Context, id string) error {
	return execOne(ctx, s.writer(), "delete volume", `DELETE FROM "persistent_volumes" WHERE "id" = ?`, id)
}
