package logs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/store"
)

// fakeDocker scripts the Engine API.
type fakeDocker struct {
	mu         sync.Mutex
	inspect    map[string]container.InspectResponse
	inspectErr error
	list       []container.Summary
	listErr    error
	logs       func(ctx context.Context) (io.ReadCloser, error)
	logOpts    client.ContainerLogsOptions
}

func (f *fakeDocker) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if f.inspectErr != nil {
		return client.ContainerInspectResult{}, f.inspectErr
	}
	info, ok := f.inspect[id]
	if !ok {
		return client.ContainerInspectResult{}, cerrdefs.ErrNotFound.WithMessage("No such container: " + id)
	}
	return client.ContainerInspectResult{Container: info}, nil
}

func (f *fakeDocker) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{Items: f.list}, f.listErr
}

func (f *fakeDocker) ContainerLogs(ctx context.Context, _ string, opts client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	f.mu.Lock()
	f.logOpts = opts
	f.mu.Unlock()
	return f.logs(ctx)
}

func running(tty bool) container.InspectResponse {
	return container.InspectResponse{State: &container.State{Status: "running", Running: true}, Config: &container.Config{Tty: tty}}
}

func frame(stream byte, payload string) []byte {
	h := make([]byte, 8, 8+len(payload))
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

// connectionRefused returns the error the real client gives for a dead daemon.
var connectionRefused = sync.OnceValue(func() error {
	c, err := client.New(client.WithHost("tcp://127.0.0.1:1"))
	if err != nil {
		panic(err)
	}
	defer c.Close()
	_, err = c.ContainerInspect(context.Background(), "x", client.ContainerInspectOptions{})
	return err
})

func seed(t *testing.T, d *app.Deps) (*store.Project, *store.Service) {
	t.Helper()
	p, err := d.Store.CreateProject(t.Context(), store.ProjectCreate{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.Store.CreateService(t.Context(), store.ServiceCreate{ProjectID: p.ID, Name: "app", DockerfilePath: "Dockerfile",
		ContainerName: store.Ptr("docklift-app-1"), InternalPort: store.Ptr(8080)})
	if err != nil {
		t.Fatal(err)
	}
	return p, s
}

func router(d *app.Deps, docker Docker) http.Handler {
	r := chi.NewRouter()
	MountWith(r, d, docker)
	return r
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestContainers(t *testing.T) {
	d := apptest.NewDeps(t)
	p, svc := seed(t, d)
	empty, _ := d.Store.CreateProject(t.Context(), store.ProjectCreate{Name: "empty"})
	noName, _ := d.Store.CreateProject(t.Context(), store.ProjectCreate{Name: "noname"})
	_, _ = d.Store.CreateService(t.Context(), store.ServiceCreate{ProjectID: noName.ID, Name: "x", DockerfilePath: "Dockerfile"})

	cases := []struct {
		name   string
		docker Docker
		path   string
		code   int
		body   string
	}{
		{"missing project", &fakeDocker{}, "/missing/containers", 404, `{"error":"Project not found"}`},
		{"no services", &fakeDocker{}, "/" + empty.ID + "/containers", 200, `[]`},
		{"no container name needs no docker", nil, "/" + noName.ID + "/containers", 200, ""},
		{"running", &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(false)}}, "/" + p.ID + "/containers", 200,
			`[{"id":"` + svc.ID + `","name":"app","container_name":"docklift-app-1","port":null,"internal_port":8080,"dockerfile_path":"Dockerfile","status":"running","running":true}]`},
		{"partial name match", &fakeDocker{list: []container.Summary{{State: "exited"}}}, "/" + p.ID + "/containers", 200, ""},
		{"not found", &fakeDocker{}, "/" + p.ID + "/containers", 200, ""},
		{"docker unreachable", &fakeDocker{inspectErr: connectionRefused()}, "/" + p.ID + "/containers", 503, `{"error":"Docker is not available"}`},
		{"no docker client", nil, "/" + p.ID + "/containers", 503, `{"error":"Docker is not available"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := get(t, router(d, c.docker), c.path)
			if rec.Code != c.code {
				t.Fatalf("status %d, body %s", rec.Code, rec.Body)
			}
			if c.body != "" && rec.Body.String() != c.body {
				t.Fatalf("body %s\nwant %s", rec.Body, c.body)
			}
		})
	}

	var got []map[string]any
	_ = json.Unmarshal(get(t, router(d, &fakeDocker{list: []container.Summary{{State: "exited"}}}), "/"+p.ID+"/containers").Body.Bytes(), &got)
	if got[0]["status"] != "exited" || got[0]["running"] != false {
		t.Fatalf("partial match: %v", got)
	}
	_ = json.Unmarshal(get(t, router(d, &fakeDocker{}), "/"+p.ID+"/containers").Body.Bytes(), &got)
	if got[0]["status"] != "not_found" {
		t.Fatalf("not found: %v", got)
	}
	_ = json.Unmarshal(get(t, router(d, nil), "/"+noName.ID+"/containers").Body.Bytes(), &got)
	if got[0]["status"] != "unknown" || got[0]["container_name"] != nil {
		t.Fatalf("unnamed service: %v", got)
	}
}

func events(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, f := range strings.Split(body, "\n\n") {
		if f == "" {
			continue
		}
		data, ok := strings.CutPrefix(f, "data: ")
		if !ok {
			t.Fatalf("frame %q", f)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("frame %q: %v", f, err)
		}
		out = append(out, m)
	}
	return out
}

func TestStreamEvents(t *testing.T) {
	d := apptest.NewDeps(t)
	p, _ := seed(t, d)
	stream := "/" + p.ID + "/stream/docklift-app-1"
	muxed := bytes.Join([][]byte{
		frame(1, "2026-01-01T00:00:00Z hello\n"),
		frame(2, "2026-01-01T00:00:01Z oops\n"),
		frame(1, "   \n"),
	}, nil)
	cases := []struct {
		name   string
		docker Docker
		path   string
		want   []map[string]any
	}{
		{"inspect fails", &fakeDocker{}, stream, []map[string]any{{"type": "error", "message": "Container not found: No such container: docklift-app-1"}}},
		{"no docker client", nil, stream, []map[string]any{{"type": "error", "message": "Container not found: docker client is not configured"}}},
		{"stopped", &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": {State: &container.State{Status: "exited"}}}}, stream,
			[]map[string]any{{"type": "status", "message": "Container is not running"}}},
		{"logs fail", &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(false)},
			logs: func(context.Context) (io.ReadCloser, error) { return nil, errors.New("daemon said no") }}, stream,
			[]map[string]any{{"type": "error", "message": "Failed to stream logs: daemon said no"}}},
		{"multiplexed", &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(false)},
			logs: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(muxed)), nil }}, stream + "?tail=5",
			[]map[string]any{
				{"type": "connected", "container": "docklift-app-1"},
				{"type": "log", "message": "2026-01-01T00:00:00Z hello\n"},
				{"type": "log", "message": "2026-01-01T00:00:01Z oops\n"},
				{"type": "end", "message": "Log stream ended"},
			}},
		{"tty", &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(true)},
			logs: func(context.Context) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("\x01raw tty bytes\n")), nil
			}}, stream,
			[]map[string]any{{"type": "connected", "container": "docklift-app-1"}, {"type": "log", "message": "\x01raw tty bytes\n"}, {"type": "end", "message": "Log stream ended"}}},
		{"daemon error frame", &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(false)},
			logs: func(context.Context) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(frame(3, "disk full"))), nil
			}}, stream,
			[]map[string]any{{"type": "connected", "container": "docklift-app-1"}, {"type": "error", "message": "error from daemon in stream: disk full"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := get(t, router(d, c.docker), c.path)
			if rec.Code != 200 {
				t.Fatalf("status %d", rec.Code)
			}
			for k, v := range map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-cache, no-transform", "X-Accel-Buffering": "no", "Connection": "keep-alive"} {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("%s = %q", k, got)
				}
			}
			got := events(t, rec.Body.String())
			gb, _ := json.Marshal(got)
			wb, _ := json.Marshal(c.want)
			if string(gb) != string(wb) {
				t.Fatalf("events %s\nwant   %s", gb, wb)
			}
		})
	}
}

func TestStreamOwnershipAndTail(t *testing.T) {
	d := apptest.NewDeps(t)
	p, _ := seed(t, d)
	rec := get(t, router(d, &fakeDocker{}), "/"+p.ID+"/stream/docklift-backend")
	if rec.Code != 404 || rec.Body.String() != `{"error":"Container not found for this project"}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for query, want := range map[string]string{"": "200", "?tail=abc": "200", "?tail=0": "200", "?tail=50": "50", "?tail=-5": "-5", "?tail=12x": "12", "?tail=1&tail=2": "1"} {
		f := &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(false)},
			logs: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil }}
		get(t, router(d, f), "/"+p.ID+"/stream/docklift-app-1"+query)
		if f.logOpts.Tail != want || !f.logOpts.Follow || !f.logOpts.Timestamps || !f.logOpts.ShowStderr || !f.logOpts.ShowStdout {
			t.Errorf("query %q: options %+v, want tail %s", query, f.logOpts, want)
		}
	}
}

// blockingLogs never ends until its context is cancelled, like a follow
// stream of a quiet container.
type blockingLogs struct {
	ctx    context.Context
	first  chan struct{}
	once   sync.Once
	closed chan struct{}
	close  sync.Once
}

func (b *blockingLogs) Read(p []byte) (int, error) {
	sent := false
	b.once.Do(func() { sent = true })
	if sent {
		close(b.first)
		return copy(p, frame(1, "line\n")), nil
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *blockingLogs) Close() error {
	b.close.Do(func() { close(b.closed) })
	return nil
}

func TestStreamStopsWhenClientDisconnects(t *testing.T) {
	d := apptest.NewDeps(t)
	p, _ := seed(t, d)
	bl := &blockingLogs{first: make(chan struct{}), closed: make(chan struct{})}
	f := &fakeDocker{inspect: map[string]container.InspectResponse{"docklift-app-1": running(false)},
		logs: func(ctx context.Context) (io.ReadCloser, error) { bl.ctx = ctx; return bl, nil }}
	srv := httptest.NewServer(router(d, f))
	defer srv.Close()
	ctx, cancel := context.WithCancel(t.Context())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/"+p.ID+"/stream/docklift-app-1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(resp.Body)
	for _, want := range []string{`"connected"`, `"line\n"`} {
		line, err := r.ReadString('\n')
		if err != nil || !strings.Contains(line, want) {
			t.Fatalf("read %q %v, want %s", line, err, want)
		}
		_, _ = r.ReadString('\n')
	}
	cancel()
	resp.Body.Close()
	select {
	case <-bl.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the Docker log stream was not closed after the client left")
	}
}
