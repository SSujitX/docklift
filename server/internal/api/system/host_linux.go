//go:build linux

package system

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"time"
)

// linuxHost runs the host actions with exec (no shell built from input).
type linuxHost struct {
	background context.Context
	log        *slog.Logger
}

func newHost(background context.Context, log *slog.Logger) Host {
	return &linuxHost{background: background, log: log}
}

// detached is a context that survives server shutdown: an apt upgrade or a
// reboot must not be cancelled half-way because this process is stopping.
func (h *linuxHost) detached(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(h.background), timeout)
}

func (h *linuxHost) run(ctx context.Context, argv []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (h *linuxHost) Reboot() error {
	go func() {
		time.Sleep(rebootDelay)
		ctx, cancel := h.detached(time.Minute)
		defer cancel()
		// Without sudo: we are root in a privileged container.
		if _, stderr, err := h.run(ctx, rebootCommand); err != nil {
			h.log.Error("CRITICAL: Reboot command failed locally!", "error", err, "stderr", stderr)
			return
		}
		h.log.Info("Reboot command executed successfully. System should go down.")
	}()
	return nil
}

func (h *linuxHost) UpdateSystem() error {
	h.log.Info("System update initiated on host")
	go func() {
		ctx, cancel := h.detached(updateSystemTimeout)
		defer cancel()
		if _, stderr, err := h.run(ctx, updateSystemCommand); err != nil {
			h.log.Error("System update error", "error", err, "stderr", stderr)
			return
		}
		h.log.Info("System update completed successfully")
	}()
	return nil
}

func (h *linuxHost) Upgrade() error {
	h.log.Info("Docklift upgrade initiated on host")
	go func() {
		ctx, cancel := h.detached(upgradeTimeout)
		defer cancel()
		_, _, err := h.run(ctx, upgradeCommand)
		// A timeout is benign: the upgrade detached and nsenter lingered.
		if err != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			h.log.Error("Upgrade initiation error (might be benign if process detached)", "error", err)
			return
		}
		h.log.Info("Docklift upgrade initiated successfully")
	}()
	return nil
}
