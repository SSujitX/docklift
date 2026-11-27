package dockerx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"docklift/internal/execx"
)

// DefaultPullTimeout is the hard limit of an image pull (10 minutes).
const DefaultPullTimeout = 600 * time.Second

// PullOptions configures PullImage.
type PullOptions struct {
	// Output receives the docker pull output as it streams (may be nil).
	Output io.Writer
	// Timeout kills the pull after this long (0 = DefaultPullTimeout).
	Timeout time.Duration
}

// PullImage runs `docker pull image`, streaming its output (pullImage in
// docker.ts). Cancelling ctx kills the pull. Errors carry the TS messages:
// "Pull cancelled: <image>", "Timed out pulling <image> after <ms>ms",
// "Failed to pull <image> (exit <code>)".
func (d *Docker) PullImage(ctx context.Context, image string, o PullOptions) error {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultPullTimeout
	}
	if ctx.Err() != nil {
		return fmt.Errorf("Pull cancelled: %s", image)
	}
	out := o.Output
	if out == nil {
		out = io.Discard
	}
	res, err := d.cli(ctx, execx.Cmd{Args: []string{"pull", image}, Stdout: out, Stderr: out, Timeout: timeout, KillGrace: 2 * time.Second})
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return fmt.Errorf("Pull cancelled: %s", image)
	case errors.Is(err, execx.ErrTimeout):
		return fmt.Errorf("Timed out pulling %s after %dms", image, timeout.Milliseconds())
	}
	var ee *execx.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("Failed to pull %s (exit %d)", image, res.ExitCode)
	}
	return err
}

// ComposeCmd is one `docker compose` invocation:
// docker compose [-f File]... -p Project Args...
type ComposeCmd struct {
	Project string
	// Files are compose files passed with -f (empty = compose's defaults in Dir).
	Files []string
	// Dir is the working directory; never a source checkout (callers use
	// deploy.Runtime.ComposeWorkDir), so compose cannot load a repository's
	// own compose file or .env.
	Dir string
	// Args is the subcommand and its flags, e.g. {"up", "-d", "--remove-orphans"}.
	Args []string
	// Output receives stdout and stderr as they stream; nil captures them
	// into the result.
	Output io.Writer
	// Timeout kills the command after this long (0 = none).
	Timeout time.Duration
	// OnStart is called with the pid once the command runs.
	OnStart func(pid int)
}

// ComposeArgs returns the CLI arguments of c (after the docker program).
func (c ComposeCmd) ComposeArgs() []string {
	args := []string{"compose"}
	for _, f := range c.Files {
		args = append(args, "-f", f)
	}
	args = append(args, "-p", c.Project)
	return append(args, c.Args...)
}

// Compose runs a docker compose command (runCompose). err is non-nil only
// when the command did not run to completion: it could not start, ctx was
// cancelled (the process tree is killed) or the timeout fired. Otherwise err
// is nil and the result carries the exit code, which the caller judges (the
// TS onClose(code) callback).
func (d *Docker) Compose(ctx context.Context, c ComposeCmd) (execx.Result, error) {
	cmd := execx.Cmd{Args: c.ComposeArgs(), Dir: c.Dir, Timeout: c.Timeout, OnStart: c.OnStart}
	if c.Output != nil {
		cmd.Stdout, cmd.Stderr = c.Output, c.Output
	}
	res, err := d.cli(ctx, cmd)
	var ee *execx.ExitError
	if errors.As(err, &ee) {
		err = nil
	}
	return res, err
}

// ComposeResources are the containers and networks labelled with a compose
// project name.
type ComposeResources struct {
	ContainerIDs []string
	NetworkIDs   []string
}

// ProbeFailed is the id reported when a probe command fails, so a failed
// probe never proves absence.
const ProbeFailed = "__probe_failed__"

// ProbeComposeProject lists the containers and networks carrying the exact
// label com.docker.compose.project=<project> (probeComposeProjectResources).
func (d *Docker) ProbeComposeProject(ctx context.Context, project string) ComposeResources {
	filter := "label=com.docker.compose.project=" + project
	list := func(args ...string) []string {
		res, err := d.cli(ctx, execx.Cmd{Args: args, Timeout: 30 * time.Second})
		if err != nil || res.ExitCode != 0 {
			return []string{ProbeFailed}
		}
		return splitIDs(string(res.Stdout))
	}
	return ComposeResources{
		ContainerIDs: list("ps", "-aq", "--filter", filter),
		NetworkIDs:   list("network", "ls", "-q", "--filter", filter),
	}
}

