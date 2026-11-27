package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
)

// Docker is what a restore asks of Docker. DockerCLI is the production
// implementation; tests substitute a fake.
type Docker interface {
	// RunningUserContainers lists running Docklift app containers
	// (`docker ps -q --filter name=dl_`).
	RunningUserContainers(ctx context.Context) ([]string, error)
	// StopContainers runs `docker stop` on ids.
	StopContainers(ctx context.Context, ids []string) error
	// VolumeCreate runs `docker volume create name` (exists-ok).
	VolumeCreate(ctx context.Context, name string) error
	// ComposeUp runs `docker compose -f file -p project up -d` in dir.
	ComposeUp(ctx context.Context, file, project, dir string) error
	// ConnectProxy attaches the edge proxy to the project's network.
	ConnectProxy(ctx context.Context, projectID string) error
	// ReloadProxy runs `docker exec <nginx-proxy> nginx -s reload` (no
	// self-heal: restored confs are never deleted mid-restore).
	ReloadProxy(ctx context.Context) error
}

// Timeouts of the restore's Docker commands.
const (
	psTimeout      = 30 * time.Second
	stopTimeout    = 5 * time.Minute
	composeTimeout = 5 * time.Minute
	reloadTimeout  = 30 * time.Second
)

// DockerCLI runs the restore's Docker commands through the docker CLI of
// dockerx (argument lists, never a shell).
type DockerCLI struct {
	X       *dockerx.Docker
	Runtime *deploy.Runtime
	// ProxyContainer is NGINX_PROXY_CONTAINER.
	ProxyContainer string
}

var _ Docker = DockerCLI{}

func (c DockerCLI) run(ctx context.Context, cmd execx.Cmd) (execx.Result, error) {
	cmd.Name = c.X.Bin
	if cmd.Name == "" {
		cmd.Name = "docker"
	}
	run := c.X.Run
	if run == nil {
		run = execx.Run
	}
	return run(ctx, cmd)
}

// commandError is the message of a failed command: "Command failed: <cmd>"
// plus stderr, the shape of a child_process exec error.
func commandError(display string, res execx.Result, err error) error {
	detail := strings.TrimSpace(string(res.Stderr))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("Command failed: %s\n%s", display, detail)
}

// RunningUserContainers implements Docker.
func (c DockerCLI) RunningUserContainers(ctx context.Context) ([]string, error) {
	res, err := c.run(ctx, execx.Cmd{Args: []string{"ps", "-q", "--filter", "name=dl_"}, Timeout: psTimeout})
	if err != nil {
		return nil, commandError("docker ps -q --filter name=dl_", res, err)
	}
	return strings.Fields(string(res.Stdout)), nil
}

// StopContainers implements Docker.
func (c DockerCLI) StopContainers(ctx context.Context, ids []string) error {
	res, err := c.run(ctx, execx.Cmd{Args: append([]string{"stop"}, ids...), Timeout: stopTimeout})
	if err != nil {
		return commandError("docker stop "+strings.Join(ids, " "), res, err)
	}
	return nil
}

// VolumeCreate implements Docker.
func (c DockerCLI) VolumeCreate(ctx context.Context, name string) error {
	return c.X.VolumeCreate(ctx, name)
}

// ComposeUp implements Docker with the environment Docklift 2.x used.
func (c DockerCLI) ComposeUp(ctx context.Context, file, project, dir string) error {
	args := dockerx.ComposeCmd{Project: project, Files: []string{file}, Args: []string{"up", "-d"}}.ComposeArgs()
	res, err := c.run(ctx, execx.Cmd{
		Args:     args,
		Dir:      dir,
		ExtraEnv: []string{"DOCKER_BUILDKIT=1", "COMPOSE_DOCKER_CLI_BUILD=1"},
		Timeout:  composeTimeout,
	})
	if err != nil {
		return commandError(fmt.Sprintf("docker compose -f %q -p %s up -d", file, project), res, err)
	}
	return nil
}

// ConnectProxy implements Docker.
func (c DockerCLI) ConnectProxy(ctx context.Context, projectID string) error {
	return c.Runtime.ConnectProxyToProjectNetwork(ctx, projectID)
}

// ReloadProxy implements Docker.
func (c DockerCLI) ReloadProxy(ctx context.Context) error {
	res, err := c.run(ctx, execx.Cmd{Args: []string{"exec", c.ProxyContainer, "nginx", "-s", "reload"}, Timeout: reloadTimeout})
	if err != nil {
		return commandError("docker exec "+c.ProxyContainer+" nginx -s reload", res, err)
	}
	return nil
}
