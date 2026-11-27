package deploy

import (
	"context"
	"errors"
	"fmt"

	"docklift/internal/store"
)

// AllocatePort assigns the first free host port of the configured range to
// the project without stealing another project's port (allocatePort): in one
// transaction it skips locked ports and ports owned by a project, claims an
// unowned unlocked row, or inserts a new locked row. The project's port
// column is then set best-effort.
func (r *Runtime) AllocatePort(ctx context.Context, projectID string) (int, error) {
	port := 0
	err := r.Store.InTx(ctx, func(tx *store.Store) error {
		locked, err := tx.ListLockedPorts(ctx)
		if err != nil {
			return err
		}
		lockedSet := make(map[int]bool, len(locked))
		for _, p := range locked {
			lockedSet[p] = true
		}
		for candidate := r.PortRangeStart; candidate <= r.PortRangeEnd; candidate++ {
			if lockedSet[candidate] {
				continue
			}
			existing, err := tx.GetPort(ctx, candidate)
			switch {
			case err == nil:
				if (existing.IsLocked != nil && *existing.IsLocked) || existing.ProjectID != nil {
					continue
				}
				claimed, err := tx.ClaimPort(ctx, candidate, projectID)
				if err != nil {
					return err
				}
				if claimed {
					port = candidate
					return nil
				}
				continue
			case !errors.Is(err, store.ErrNotFound):
				return err
			}
			inserted, err := tx.InsertLockedPort(ctx, candidate, projectID)
			if err != nil {
				return err
			}
			if inserted {
				port = candidate
				return nil
			}
		}
		return fmt.Errorf("No free ports in range %d-%d", r.PortRangeStart, r.PortRangeEnd)
	})
	if err != nil {
		return 0, err
	}
	_, _ = r.Store.UpdateProject(ctx, projectID, store.ProjectUpdate{Port: store.Some(&port)})
	return port, nil
}

// ReleaseServicePort frees a project's port row (project_id null, unlocked),
// as the deploy route does when host publishing is turned off or a stale
// service is removed.
func (r *Runtime) ReleaseServicePort(ctx context.Context, projectID string, port int) error {
	_, err := r.Store.ReleasePort(ctx, projectID, port)
	return err
}
