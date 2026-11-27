package dockerx

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"

	"docklift/internal/execx"
)

func TestIsAlreadyConnectedError(t *testing.T) {
	// Cases of docker.proxy.test.ts.
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("endpoint with name x already exists in network y"), true},
		{errors.New("container already connected"), true},
		{errors.New("Container ALREADY CONNECTED to network"), true},
		{errors.New("network not found"), false},
		{nil, false},
	}
	for _, c := range cases {
		if got := IsAlreadyConnectedError(c.err); got != c.want {
			t.Errorf("IsAlreadyConnectedError(%v) = %v", c.err, got)
		}
	}
}

func TestEnsureNetwork(t *testing.T) {
	e := &fakeEngine{networkInspect: func(string) error { return nil }}
	if err := newTestDocker(e, nil).EnsureNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(e.Calls(), "network create docklift_network") {
		t.Fatal("existing network must not be recreated")
	}

	var created client.NetworkCreateOptions
	e = &fakeEngine{networkCreate: func(_ string, o client.NetworkCreateOptions) error { created = o; return nil }}
	if err := newTestDocker(e, nil).EnsureNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"com.docklift.managed": "true", "com.docklift.role": "control-plane"}
	if created.Driver != "bridge" || !reflect.DeepEqual(created.Labels, want) {
		t.Fatalf("created %+v", created)
	}

	e = &fakeEngine{networkCreate: func(string, client.NetworkCreateOptions) error { return errBoom }}
	if err := newTestDocker(e, nil).EnsureNetwork(context.Background()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	nilEngine := New(nil, "n", "p", nil)
	if err := nilEngine.EnsureNetwork(context.Background()); !IsUnavailable(err) {
		t.Fatalf("nil engine: %v", err)
	}
}

