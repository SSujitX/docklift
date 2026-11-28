package logs

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	"docklift/internal/httpx"
)

// sseWriteTimeout bounds one event write, so a stalled client cannot pin the
// stream goroutine and the Docker log connection forever.
const sseWriteTimeout = 30 * time.Second

type sseEvent struct {
	Type      string `json:"type"`
	Message   string `json:"message,omitempty"`
	Container string `json:"container,omitempty"`
}

// sse writes `data: <json>\n\n` events and flushes each one. After the
// first failed write (client gone) every later write is a no-op.
type sse struct {
	w      http.ResponseWriter
	rc     *http.ResponseController
	failed bool
}

func (s *sse) send(ev sseEvent) {
	if s.failed {
		return
	}
	body, err := httpx.Marshal(ev)
	if err != nil {
		s.failed = true
		return
	}
	_ = s.rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
	if _, err := io.WriteString(s.w, "data: "+string(body)+"\n\n"); err != nil {
		s.failed = true
		return
	}
	if err := s.rc.Flush(); err != nil {
		s.failed = true
	}
}

// logWriter turns every demultiplexed frame (or raw TTY chunk) into one
// `log` event, skipping whitespace-only payloads.
type logWriter struct{ s *sse }

var errClientGone = errors.New("client disconnected")

func (l logWriter) Write(p []byte) (int, error) {
	if l.s.failed {
		return 0, errClientGone
	}
	if text := string(p); strings.TrimSpace(text) != "" {
		l.s.send(sseEvent{Type: "log", Message: text})
		if l.s.failed {
			return 0, errClientGone
		}
	}
	return len(p), nil
}

// Stream is an SSE response that
// reports `status` when the container is not running, otherwise `connected`
// followed by one `log` event per log frame (stdout and stderr, with Docker
// timestamps) until the stream `end`s or fails (`error`). Docker errors are
// events on a 200 stream, as in 2.x: the dashboard's EventSource shows
// their message, which a non-200 status would hide. The stream stops when
// the client disconnects.
func Stream(w http.ResponseWriter, r *http.Request, docker Docker, name, tail string) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &sse{w: w, rc: http.NewResponseController(w)}
	ctx := r.Context()

	if docker == nil {
		s.send(sseEvent{Type: "error", Message: "Container not found: " + ErrDockerNotConfigured.Error()})
		return
	}
	info, err := docker.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		s.send(sseEvent{Type: "error", Message: "Container not found: " + err.Error()})
		return
	}
	if info.Container.State == nil || !info.Container.State.Running {
		s.send(sseEvent{Type: "status", Message: "Container is not running"})
		return
	}
	stream, err := docker.ContainerLogs(ctx, name, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true, Tail: tail, Timestamps: true,
	})
	if err != nil {
		s.send(sseEvent{Type: "error", Message: "Failed to stream logs: " + err.Error()})
		return
	}
	defer stream.Close()

	s.send(sseEvent{Type: "connected", Container: name})
	out := logWriter{s: s}
	if info.Container.Config != nil && info.Container.Config.Tty {
		_, err = io.CopyBuffer(out, stream, make([]byte, 32*1024))
	} else {
		_, err = stdcopy.StdCopy(out, out, stream)
	}
	if s.failed || ctx.Err() != nil {
		return
	}
	if err != nil {
		s.send(sseEvent{Type: "error", Message: err.Error()})
		return
	}
	s.send(sseEvent{Type: "end", Message: "Log stream ended"})
}
