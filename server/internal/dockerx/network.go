package dockerx

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/moby/moby/client"
)

// EnsureNetwork creates the control-plane bridge network (ControlNetwork)
// when inspecting it fails.
func (d *Docker) EnsureNetwork(ctx context.Context) error {
	if d.Engine == nil {
		return ErrUnavailable
	}
	if _, err := d.Engine.NetworkInspect(ctx, d.ControlNetwork, client.NetworkInspectOptions{}); err == nil {
		return nil
	}
	_, err := d.Engine.NetworkCreate(ctx, d.ControlNetwork, client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"com.docklift.managed": "true",
			"com.docklift.role":    "control-plane",
		},
	})
	if err != nil {
		return fmt.Errorf("create network %s: %w", d.ControlNetwork, err)
	}
	return nil
}

var alreadyConnectedRe = regexp.MustCompile(`(?i)already exists|already connected`)

// IsAlreadyConnectedError reports whether a network connect failed only
// because the container is already attached (isProxyAlreadyConnectedError).
func IsAlreadyConnectedError(err error) bool {
	return err != nil && alreadyConnectedRe.MatchString(err.Error())
}

// ConnectContainer attaches container to network. The network must exist
// (inspect first, so a missing network is an error, never silent success);
// an "already connected" answer counts as success
// (connectContainerToProjectNetwork).
func (d *Docker) ConnectContainer(ctx context.Context, network, container string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("containerName is required")
	}
	return d.connect(ctx, network, container, false)
}

func (d *Docker) connect(ctx context.Context, network, container string, logFailure bool) error {
	if d.Engine == nil {
		return ErrUnavailable
	}
	if _, err := d.Engine.NetworkInspect(ctx, network, client.NetworkInspectOptions{}); err != nil {
		return fmt.Errorf("inspect network %s: %w", network, err)
	}
	if _, err := d.Engine.NetworkConnect(ctx, network, client.NetworkConnectOptions{Container: container}); err != nil {
		if IsAlreadyConnectedError(err) {
			return nil
		}
		if logFailure {
			d.Log.Error("[docker] failed to attach edge proxy", "proxy", container, "network", network, "error", err)
		}
		return fmt.Errorf("connect %s to %s: %w", container, network, err)
	}
	return nil
}

// DisconnectContainer force-detaches container from network, ignoring every
// error (not connected, network gone, daemon down) like the 2.x best-effort
// helpers.
func (d *Docker) DisconnectContainer(ctx context.Context, network, container string) {
	if d.Engine == nil || strings.TrimSpace(container) == "" {
		return
	}
	_, _ = d.Engine.NetworkDisconnect(ctx, network, client.NetworkDisconnectOptions{Container: container, Force: true})
}

// ConnectProxy attaches the edge proxy to a project network so it resolves
// container_name DNS (connectProxyToProjectNetwork). Failures other than
// "already connected" are logged and returned: callers must mark the deploy
// degraded or failed.
func (d *Docker) ConnectProxy(ctx context.Context, network string) error {
	return d.connect(ctx, network, d.ProxyContainer, true)
}

// DisconnectProxy detaches the edge proxy so compose down can remove the
// network (best-effort).
func (d *Docker) DisconnectProxy(ctx context.Context, network string) {
	d.DisconnectContainer(ctx, network, d.ProxyContainer)
}

// TeardownNetwork detaches the edge proxy and removes the network,
// best-effort (teardownProjectNetwork).
func (d *Docker) TeardownNetwork(ctx context.Context, network string) {
	d.DisconnectProxy(ctx, network)
	if d.Engine == nil {
		return
	}
	_, _ = d.Engine.NetworkRemove(ctx, network, client.NetworkRemoveOptions{})
}