func TestConnectProxy(t *testing.T) {
	ctx := context.Background()
	// Missing project network -> error, never silent success.
	e := &fakeEngine{}
	if err := newTestDocker(e, nil).ConnectProxy(ctx, "dl-net-00000000"); err == nil {
		t.Fatal("missing network must fail")
	}
	if slices.ContainsFunc(e.Calls(), func(s string) bool { return strings.HasPrefix(s, "network connect") }) {
		t.Fatal("must not connect when inspect failed")
	}

	e = &fakeEngine{
		networkInspect: func(string) error { return nil },
		networkConnect: func(string, string) error {
			return errors.New("endpoint with name docklift-nginx-proxy already exists in network dl-net-1")
		},
	}
	if err := newTestDocker(e, nil).ConnectProxy(ctx, "dl-net-1"); err != nil {
		t.Fatalf("already connected must succeed: %v", err)
	}
	if !slices.Contains(e.Calls(), "network connect dl-net-1 docklift-nginx-proxy") {
		t.Fatalf("calls %v", e.Calls())
	}

	e.networkConnect = func(string, string) error { return errBoom }
	if err := newTestDocker(e, nil).ConnectProxy(ctx, "dl-net-1"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectContainerRequiresName(t *testing.T) {
	d := newTestDocker(&fakeEngine{networkInspect: func(string) error { return nil }}, nil)
	if err := d.ConnectContainer(context.Background(), "n", "  "); err == nil || err.Error() != "containerName is required" {
		t.Fatalf("err = %v", err)
	}
}

func TestDisconnectAndTeardownAreBestEffort(t *testing.T) {
	var forced bool
	e := &fakeEngine{
		networkDisconnect: func(_, _ string, force bool) error { forced = force; return errBoom },
		networkRemove:     func(string) error { return errBoom },
	}
	d := newTestDocker(e, nil)
	d.DisconnectContainer(context.Background(), "dl-net-1", "")
	if len(e.Calls()) != 0 {
		t.Fatal("empty container name must be a no-op")
	}
	d.TeardownNetwork(context.Background(), "dl-net-1")
	want := []string{"network disconnect dl-net-1 docklift-nginx-proxy", "network rm dl-net-1"}
	if !reflect.DeepEqual(e.Calls(), want) || !forced {
		t.Fatalf("calls %v forced %v", e.Calls(), forced)
	}
}

func inspectResult(status string, running, tty bool, img string) client.ContainerInspectResult {
	return client.ContainerInspectResult{Container: container.InspectResponse{
		State:  &container.State{Status: container.ContainerState(status), Running: running},
		Config: &container.Config{Tty: tty, Image: img},
		Image:  "sha256:abc",
	}}
}

func TestContainerStatus(t *testing.T) {
	ctx := context.Background()
	e := &fakeEngine{inspect: func(string) (client.ContainerInspectResult, error) {
		return inspectResult("running", true, false, ""), nil
	}}
	if got := newTestDocker(e, nil).ContainerStatus(ctx, "c"); got != (ContainerStatus{"running", true}) {
		t.Fatalf("got %+v", got)
	}

	var filters client.Filters
	e = &fakeEngine{list: func(o client.ContainerListOptions) (client.ContainerListResult, error) {
		filters = o.Filters
		if !o.All {
			t.Error("list must include stopped containers")
		}
		return client.ContainerListResult{Items: []container.Summary{{State: "exited"}}}, nil
	}}
	if got := newTestDocker(e, nil).ContainerStatus(ctx, "c"); got != (ContainerStatus{"exited", false}) {
		t.Fatalf("got %+v", got)
	}
	if !filters["name"]["c"] {
		t.Fatalf("filters %v", filters)
	}

	e = &fakeEngine{list: func(client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{}, errBoom
	}}
	if got := newTestDocker(e, nil).ContainerStatus(ctx, "c"); got != (ContainerStatus{StatusNotFound, false}) {
		t.Fatalf("got %+v", got)
	}
	if got := New(nil, "", "", nil).ContainerStatus(ctx, "c"); got.Status != StatusNotFound {
		t.Fatalf("nil engine %+v", got)
	}
}

func TestContainerImageRef(t *testing.T) {
	e := &fakeEngine{inspect: func(string) (client.ContainerInspectResult, error) {
		return inspectResult("running", true, false, "docklift-abcdef12-app:11111111"), nil
	}}
	d := newTestDocker(e, nil)
	if got := d.ContainerImageRef(context.Background(), "c"); got != "docklift-abcdef12-app:11111111" {
		t.Fatalf("got %q", got)
	}
	e.inspect = func(string) (client.ContainerInspectResult, error) {
		return inspectResult("running", true, false, ""), nil
	}
	if got := d.ContainerImageRef(context.Background(), "c"); got != "sha256:abc" {
		t.Fatalf("fallback to image id: %q", got)
	}
	if got := d.ContainerImageRef(context.Background(), " "); got != "" || len(e.Calls()) != 2 {
		t.Fatalf("blank name: %q %v", got, e.Calls())
	}
}

func frame(stream byte, payload string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

type logsRC struct{ io.Reader }

func (logsRC) Close() error { return nil }

func TestContainerLogsDemux(t *testing.T) {
	raw := append(frame(1, "out line\n"), frame(2, "err line\n")...)
	var gotTail string
	e := &fakeEngine{
		inspect: func(string) (client.ContainerInspectResult, error) {
			return inspectResult("running", true, false, ""), nil
		},
		logs: func(_ string, o client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
			gotTail = o.Tail
			if !o.ShowStdout || !o.ShowStderr || o.Timestamps || o.Follow {
				t.Errorf("options %+v", o)
			}
			return logsRC{bytes.NewReader(raw)}, nil
		},
	}
	d := newTestDocker(e, nil)
	if got := d.ContainerLogs(context.Background(), "c", 100); got != "out line\nerr line\n" || gotTail != "100" {
		t.Fatalf("logs %q tail %q", got, gotTail)
	}
	e.inspect = func(string) (client.ContainerInspectResult, error) {
		return inspectResult("running", true, true, ""), nil
	}
	e.logs = func(string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return logsRC{strings.NewReader("\x01raw tty")}, nil
	}
	if got := d.ContainerLogs(context.Background(), "c", 5); got != "\x01raw tty" {
		t.Fatalf("tty logs %q", got)
	}
	e.logs = nil
	if got := d.ContainerLogs(context.Background(), "c", 5); got != "" {
		t.Fatalf("error must give empty logs, got %q", got)
	}
}

func TestFormatStats(t *testing.T) {
	s := &container.StatsResponse{}
	s.CPUStats.CPUUsage.TotalUsage = 2_000
	s.PreCPUStats.CPUUsage.TotalUsage = 1_000
	s.CPUStats.SystemUsage = 20_000
	s.PreCPUStats.SystemUsage = 10_000
	s.CPUStats.OnlineCPUs = 2
	s.MemoryStats.Usage = 50 * 1024 * 1024
	s.MemoryStats.Limit = 200 * 1024 * 1024
	got := FormatStats(s)
	want := &ContainerStats{CPUPercent: "20.00", MemoryUsage: "50.00 MB", MemoryLimit: "200.00 MB", MemoryPercent: "25.00"}
	if *got != *want {
		t.Fatalf("got %+v", got)
	}
	// Online CPUs fall back to the per-CPU list, then 1; zero limit counts as 1 byte.
	s = &container.StatsResponse{}
	s.CPUStats.CPUUsage.TotalUsage = 30
	s.CPUStats.CPUUsage.PercpuUsage = []uint64{1, 2, 3}
	s.CPUStats.SystemUsage = 100
	got = FormatStats(s)
	if got.CPUPercent != "90.00" || got.MemoryLimit != "0.00 MB" || got.MemoryPercent != "0.00" {
		t.Fatalf("got %+v", got)
	}
	// A counter reset (negative system delta) yields 0.
	s = &container.StatsResponse{}
	s.PreCPUStats.SystemUsage = 10
	if got = FormatStats(s); got.CPUPercent != "0.00" {
		t.Fatalf("got %+v", got)
	}
	b, _ := json.Marshal(got)
	if string(b) != `{"cpu_percent":"0.00","memory_usage":"0.00 MB","memory_limit":"0.00 MB","memory_percent":"0.00"}` {
		t.Fatalf("json %s", b)
	}
}

func TestContainerStatsReadsOneSampleWithPrevious(t *testing.T) {
	var opts client.ContainerStatsOptions
	e := &fakeEngine{stats: func(_ string, o client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		opts = o
		return client.ContainerStatsResult{Body: io.NopCloser(strings.NewReader(`{"memory_stats":{"usage":1048576,"limit":2097152}}`))}, nil
	}}
	d := newTestDocker(e, nil)
	got := d.ContainerStats(context.Background(), "c")
	if got == nil || got.MemoryPercent != "50.00" || opts.Stream || !opts.IncludePreviousSample {
		t.Fatalf("got %+v opts %+v", got, opts)
	}
	e.stats = nil
	if d.ContainerStats(context.Background(), "c") != nil {
		t.Fatal("error must give nil stats")
	}
}

func TestLogEventJSON(t *testing.T) {
	cases := []struct {
		ev   LogEvent
		want string
	}{
		{LogEvent{Type: LogEventConnected, Container: "dl_a_b"}, `{"type":"connected","container":"dl_a_b"}`},
		{LogEvent{Type: LogEventLog, Message: "<b>&\"x\"\n"}, `{"type":"log","message":"<b>&\"x\"\n"}`},
		{LogEvent{Type: LogEventEnd, Message: "Log stream ended"}, `{"type":"end","message":"Log stream ended"}`},
	}
	for _, c := range cases {
		b, err := c.ev.MarshalJSON()
		if err != nil || string(b) != c.want {
			t.Errorf("%+v -> %s (%v)", c.ev, b, err)
		}
	}
	if got := string(LogEvent{Type: LogEventStatus, Message: "m"}.SSEFrame()); got != "data: {\"type\":\"status\",\"message\":\"m\"}\n\n" {
		t.Fatalf("frame %q", got)
	}
}

// chunkReader returns one chunk per Read.
type chunkReader struct {
	chunks [][]byte
	err    error
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.chunks) == 0 {
		if c.err != nil {
			return 0, c.err
		}
		return 0, io.EOF
	}
	n := copy(p, c.chunks[0])
	c.chunks[0] = c.chunks[0][n:]
	if len(c.chunks[0]) == 0 {
		c.chunks = c.chunks[1:]
	}
	return n, nil
}

func (c *chunkReader) Close() error { return nil }

func collect(t *testing.T, d *Docker, ctx context.Context) []LogEvent {
	t.Helper()
	var evs []LogEvent
	_ = d.StreamContainerLogs(ctx, "c", 200, func(ev LogEvent) error {
		evs = append(evs, ev)
		return nil
	})
	return evs
}

func TestStreamContainerLogs(t *testing.T) {
	ctx := context.Background()
	d := newTestDocker(&fakeEngine{}, nil)
	evs := collect(t, d, ctx)
	if len(evs) != 1 || evs[0].Type != LogEventError || !strings.HasPrefix(evs[0].Message, "Container not found: ") {
		t.Fatalf("missing container: %+v", evs)
	}

	e := &fakeEngine{inspect: func(string) (client.ContainerInspectResult, error) {
		return inspectResult("exited", false, false, ""), nil
	}}
	evs = collect(t, newTestDocker(e, nil), ctx)
	if !reflect.DeepEqual(evs, []LogEvent{{Type: LogEventStatus, Message: "Container is not running"}}) {
		t.Fatalf("not running: %+v", evs)
	}

	// Frames split across reads, a blank frame, then EOF.
	stream := slices.Concat(frame(1, "2026-01-01T00:00:00Z hello\n"), frame(2, "  \n"), frame(1, "bye\n"))
	var opts client.ContainerLogsOptions
	e = &fakeEngine{
		inspect: func(string) (client.ContainerInspectResult, error) {
			return inspectResult("running", true, false, ""), nil
		},
		logs: func(_ string, o client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
			opts = o
			return &chunkReader{chunks: [][]byte{stream[:5], stream[5:20], stream[20:]}}, nil
		},
	}
	evs = collect(t, newTestDocker(e, nil), ctx)
	want := []LogEvent{
		{Type: LogEventConnected, Container: "c"},
		{Type: LogEventLog, Message: "2026-01-01T00:00:00Z hello\n"},
		{Type: LogEventLog, Message: "bye\n"},
		{Type: LogEventEnd, Message: "Log stream ended"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Fatalf("events %+v", evs)
	}
	if !opts.Follow || !opts.Timestamps || opts.Tail != "200" {
		t.Fatalf("options %+v", opts)
	}

	// TTY containers stream raw chunks; a read error becomes an error event.
	e.inspect = func(string) (client.ContainerInspectResult, error) {
		return inspectResult("running", true, true, ""), nil
	}
	e.logs = func(string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return &chunkReader{chunks: [][]byte{[]byte("raw \xff text")}, err: errBoom}, nil
	}
	evs = collect(t, newTestDocker(e, nil), ctx)
	if len(evs) != 3 || evs[1].Message != "raw � text" || evs[2] != (LogEvent{Type: LogEventError, Message: "boom"}) {
		t.Fatalf("tty events %+v", evs)
	}

	// A failing emit (client gone) stops the stream.
	e.logs = func(string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return &chunkReader{chunks: [][]byte{[]byte("a"), []byte("b")}}, nil
	}
	n := 0
	err := newTestDocker(e, nil).StreamContainerLogs(ctx, "c", 1, func(LogEvent) error {
		n++
		if n == 2 {
			return errBoom
		}
		return nil
	})
	if !errors.Is(err, errBoom) || n != 2 {
		t.Fatalf("err %v n %d", err, n)
	}

	// Cancelled context: nothing is emitted after the read fails.
	cctx, cancel := context.WithCancel(ctx)
	e.logs = func(string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		cancel()
		return &chunkReader{err: context.Canceled}, nil
	}
	evs = collect(t, newTestDocker(e, nil), cctx)
	if len(evs) != 1 || evs[0].Type != LogEventConnected {
		t.Fatalf("cancelled events %+v", evs)
	}
}

func TestPullImage(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	r := &fakeRun{answer: func(c execx.Cmd) (execx.Result, error) {
		_, _ = c.Stdout.Write([]byte("Pulling...\n"))
		return execx.Result{ExitCode: 0}, nil
	}}
	if err := newTestDocker(&fakeEngine{}, r).PullImage(ctx, "postgres:18-alpine", PullOptions{Output: &out}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Pulling...\n" || r.Args()[0] != "docker pull postgres:18-alpine" || r.calls[0].Timeout != DefaultPullTimeout {
		t.Fatalf("out %q args %v", out.String(), r.Args())
	}

	r = &fakeRun{answer: func(execx.Cmd) (execx.Result, error) { return execx.Result{ExitCode: 1}, exitErr(1) }}
	if err := newTestDocker(&fakeEngine{}, r).PullImage(ctx, "x:1", PullOptions{}); err == nil || err.Error() != "Failed to pull x:1 (exit 1)" {
		t.Fatalf("err %v", err)
	}
	r = &fakeRun{answer: func(execx.Cmd) (execx.Result, error) { return execx.Result{ExitCode: -1}, execx.ErrTimeout }}
	if err := newTestDocker(&fakeEngine{}, r).PullImage(ctx, "x:1", PullOptions{Timeout: 1500 * time.Millisecond}); err == nil || err.Error() != "Timed out pulling x:1 after 1500ms" {
		t.Fatalf("err %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := newTestDocker(&fakeEngine{}, &fakeRun{}).PullImage(cctx, "x:1", PullOptions{}); err == nil || err.Error() != "Pull cancelled: x:1" {
		t.Fatalf("err %v", err)
	}
}

func TestComposeArgsAndExitCodes(t *testing.T) {
	c := ComposeCmd{Project: "dl-app-aaaaaaaa", Files: []string{"/d/.docklift/x/compose.yml"}, Args: []string{"up", "-d", "--remove-orphans"}}
	want := []string{"compose", "-f", "/d/.docklift/x/compose.yml", "-p", "dl-app-aaaaaaaa", "up", "-d", "--remove-orphans"}
	if !reflect.DeepEqual(c.ComposeArgs(), want) {
		t.Fatalf("args %v", c.ComposeArgs())
	}
	if got := (ComposeCmd{Project: "uuid", Args: []string{"down"}}).ComposeArgs(); !reflect.DeepEqual(got, []string{"compose", "-p", "uuid", "down"}) {
		t.Fatalf("args %v", got)
	}

	r := &fakeRun{answer: func(execx.Cmd) (execx.Result, error) { return execx.Result{ExitCode: 3}, exitErr(3) }}
	res, err := newTestDocker(&fakeEngine{}, r).Compose(context.Background(), c)
	if err != nil || res.ExitCode != 3 {
		t.Fatalf("non-zero exit is a result, not an error: %v %d", err, res.ExitCode)
	}
	r = &fakeRun{answer: func(execx.Cmd) (execx.Result, error) {
		return execx.Result{ExitCode: -1}, &execx.StartError{Name: "docker", Err: errBoom}
	}}
	if _, err := newTestDocker(&fakeEngine{}, r).Compose(context.Background(), c); !errors.Is(err, errBoom) {
		t.Fatalf("start failure must be an error: %v", err)
	}
}

func TestIsComposeTeardownOK(t *testing.T) {
	// Cases of composeTeardown.test.ts.
	empty := func() ComposeResources { return ComposeResources{} }
	left := func() ComposeResources { return ComposeResources{ContainerIDs: []string{"abc"}} }
	leftNet := func() ComposeResources { return ComposeResources{NetworkIDs: []string{"n"}} }
	if !IsComposeTeardownOK(nil, empty) {
		t.Error("exit 0 + empty probe must be OK")
	}
	if IsComposeTeardownOK(nil, left) || IsComposeTeardownOK(nil, leftNet) {
		t.Error("leftover containers or networks are never OK")
	}
	// A non-zero exit (stderr "not found" for an unrelated reason) is judged
	// only by the probe; Compose already turned it into a nil error.
	if IsComposeTeardownOK(errBoom, empty) {
		t.Error("a run that did not complete is never OK")
	}
	if !(ComposeResources{}).Gone() || (ComposeResources{NetworkIDs: []string{"n"}}).Gone() {
		t.Error("Gone requires empty containers and networks")
	}
}

func TestComposeDownVerifiesWithProbe(t *testing.T) {
	probe := map[string]string{}
	r := &fakeRun{answer: func(c execx.Cmd) (execx.Result, error) {
		args := strings.Join(c.Args, " ")
		switch {
		case strings.HasPrefix(args, "compose"):
			return execx.Result{ExitCode: 1, Stderr: []byte("configuration file not found: /tmp/missing.yml")}, exitErr(1)
		case strings.HasPrefix(args, "ps"):
			return execx.Result{Stdout: []byte(probe["ps"])}, nil
		case strings.HasPrefix(args, "network ls"):
			if probe["net"] == "fail" {
				return execx.Result{ExitCode: 1}, exitErr(1)
			}
			return execx.Result{Stdout: []byte(probe["net"])}, nil
		}
		return execx.Result{}, nil
	}}
	d := newTestDocker(&fakeEngine{}, r)
	c := ComposeCmd{Project: "dl-app-aaaaaaaa", Args: []string{"down", "--remove-orphans"}}

	res := d.ComposeDown(context.Background(), c)
	if !res.OK || res.Stderr == "" {
		t.Fatalf("nonzero exit with empty probe is OK: %+v", res)
	}
	if got := r.Args(); got[1] != "docker ps -aq --filter label=com.docker.compose.project=dl-app-aaaaaaaa" ||
		got[2] != "docker network ls -q --filter label=com.docker.compose.project=dl-app-aaaaaaaa" || r.calls[0].Timeout != 60*time.Second {
		t.Fatalf("calls %v", got)
	}

	probe["ps"] = "c1\r\n\n c2 \n"
	res = d.ComposeDown(context.Background(), c)
	if res.OK || res.Detail() != "configuration file not found: /tmp/missing.yml" {
		t.Fatalf("leftover containers: %+v", res)
	}
	probe["ps"], probe["net"] = "", "fail"
	if d.ComposeDown(context.Background(), c).OK {
		t.Fatal("a failed probe must never prove absence")
	}
	if got := d.ProbeComposeProject(context.Background(), "x"); !reflect.DeepEqual(got.NetworkIDs, []string{ProbeFailed}) {
		t.Fatalf("probe %+v", got)
	}
	if got := (DownResult{}).Detail(); got != "compose down failed" {
		t.Fatalf("detail %q", got)
	}
}

func TestVolumeAndPruneCommands(t *testing.T) {
	r := &fakeRun{answer: func(c execx.Cmd) (execx.Result, error) {
		if c.Args[0] == "volume" && c.Args[1] == "create" {
			return execx.Result{ExitCode: 1, Stderr: []byte("  permission denied\n")}, exitErr(1)
		}
		if c.Args[0] == "builder" {
			return execx.Result{Stdout: []byte("Total reclaimed space: 0B\n")}, nil
		}
		return execx.Result{}, nil
	}}
	d := newTestDocker(&fakeEngine{}, r)
	err := d.VolumeCreate(context.Background(), "dl-aaaaaaaa-data-1234", "com.docker.compose.project=dl-x-aaaaaaaa", "com.docklift.project=id")
	if err == nil || err.Error() != "permission denied" {
		t.Fatalf("err %v", err)
	}
	if r.Args()[0] != "docker volume create --label com.docker.compose.project=dl-x-aaaaaaaa --label com.docklift.project=id dl-aaaaaaaa-data-1234" {
		t.Fatalf("args %v", r.Args())
	}
	if got := d.BuilderPrune(context.Background(), true); !got.OK || got.Output != "Total reclaimed space: 0B" {
		t.Fatalf("prune %+v", got)
	}
	d.RemoveContainer(context.Background(), "dl_x")
	d.RemoveContainer(context.Background(), "")
	if _, err := d.VolumeRemove(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	want := []string{"docker builder prune -af", "docker rm -f dl_x", "docker volume rm -f v"}
	if got := r.Args()[1:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("args %v", got)
	}
	r.answer = func(execx.Cmd) (execx.Result, error) {
		return execx.Result{ExitCode: -1}, &execx.StartError{Name: "docker", Err: errors.New("executable file not found")}
	}
	if got := d.BuilderPrune(context.Background(), false); got.OK || got.Output != "executable file not found" {
		t.Fatalf("prune %+v", got)
	}
}

func TestImages(t *testing.T) {
	e := &fakeEngine{
		imageList: func() (client.ImageListResult, error) {
			return client.ImageListResult{Items: []image.Summary{
				{ID: "sha256:1", RepoTags: []string{"docklift-abcdef12-api:aaaaaaaa", "<none>:<none>"}},
				{ID: "sha256:2"},
			}}, nil
		},
		imageInspect: func(ref string) error {
			if ref == "present:1" {
				return nil
			}
			return notFoundErr{"No such image"}
		},
		imageRemove: func(ref string, o client.ImageRemoveOptions) error {
			if !o.Force {
				t.Error("remove must force")
			}
			switch ref {
			case "gone:1":
				return errors.New("Error response from daemon: No such image: gone:1")
			case "busy:1":
				return errors.New("conflict: unable to remove repository reference")
			}
			return nil
		},
	}
	d := newTestDocker(e, nil)
	imgs, err := d.ListImages(context.Background())
	if err != nil || len(imgs) != 2 || !reflect.DeepEqual(imgs[0].Tags, []string{"docklift-abcdef12-api:aaaaaaaa"}) || len(imgs[1].Tags) != 0 {
		t.Fatalf("images %+v %v", imgs, err)
	}
	if !d.ImageExists(context.Background(), "present:1") || d.ImageExists(context.Background(), "absent:1") {
		t.Fatal("ImageExists")
	}
	if ok, err := d.RemoveImage(context.Background(), "x:1"); !ok || err != nil {
		t.Fatalf("remove %v %v", ok, err)
	}
	if ok, err := d.RemoveImage(context.Background(), "gone:1"); ok || err != nil {
		t.Fatalf("not found must be (false, nil): %v %v", ok, err)
	}
	if _, err := d.RemoveImage(context.Background(), "busy:1"); err == nil {
		t.Fatal("conflict must be an error")
	}
}

func TestPingAndUnavailable(t *testing.T) {
	if err := New(nil, "", "", nil).Ping(context.Background()); !IsUnavailable(err) {
		t.Fatalf("err %v", err)
	}
	if err := newTestDocker(&fakeEngine{}, nil).Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if IsUnavailable(errBoom) || !IsNotFound(notFoundErr{"x"}) || !IsNotFound(errors.New("No such image: y")) || IsNotFound(nil) {
		t.Fatal("error classifiers")
	}
}
