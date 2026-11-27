package deployments

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"docklift/internal/store"
)

// redactMask replaces a secret in deploy logs.
const redactMask = "***"

// minSecretLen is the shortest value redacted from logs; shorter values
// would mangle ordinary output.
const minSecretLen = 4

// redactor masks secrets in a byte stream whose writes may split a secret:
// it holds back the last len(longest secret)-1 bytes until the next write or
// flush.
type redactor struct {
	secrets [][]byte
	first   [256]bool
	maxLen  int
	pending []byte
}

// add registers secret values (short and duplicate values are ignored).
func (r *redactor) add(values ...string) {
	for _, v := range values {
		if len(v) < minSecretLen {
			continue
		}
		dup := false
		for _, s := range r.secrets {
			if string(s) == v {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		r.secrets = append(r.secrets, []byte(v))
		r.first[v[0]] = true
		r.maxLen = max(r.maxLen, len(v))
	}
	// Longest first, so a secret containing another is masked whole.
	sort.SliceStable(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
}

// write returns the part of pending+p that is safe to emit.
func (r *redactor) write(p []byte) []byte {
	if len(r.secrets) == 0 {
		return p
	}
	buf := append(r.pending, p...)
	out, rest := r.scan(buf, len(buf)-(r.maxLen-1))
	r.pending = append([]byte(nil), rest...)
	return out
}

// flush returns everything held back, redacted.
func (r *redactor) flush() []byte {
	if len(r.pending) == 0 {
		return nil
	}
	out, _ := r.scan(r.pending, len(r.pending))
	r.pending = nil
	return out
}

// scan redacts buf up to cut: secrets starting before cut are masked (even
// when they extend past it); the unscanned remainder is returned as rest.
func (r *redactor) scan(buf []byte, cut int) (out, rest []byte) {
	var b bytes.Buffer
	i := 0
	for i < cut {
		matched := false
		if r.first[buf[i]] {
			for _, s := range r.secrets {
				if bytes.HasPrefix(buf[i:], s) {
					b.WriteString(redactMask)
					i += len(s)
					matched = true
					break
				}
			}
		}
		if !matched {
			b.WriteByte(buf[i])
			i++
		}
	}
	return b.Bytes(), buf[i:]
}

// clientStreamLimit bounds output buffered for a slow client; beyond it the
// client misses output (the deployment row keeps the full log).
const clientStreamLimit = 8 << 20

// clientStream decouples a pipeline from the HTTP client: Write never blocks
// and never fails; pump copies the buffered output to the response. Once the
// client is gone, writes are dropped.
type clientStream struct {
	mu       sync.Mutex
	buf      []byte
	detached bool
	notify   chan struct{}
}

func newClientStream() *clientStream {
	return &clientStream{notify: make(chan struct{}, 1)}
}

func (c *clientStream) Write(p []byte) (int, error) {
	c.mu.Lock()
	if !c.detached && len(c.buf)+len(p) <= clientStreamLimit {
		c.buf = append(c.buf, p...)
	}
	c.mu.Unlock()
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (c *clientStream) take() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.buf
	c.buf = nil
	return b
}

func (c *clientStream) detach() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.detached = true
	c.buf = nil
}

// streamHeaders are the headers of the plain-text progress streams.
func streamHeaders(w http.ResponseWriter, keepAlive bool) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache, no-transform")
	if keepAlive {
		h.Set("Connection", "keep-alive")
	}
}

// pump sends the status line and headers immediately, then copies c to the
// response until done is closed (after which it drains c) or the client goes
// away (after which c drops further output). The work behind c keeps running
// in both cases.
func pump(w http.ResponseWriter, r *http.Request, c *clientStream, done <-chan struct{}) {
	rc := http.NewResponseController(w)
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()
	send := func() bool {
		b := c.take()
		if len(b) == 0 {
			return true
		}
		if _, err := w.Write(b); err != nil {
			c.detach()
			return false
		}
		_ = rc.Flush()
		return true
	}
	for {
		select {
		case <-c.notify:
			if !send() {
				return
			}
		case <-done:
			send()
			return
		case <-r.Context().Done():
			c.detach()
			return
		}
	}
}

// deployLog is writeLog of the deploy, stop, restart and rollback routes:
// output is redacted, accumulated for the deployment row, and streamed to
// the client without ever blocking. When persisting, a syncer goroutine
// writes the accumulated logs to the row every logSyncInterval so polling
// sees progress (syncLogsToDb), without the pipeline waiting on the
// database.
type deployLog struct {
	mu     sync.Mutex
	buf    strings.Builder
	red    redactor
	client io.Writer

	st           *store.Store
	deploymentID string
	syncCtx      context.Context
	stopSync     chan struct{}
	syncDone     chan struct{}
	syncMu       sync.Mutex
}

// logSyncInterval is how often streamed deploy logs are persisted.
const logSyncInterval = 2 * time.Second

func newDeployLog(client io.Writer) *deployLog {
	if client == nil {
		client = io.Discard
	}
	return &deployLog{client: client}
}

// redact registers secret values to mask from now on.
func (l *deployLog) redact(values ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.red.add(values...)
}

// Write records p and streams it. It never fails and never blocks on the
// client or the database.
func (l *deployLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	out := l.red.write(p)
	l.emitLocked(out, true)
	l.mu.Unlock()
	return len(p), nil
}

func (l *deployLog) emitLocked(out []byte, toClient bool) {
	if len(out) == 0 {
		return
	}
	l.buf.Write(out)
	if toClient {
		_, _ = l.client.Write(out)
	}
}

// line writes s after flushing held-back output, so Docklift's own lines
// appear immediately.
func (l *deployLog) line(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emitLocked(l.red.flush(), true)
	l.emitLocked(l.red.write([]byte(s)), true)
	l.emitLocked(l.red.flush(), true)
}

// record appends s to the persisted log only (logs.push without res.write).
func (l *deployLog) record(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emitLocked(l.red.flush(), true)
	l.emitLocked(l.red.write([]byte(s)), false)
	l.emitLocked(l.red.flush(), false)
}

// toClient streams s to the client only (res.write without logs.push).
func (l *deployLog) toClient(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emitLocked(l.red.flush(), true)
	out, _ := l.red.scan([]byte(s), len(s))
	_, _ = l.client.Write(out)
}

// String returns the accumulated log (logs.join(”)). Output held back by
// the redactor is not included: flushing it while a command still writes
// could expose a secret split across two writes.
func (l *deployLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// final flushes held-back output (call it once no command writes any more)
// and returns the whole log.
func (l *deployLog) final() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emitLocked(l.red.flush(), true)
	return l.buf.String()
}

// startSync persists the log into the deployment row every logSyncInterval
// until stopSync.
func (l *deployLog) startSync(ctx context.Context, st *store.Store, deploymentID string) {
	l.st, l.deploymentID, l.syncCtx = st, deploymentID, ctx
	l.stopSync, l.syncDone = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(l.syncDone)
		t := time.NewTicker(logSyncInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.syncNow()
			case <-l.stopSync:
				return
			}
		}
	}()
}

// syncNow persists the log now (syncLogsToDb(true)). Errors are ignored like
// the TS .catch.
func (l *deployLog) syncNow() {
	l.syncMu.Lock()
	defer l.syncMu.Unlock()
	if l.st == nil {
		return
	}
	logs := l.String()
	_, _ = l.st.UpdateDeployment(l.syncCtx, l.deploymentID, store.DeploymentUpdate{Logs: store.Some(&logs)})
}

// endSync stops the syncer and waits for it, so no periodic write can land
// after the final status write. It is safe to call more than once.
func (l *deployLog) endSync() {
	if l.stopSync == nil {
		return
	}
	select {
	case <-l.stopSync:
	default:
		close(l.stopSync)
	}
	<-l.syncDone
	l.syncMu.Lock()
	l.st = nil
	l.syncMu.Unlock()
}