func splitIDs(out string) []string {
	ids := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// Gone reports whether no labelled containers and networks remain
// (verifyComposeProjectGone).
func (r ComposeResources) Gone() bool {
	return len(r.ContainerIDs) == 0 && len(r.NetworkIDs) == 0
}

// IsComposeTeardownOK decides whether a compose down succeeded
// (isComposeTeardownOk): a run that did not complete (start failure,
// timeout) is never OK; otherwise — exit 0 or not — only the exact-label
// probe proves the project is gone. Absence is never inferred from stderr
// text such as "not found".
func IsComposeTeardownOK(runErr error, probe func() ComposeResources) bool {
	if runErr != nil {
		return false
	}
	return probe().Gone()
}

// DownResult is the outcome of one ComposeDown.
type DownResult struct {
	// OK is IsComposeTeardownOK for this run.
	OK bool
	// Stdout and Stderr are the captured output.
	Stdout string
	Stderr string
	// Err is set when the command did not complete (start failure, timeout).
	Err error
}

// Detail is the failure text the project delete route reports:
// stderr, else stdout, else the run error, else "compose down failed".
func (r DownResult) Detail() string {
	for _, s := range []string{r.Stderr, r.Stdout, errMessage(r.Err)} {
		if s != "" {
			return strings.TrimSpace(s)
		}
	}
	return "compose down failed"
}

// ComposeDown runs a compose teardown command (c.Args such as {"down",
// "--remove-orphans"}; default timeout 60 s) with captured output and
// verifies it with the exact-label probe of c.Project.
func (d *Docker) ComposeDown(ctx context.Context, c ComposeCmd) DownResult {
	if c.Timeout <= 0 {
		c.Timeout = 60 * time.Second
	}
	c.Output = nil
	res, err := d.Compose(ctx, c)
	return DownResult{
		OK:     IsComposeTeardownOK(err, func() ComposeResources { return d.ProbeComposeProject(ctx, c.Project) }),
		Stdout: string(res.Stdout),
		Stderr: string(res.Stderr),
		Err:    err,
	}
}

// RemoveContainer force-removes a container by name (`docker rm -f`),
// ignoring the outcome like the TS spawnSync calls with stdio ignored.
func (d *Docker) RemoveContainer(ctx context.Context, name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	_, _ = d.cli(ctx, execx.Cmd{Args: []string{"rm", "-f", name}, Timeout: 60 * time.Second})
}

// VolumeCreate runs `docker volume create --label L... name` (30 s timeout).
// On failure the returned error's message is docker's trimmed stderr when
// there is one (routes return it to the client).
func (d *Docker) VolumeCreate(ctx context.Context, name string, labels ...string) error {
	args := []string{"volume", "create"}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	res, err := d.cli(ctx, execx.Cmd{Args: append(args, name), Timeout: 30 * time.Second})
	if err == nil {
		return nil
	}
	if msg := strings.TrimSpace(string(res.Stderr)); msg != "" {
		return errors.New(msg)
	}
	return err
}

// VolumeRemove runs `docker volume rm -f name` (30 s timeout) and returns its
// captured result; err is non-nil for a non-zero exit too.
func (d *Docker) VolumeRemove(ctx context.Context, name string) (execx.Result, error) {
	return d.cli(ctx, execx.Cmd{Args: []string{"volume", "rm", "-f", name}, Timeout: 30 * time.Second})
}

// PruneResult is the outcome of a BuildKit prune.
type PruneResult struct {
	OK     bool
	Output string
}

// BuilderPrune runs `docker builder prune -f` (unused cache) or, with all,
// `docker builder prune -af` (pruneBuildKitUnused / pruneBuildKitAll).
// Output is the trimmed stdout, else stderr.
func (d *Docker) BuilderPrune(ctx context.Context, all bool) PruneResult {
	flag := "-f"
	if all {
		flag = "-af"
	}
	res, err := d.cli(ctx, execx.Cmd{Args: []string{"builder", "prune", flag}, Timeout: 30 * time.Minute})
	out := string(res.Stdout)
	if out == "" {
		out = string(res.Stderr)
	}
	var se *execx.StartError
	if out == "" && errors.As(err, &se) {
		out = se.Err.Error()
	}
	return PruneResult{OK: err == nil, Output: strings.TrimSpace(out)}
}
