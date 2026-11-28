package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"docklift/internal/config"
	"docklift/internal/dockerx"
	"docklift/internal/store"
)

// DockerOps is the Docker surface the deploy service layer uses;
// *dockerx.Docker implements it and tests substitute a fake.
type DockerOps interface {
	ContainerStatus(ctx context.Context, name string) dockerx.ContainerStatus
	ContainerImageRef(ctx context.Context, name string) string
	ConnectProxy(ctx context.Context, network string) error
	DisconnectProxy(ctx context.Context, network string)
	TeardownNetwork(ctx context.Context, network string)
	ConnectContainer(ctx context.Context, network, container string) error
	DisconnectContainer(ctx context.Context, network, container string)
	ListImages(ctx context.Context) ([]dockerx.Image, error)
	RemoveImage(ctx context.Context, ref string) (bool, error)
	BuilderPrune(ctx context.Context, all bool) dockerx.PruneResult
}

var _ DockerOps = (*dockerx.Docker)(nil)

// Runtime is the deploy service layer the route groups call: it combines
// the store, Docker, the process-wide Tracker and the managed-database tag
// catalog. Create one per process with NewRuntime and share it.
type Runtime struct {
	Store   *store.Store
	Docker  DockerOps
	Tracker *Tracker
	Hub     *HubTags
	// DeploymentsPath is DEPLOYMENTS_PATH.
	DeploymentsPath string
	// PortRangeStart and PortRangeEnd bound host port allocation.
	PortRangeStart int
	PortRangeEnd   int
	Log            *slog.Logger
}

// NewRuntime wires a Runtime from the process configuration.
func NewRuntime(cfg *config.Config, st *store.Store, docker DockerOps, log *slog.Logger) *Runtime {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Runtime{
		Store:           st,
		Docker:          docker,
		Tracker:         NewTracker(),
		Hub:             NewHubTags(nil, log),
		DeploymentsPath: cfg.DeploymentsPath,
		PortRangeStart:  cfg.PortRangeStart,
		PortRangeEnd:    cfg.PortRangeEnd,
		Log:             log,
	}
}

// ProjectPath is the project's source checkout, DEPLOYMENTS_PATH/<id>.
func (r *Runtime) ProjectPath(projectID string) string {
	return filepath.Join(r.DeploymentsPath, projectID)
}

// StatePath is DEPLOYMENTS_PATH/.docklift/<id>.
func (r *Runtime) StatePath(projectID string) string {
	return RuntimeStatePath(r.DeploymentsPath, projectID)
}

// ComposePath is the project's runtime compose file.
func (r *Runtime) ComposePath(projectID string) string {
	return RuntimeComposePath(r.DeploymentsPath, projectID)
}

// ComposeWorkDir returns (creating it when needed) the working directory of
// every `docker compose` command: DEPLOYMENTS_PATH/.docklift/.compose-cwd, an
// empty directory Docklift owns. It is never a source checkout: without -f,
// compose looks for compose.yaml/docker-compose.yml in the working directory
// and its parents and reads .env from the project directory, so running it
// inside a repository would load the repository's own compose file and .env
// (and `down --rmi` would remove whatever images that file names).
func (r *Runtime) ComposeWorkDir() (string, error) {
	dir := filepath.Join(r.DeploymentsPath, ".docklift", ".compose-cwd")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create compose working directory: %w", err)
	}
	return dir, nil
}

// ErrRuntimeMissing is returned by ComposeFiles before the first deploy.
var ErrRuntimeMissing = errors.New("DockLift runtime configuration is missing. Deploy the project first.")

// ComposeFiles returns the runtime compose file for `docker compose -f`, or
// ErrRuntimeMissing when it does not exist (composeFileArgs).
func (r *Runtime) ComposeFiles(projectID string) ([]string, error) {
	p := r.ComposePath(projectID)
	if _, err := os.Stat(p); err != nil {
		return nil, ErrRuntimeMissing
	}
	return []string{p}, nil
}

// ConnectProxyToProjectNetwork attaches the edge proxy to the project network.
func (r *Runtime) ConnectProxyToProjectNetwork(ctx context.Context, projectID string) error {
	return r.Docker.ConnectProxy(ctx, ProjectNetworkName(projectID))
}

// ReattachProxyOnBoot attaches the edge proxy to the network of every
// project that has one. Deploys attach it, but the attachment belongs to the
// proxy container: when install or upgrade recreates docklift-nginx-proxy
// (the Go compose file gives it fixed addresses, so the first upgrade from
// 2.1.x always does), every app domain answered 502 until its project was
// redeployed. 2.x had no such step (found on a real 2.1.6 → Go upgrade).
// Projects without a network (never deployed, stopped) are skipped; other
// failures are logged and do not stop the boot.
func (r *Runtime) ReattachProxyOnBoot(ctx context.Context) error {
	projects, err := r.Store.ListProjects(ctx, store.ProjectFilter{}, store.OrderNone)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	attached := 0
	for _, p := range projects {
		err := r.ConnectProxyToProjectNetwork(ctx, p.ID)
		switch {
		case err == nil:
			attached++
		case dockerx.IsUnavailable(err):
			return nil
		case dockerx.IsNotFound(err):
		default:
			r.Log.Warn("edge proxy re-attach failed", "project", p.ID, "network", ProjectNetworkName(p.ID), "error", err)
		}
	}
	if attached > 0 {
		r.Log.Info(fmt.Sprintf("Edge proxy attached to %d project network(s)", attached))
	}
	return nil
}

// DisconnectProxyFromProjectNetwork detaches the edge proxy (best-effort).
func (r *Runtime) DisconnectProxyFromProjectNetwork(ctx context.Context, projectID string) {
	r.Docker.DisconnectProxy(ctx, ProjectNetworkName(projectID))
}

// TeardownProjectNetwork detaches the proxy and removes the project network
// (best-effort, used when deleting a project).
func (r *Runtime) TeardownProjectNetwork(ctx context.Context, projectID string) {
	r.Docker.TeardownNetwork(ctx, ProjectNetworkName(projectID))
}

// ConnectContainerToProjectNetwork attaches a container (e.g. a managed
// database) to the project network.
func (r *Runtime) ConnectContainerToProjectNetwork(ctx context.Context, projectID, containerName string) error {
	return r.Docker.ConnectContainer(ctx, ProjectNetworkName(projectID), containerName)
}

// DisconnectContainerFromProjectNetwork detaches a container (best-effort).
func (r *Runtime) DisconnectContainerFromProjectNetwork(ctx context.Context, projectID, containerName string) {
	r.Docker.DisconnectContainer(ctx, ProjectNetworkName(projectID), containerName)
}

// IsDeploymentCancelled reports whether the deployment was cancelled in
// memory or its row says "cancelled" (isDeploymentCancelled).
func (r *Runtime) IsDeploymentCancelled(ctx context.Context, projectID, deploymentID string) bool {
	if r.Tracker.WasBuildCancelled(projectID, deploymentID) {
		return true
	}
	d, err := r.Store.GetDeployment(ctx, deploymentID)
	return err == nil && d.Status != nil && *d.Status == StatusCancelled
}
