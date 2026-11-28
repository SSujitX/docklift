package dockerx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/moby/moby/client"

	"docklift/internal/execx"
)

// fakeEngine implements Engine with per-method hooks; a nil hook answers
// "not found".
type fakeEngine struct {
	mu    sync.Mutex
	calls []string

	inspect           func(name string) (client.ContainerInspectResult, error)
	list              func(o client.ContainerListOptions) (client.ContainerListResult, error)
	logs              func(name string, o client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	stats             func(name string, o client.ContainerStatsOptions) (client.ContainerStatsResult, error)
	networkInspect    func(name string) error
	networkCreate     func(name string, o client.NetworkCreateOptions) error
	networkConnect    func(network, container string) error
	networkDisconnect func(network, container string, force bool) error
	networkRemove     func(name string) error
	imageList         func() (client.ImageListResult, error)
	imageInspect      func(ref string) error
	imageRemove       func(ref string, o client.ImageRemoveOptions) error
}

type notFoundErr struct{ msg string }

func (e notFoundErr) Error() string { return e.msg }
func (notFoundErr) NotFound()       {}

func (f *fakeEngine) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeEngine) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeEngine) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (f *fakeEngine) ContainerInspect(_ context.Context, name string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	f.record("inspect " + name)
	if f.inspect == nil {
		return client.ContainerInspectResult{}, notFoundErr{"No such container: " + name}
	}
	return f.inspect(name)
}

func (f *fakeEngine) ContainerList(_ context.Context, o client.ContainerListOptions) (client.ContainerListResult, error) {
	f.record("list")
	if f.list == nil {
		return client.ContainerListResult{}, nil
	}
	return f.list(o)
}

func (f *fakeEngine) ContainerLogs(_ context.Context, name string, o client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	f.record("logs " + name)
	if f.logs == nil {
		return nil, notFoundErr{"No such container: " + name}
	}
	return f.logs(name, o)
}

func (f *fakeEngine) ContainerStats(_ context.Context, name string, o client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	f.record("stats " + name)
	if f.stats == nil {
		return client.ContainerStatsResult{}, notFoundErr{"No such container: " + name}
	}
	return f.stats(name, o)
}

func (f *fakeEngine) NetworkInspect(_ context.Context, name string, _ client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	f.record("network inspect " + name)
	if f.networkInspect == nil {
		return client.NetworkInspectResult{}, notFoundErr{"network " + name + " not found"}
	}
	return client.NetworkInspectResult{}, f.networkInspect(name)
}

func (f *fakeEngine) NetworkCreate(_ context.Context, name string, o client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
	f.record("network create " + name)
	if f.networkCreate == nil {
		return client.NetworkCreateResult{}, nil
	}
	return client.NetworkCreateResult{}, f.networkCreate(name, o)
}

func (f *fakeEngine) NetworkConnect(_ context.Context, network string, o client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
	f.record("network connect " + network + " " + o.Container)
	if f.networkConnect == nil {
		return client.NetworkConnectResult{}, nil
	}
	return client.NetworkConnectResult{}, f.networkConnect(network, o.Container)
}

func (f *fakeEngine) NetworkDisconnect(_ context.Context, network string, o client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error) {
	f.record("network disconnect " + network + " " + o.Container)
	if f.networkDisconnect == nil {
		return client.NetworkDisconnectResult{}, nil
	}
	return client.NetworkDisconnectResult{}, f.networkDisconnect(network, o.Container, o.Force)
}

func (f *fakeEngine) NetworkRemove(_ context.Context, name string, _ client.NetworkRemoveOptions) (client.NetworkRemoveResult, error) {
	f.record("network rm " + name)
	if f.networkRemove == nil {
		return client.NetworkRemoveResult{}, nil
	}
	return client.NetworkRemoveResult{}, f.networkRemove(name)
}

func (f *fakeEngine) ImageList(context.Context, client.ImageListOptions) (client.ImageListResult, error) {
	f.record("image list")
	if f.imageList == nil {
		return client.ImageListResult{}, nil
	}
	return f.imageList()
}

func (f *fakeEngine) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	f.record("image inspect " + ref)
	if f.imageInspect == nil {
		return client.ImageInspectResult{}, notFoundErr{"No such image: " + ref}
	}
	return client.ImageInspectResult{}, f.imageInspect(ref)
}

func (f *fakeEngine) ImageRemove(_ context.Context, ref string, o client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	f.record("image rm " + ref)
	if f.imageRemove == nil {
		return client.ImageRemoveResult{}, nil
	}
	return client.ImageRemoveResult{}, f.imageRemove(ref, o)
}

// fakeRun is a scripted CLI: answer maps the joined args to a result.
type fakeRun struct {
	mu     sync.Mutex
	calls  []execx.Cmd
	answer func(c execx.Cmd) (execx.Result, error)
}

func (f *fakeRun) Run(_ context.Context, c execx.Cmd) (execx.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.answer == nil {
		return execx.Result{ExitCode: 0}, nil
	}
	res, err := f.answer(c)
	if c.Stdout != nil && len(res.Stdout) > 0 {
		_, _ = c.Stdout.Write(res.Stdout)
		res.Stdout = nil
	}
	return res, err
}

func (f *fakeRun) Args() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.Name + " " + strings.Join(c.Args, " ")
	}
	return out
}

func exitErr(code int) error { return &execx.ExitError{Name: "docker", Code: code} }

func newTestDocker(e *fakeEngine, r *fakeRun) *Docker {
	d := New(e, "docklift_network", "docklift-nginx-proxy", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if r != nil {
		d.Run = r.Run
	}
	return d
}

var errBoom = errors.New("boom")
