// Package dockerx is Docklift's access to Docker: the Engine API (through the
// official moby client, where the TS backend used dockerode) and the docker
// CLI (compose, buildx, pull, volume, builder prune — where the TS backend
// spawned `docker`). It is the port of backend/src/services/docker.ts,
// backend/src/lib/runCompose.ts, backend/src/lib/composeTeardown.ts and the
// Docker half of backend/src/lib/imageCleanup.ts.
//
// Both Docker channels sit behind interfaces (Engine, Runner) so the logic is
// unit-tested without a daemon. Functions take Docker names (networks,
// containers, compose projects); Docklift's naming rules live in
// internal/deploy.
package dockerx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"

	"github.com/moby/moby/client"

	"docklift/internal/execx"
)

// Engine is the subset of the Docker Engine API client Docklift uses.
// *client.Client (github.com/moby/moby/client) implements it.
type Engine interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
	NetworkInspect(ctx context.Context, networkID string, options client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkCreate(ctx context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	NetworkConnect(ctx context.Context, networkID string, options client.NetworkConnectOptions) (client.NetworkConnectResult, error)
	NetworkDisconnect(ctx context.Context, networkID string, options client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error)
	NetworkRemove(ctx context.Context, networkID string, options client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	ImageList(ctx context.Context, options client.ImageListOptions) (client.ImageListResult, error)
	ImageInspect(ctx context.Context, imageID string, inspectOpts ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ImageRemove(ctx context.Context, imageID string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error)
}

var _ Engine = (*client.Client)(nil)

// Runner runs an external program; execx.Run is the production runner.
// Tests substitute a fake that records the calls.
type Runner func(ctx context.Context, c execx.Cmd) (execx.Result, error)

// Docker bundles the two Docker channels and Docklift's fixed names.
type Docker struct {
	// Engine is the Engine API client.
	Engine Engine
	// Run executes CLI commands (default execx.Run).
	Run Runner
	// Bin is the docker CLI program (default "docker").
	Bin string
	// ControlNetwork is DOCKER_NETWORK, the control-plane bridge network.
	ControlNetwork string
	// ProxyContainer is NGINX_PROXY_CONTAINER, the edge proxy attached to
	// every project network.
	ProxyContainer string
	// Log receives operational warnings (never secrets).
	Log *slog.Logger
}

// New returns a Docker using engine for the Engine API and the docker CLI
// from PATH. controlNetwork and proxyContainer come from Config
// (DockerNetwork, NginxProxyContainer).
func New(engine Engine, controlNetwork, proxyContainer string, log *slog.Logger) *Docker {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Docker{
		Engine:         engine,
		Run:            execx.Run,
		Bin:            "docker",
		ControlNetwork: controlNetwork,
		ProxyContainer: proxyContainer,
		Log:            log,
	}
}

// ErrUnavailable is returned (wrapped) by Ping when the daemon cannot be
// reached, so routes can answer 503 {"error":"Docker is not available"}.
var ErrUnavailable = errors.New("docker is not available")

// Ping checks that the daemon answers.
func (d *Docker) Ping(ctx context.Context) error {
	if d.Engine == nil {
		return ErrUnavailable
	}
	if _, err := d.Engine.Ping(ctx, client.PingOptions{}); err != nil {
		if IsUnavailable(err) {
			return errors.Join(ErrUnavailable, err)
		}
		return err
	}
	return nil
}

// IsUnavailable reports whether err means the daemon could not be reached
// (as opposed to an API error such as "no such container").
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable) || client.IsErrConnectionFailed(err)
}

// IsNotFound reports whether err is a Docker "no such object" error.
func IsNotFound(err error) bool {
	var nf interface{ NotFound() }
	if errors.As(err, &nf) {
		return true
	}
	return err != nil && notFoundRe.MatchString(err.Error())
}

var notFoundRe = regexp.MustCompile(`(?i)no such image|not found`)

func (d *Docker) bin() string {
	if d.Bin == "" {
		return "docker"
	}
	return d.Bin
}

func (d *Docker) runner() Runner {
	if d.Run == nil {
		return execx.Run
	}
	return d.Run
}

// cli runs the docker CLI with args and captured output.
func (d *Docker) cli(ctx context.Context, c execx.Cmd) (execx.Result, error) {
	c.Name = d.bin()
	return d.runner()(ctx, c)
}

func errMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
