//go:build linux

package sysinfo

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"docklift/internal/execx"
)

const (
	hostProcessesTimeout = 10 * time.Second
	commandTimeout       = 10 * time.Second
)

// nsenterPS runs `ps` in the host namespaces (PID 1). Docklift 2.x
// piped it through `head -n 15`; ParsePSAux keeps the first 15 lines.
var nsenterPS = []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "ps", "aux", "--sort=-%cpu"}

// topBatch is the container-local fallback (`| head -n 20` is done by ParseTop).
var topBatch = []string{"top", "-b", "-n", "1", "-w", "512", "-o", "%CPU"}

func run(ctx context.Context, timeout time.Duration, env []string, argv ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// A short kill grace keeps the timed-out nsenter -> top fallback prompt.
	res, err := execx.Run(ctx, execx.Cmd{Name: argv[0], Args: argv[1:], ExtraEnv: env, Stderr: io.Discard, KillGrace: 100 * time.Millisecond})
	return string(res.Stdout), err
}

// hostProcesses is getHostProcesses with its fallback: the host process
// table via nsenter; when that times out, top inside the container. A failing
// nsenter yields an empty list (the shell pipeline still exited 0 in
// 2.x), which makes the caller use the local process list.
func hostProcesses(ctx context.Context) []Process {
	out, err := run(ctx, hostProcessesTimeout, nil, nsenterPS...)
	if errors.Is(err, context.DeadlineExceeded) {
		slog.Error("Error getting host processes", "error", err)
		top, topErr := run(ctx, commandTimeout, nil, topBatch...)
		if topErr != nil && top == "" {
			slog.Error("Error running top", "error", topErr)
			return nil
		}
		return ParseTop(top)
	}
	return ParsePSAux(out)
}

func defaultInterface() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return ParseDefaultRoute(string(b))
}

func procUptime() (float64, bool) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	return ParseProcUptime(string(b))
}

// fsSize lists Linux filesystems: `df -kPTx squashfs` with
// LC_ALL=C, falling back to `df -kPT` (busybox df has no -x).
func fsSize(ctx context.Context) []FsEntry {
	out, err := run(ctx, commandTimeout, []string{"LC_ALL=C"}, "df", "-kPTx", "squashfs")
	data := ParseDF(out)
	if (err == nil || len(data) > 0) && strings.TrimSpace(out) != "" {
		return data
	}
	out, _ = run(ctx, commandTimeout, nil, "df", "-kPT")
	return ParseDF(out)
}

// gpu reads the first NVIDIA GPU when nvidia-smi is installed.
func gpu(ctx context.Context) *GPUController {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}
	out, err := run(ctx, commandTimeout, nil, "nvidia-smi",
		"--query-gpu=name,memory.total,memory.used,temperature.gpu,utilization.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return nil
	}
	return ParseNvidiaSMI(out)
}

func establishedConnections(context.Context) (int, error) {
	n, read := 0, 0
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6", "/proc/net/udp", "/proc/net/udp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		read++
		n += CountEstablished(string(b))
	}
	if read == 0 {
		return 0, errors.New("no /proc/net socket tables readable")
	}
	return n, nil
}

// distro is the container's own distribution name (DISTRIB_ID or NAME of
// os-release), used when /host/os-release is not mounted.
func distro(context.Context) string {
	for _, path := range []string{"/etc/lsb-release", "/etc/os-release", "/usr/lib/os-release"} {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		values := map[string]string{}
		for _, line := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				values[strings.TrimSpace(k)] = strings.ReplaceAll(strings.TrimSpace(v), `"`, "")
			}
		}
		if v := values["DISTRIB_ID"]; v != "" {
			return v
		}
		if v := values["NAME"]; v != "" {
			return v
		}
	}
	return "unknown"
}
