package store

import "context"

// ProjectUsesPort reports whether any project row has port = port
// (prisma.project.findFirst({ where: { port } }) !== null).
func (s *Store) ProjectUsesPort(ctx context.Context, port int) (bool, error) {
	var n int
	err := s.reader().QueryRowContext(ctx, `SELECT COUNT(*) FROM "projects" WHERE "port" = ?`, port).Scan(&n)
	if err != nil {
		return false, mapErr(err, "find project by port")
	}
	return n > 0, nil
}
