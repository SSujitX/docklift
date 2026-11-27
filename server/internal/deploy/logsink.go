package deploy

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"docklift/internal/store"
)

// DeployLogSyncInterval is how often streamed deploy logs are persisted, so
// Project Detail polling sees progress during long pulls and builds.
const DeployLogSyncInterval = 2 * time.Second

// DeployLog is the writeLog of the deploy/stop/restart/rollback routes: every
// write goes to the client stream (errors ignored — the client may be gone),
// is accumulated, and the deployment row's logs are updated at most every
// DeployLogSyncInterval. It is safe for concurrent use and its Write never
// fails, so it can receive command output directly.
type DeployLog struct {
	ctx          context.Context
	store        *store.Store
	deploymentID string
	client       io.Writer

	mu       sync.Mutex
	buf      strings.Builder
	lastSync time.Time
	now      func() time.Time
}

// NewDeployLog streams to client (nil = none) and persists into the
// deployment row (store nil or deploymentID "" = never persist). ctx bounds
// the database writes.
func NewDeployLog(ctx context.Context, st *store.Store, deploymentID string, client io.Writer) *DeployLog {
	return &DeployLog{ctx: ctx, store: st, deploymentID: deploymentID, client: client, lastSync: time.Now(), now: time.Now}
}

// Write records p, streams it and periodically persists the logs.
func (l *DeployLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.client != nil {
		if _, err := l.client.Write(p); err == nil {
			if f, ok := l.client.(interface{ Flush() }); ok {
				f.Flush()
			}
		}
	}
	l.buf.Write(p)
	if l.now().Sub(l.lastSync) >= DeployLogSyncInterval {
		l.syncLocked()
	}
	return len(p), nil
}

// WriteString writes s (convenience for log lines).
func (l *DeployLog) WriteString(s string) {
	_, _ = l.Write([]byte(s))
}

// Sync persists the logs now (syncLogsToDb(true)).
func (l *DeployLog) Sync() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
}

func (l *DeployLog) syncLocked() {
	l.lastSync = l.now()
	if l.store == nil || l.deploymentID == "" {
		return
	}
	logs := l.buf.String()
	_, _ = l.store.UpdateDeployment(l.ctx, l.deploymentID, store.DeploymentUpdate{Logs: store.Some(&logs)})
}

// String returns everything written so far (logs.join(”)).
func (l *DeployLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}
