package system

import (
	"errors"
	"time"
)

// Host runs the privileged host actions. Each method starts the action in
// the background and returns at once; the error is non-nil only when the
// action cannot be started on this platform. Results are logged.
type Host interface {
	// Reboot runs `reboot -f` (inside the privileged container) after a
	// one-second delay that lets the HTTP response go out.
	Reboot() error
	// UpdateSystem runs apt update/upgrade/autoremove on the host via
	// nsenter into PID 1 (15-minute timeout).
	UpdateSystem() error
	// Upgrade starts /opt/docklift/upgrade.sh on the host via nsenter,
	// detached with systemd-run (or nohup) so it survives the restart of
	// this container.
	Upgrade() error
}

// ErrHostActionsUnsupported is returned by the Host of platforms without
// nsenter (every OS but Linux).
var ErrHostActionsUnsupported = errors.New("host actions (nsenter, reboot) are only supported on Linux")

// Host command lines of system.ts. They are constants: nothing from a
// request ever reaches them.
var (
	rebootCommand = []string{"reboot", "-f"}
	// nsenter --target 1 with every namespace runs the command on the host.
	updateSystemCommand = []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--",
		"sh", "-c", "apt update -y && DEBIAN_FRONTEND=noninteractive apt upgrade -y && apt autoremove -y"}
	upgradeCommand = []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--",
		"sh", "-c", "cd /opt/docklift && (systemd-run --unit=docklift-upgrade-$(date +%s) --scope bash upgrade.sh > /dev/null 2>&1 || nohup bash upgrade.sh > /dev/null 2>&1) &"}
)

const (
	rebootDelay         = time.Second
	updateSystemTimeout = 15 * time.Minute
	// upgradeTimeout is short: the command detaches immediately.
	upgradeTimeout = 5 * time.Second
)
