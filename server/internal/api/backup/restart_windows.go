//go:build windows

package backup

import (
	"log/slog"
	"os"
)

// restartProcess ends the process after a committed restore so the service
// manager starts it again on the restored data (2.1.6 exited
// immediately). Windows has no self-deliverable SIGTERM, so it exits
// directly; the database's WAL is durable and recovered on the next open.
func restartProcess(log *slog.Logger) {
	log.Info("Restarting backend service after restore...")
	os.Exit(0)
}
