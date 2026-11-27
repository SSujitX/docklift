package dockerx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/moby/moby/client"
)

// LogEvent is one Server-Sent Event of the container log stream, with the
// shapes streamContainerLogs wrote: {type, message} or {type: "connected",
// container}.
type LogEvent struct {
	Type      string
	Message   string
	Container string
}

// Log stream event types.
const (
	LogEventStatus    = "status"
	LogEventConnected = "connected"
	LogEventLog       = "log"
	LogEventEnd       = "end"
	LogEventError     = "error"
)

// MarshalJSON writes the exact key set and order of the TS events.
func (e LogEvent) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	var err error
	if e.Type == LogEventConnected {
		err = enc.Encode(struct {
			Type      string `json:"type"`
			Container string `json:"container"`
		}{e.Type, e.Container})
	} else {
		err = enc.Encode(struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{e.Type, e.Message})
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), err
}

// SSEFrame renders the event as `data: <json>\n\n`.
func (e LogEvent) SSEFrame() []byte {
	b, _ := e.MarshalJSON()
	return append(append([]byte("data: "), b...), '\n', '\n')
}

// StreamContainerLogs follows a running container's logs (stdout+stderr with
// timestamps, starting tail lines back) and hands each event to emit, in the
// order streamContainerLogs produced them: an error or "not running" status
// and stop; otherwise "connected", one "log" per non-blank frame (or raw
// chunk for TTY containers), then "end" or "error" when the stream closes.
// It returns when the stream ends, emit returns an error (client gone), or
// ctx is cancelled (client disconnected; nothing more is emitted). The
// caller sets the SSE headers (text/event-stream, no-cache, keep-alive,
// X-Accel-Buffering: no) and flushes after each event.
func (d *Docker) StreamContainerLogs(ctx context.Context, name string, tail int, emit func(LogEvent) error) error {
	if d.Engine == nil {
		return emit(LogEvent{Type: LogEventError, Message: "Container not found: " + ErrUnavailable.Error()})
	}
	info, err := d.Engine.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return emit(LogEvent{Type: LogEventError, Message: "Container not found: " + err.Error()})
	}
	if info.Container.State == nil || !info.Container.State.Running {
		return emit(LogEvent{Type: LogEventStatus, Message: "Container is not running"})
	}
	tty := info.Container.Config != nil && info.Container.Config.Tty
	rc, err := d.Engine.ContainerLogs(ctx, name, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true, Timestamps: true, Tail: strconv.Itoa(tail),
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return emit(LogEvent{Type: LogEventError, Message: "Failed to stream logs: " + err.Error()})
	}
	defer rc.Close()
	if err := emit(LogEvent{Type: LogEventConnected, Container: name}); err != nil {
		return err
	}
	return pumpLogs(ctx, rc, tty, emit)
}

// pumpLogs reads the log stream and emits events until EOF or an error.
func pumpLogs(ctx context.Context, r io.Reader, tty bool, emit func(LogEvent) error) error {
	var pending []byte
	chunk := make([]byte, 32*1024)
	sendText := func(b []byte) error {
		text := decodeUTF8(b)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return emit(LogEvent{Type: LogEventLog, Message: text})
	}
	for {
		n, readErr := r.Read(chunk)
		if n > 0 {
			if tty {
				if err := sendText(chunk[:n]); err != nil {
					return err
				}
			} else {
				pending = append(pending, chunk[:n]...)
				for _, frame := range demuxFrames(&pending) {
					if err := sendText(frame); err != nil {
						return err
					}
				}
			}
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(readErr, io.EOF) {
				return emit(LogEvent{Type: LogEventEnd, Message: "Log stream ended"})
			}
			return emit(LogEvent{Type: LogEventError, Message: readErr.Error()})
		}
	}
}

// decodeUTF8 converts bytes to text like Buffer.toString('utf-8'): every
// invalid byte becomes U+FFFD.
func decodeUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		sb.WriteRune(r)
		b = b[size:]
	}
	return sb.String()
}
