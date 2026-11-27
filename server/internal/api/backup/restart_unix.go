//go:build unix

package backup

import (
	"log/slog"
	"os"
	"syscall"
	"time"
)

// forcedExitAfter bounds the graceful shutdown started by restartProcess.
const forcedExitAfter = 30 * time.Second

// restartProcess ends the process after a committed restore so Docker's
// restart policy starts it again on the restored data (2.1.6 called
// process.exit(0)). It sends itself SIGTERM, which runs the normal graceful
// shutdown (requests drained, shutdown hooks, database closed) and exits 0;
// if that does not finish in time the process exits anyway.
func restartProcess(log *slog.Logger) {
	log.Info("Restarting backend service after restore...")
	time.AfterFunc(forcedExitAfter, func() { os.Exit(0) })
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		log.Error("could not signal shutdown after restore — exiting", "error", err)
		os.Exit(0)
	}
}
