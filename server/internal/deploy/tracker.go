package deploy

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cancelOwnerPrefix marks a project lock held by a cancel teardown.
const cancelOwnerPrefix = "cancel:"

// Tracker is the in-memory deploy/cancel state of every project, covering the
// deployment states plus the active-build and cancelled-deployment
// registries. One Tracker is shared by every route
// group of the process. It is safe for concurrent use.
//
// The project lock is owned by a deployment id (or a cancel sentinel), so a
// cancelled deploy's cleanup cannot release a newer deploy's lock, nor a
// cancel teardown still in progress.
type Tracker struct {
	mu        sync.Mutex
	owners    map[string]string
	builds    map[string]*activeBuild
	cancelled map[string]bool
	now       func() time.Time
}

type activeBuild struct {
	deploymentID string
	cancelled    bool
	cancel       context.CancelFunc
	done         chan struct{}
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{
		owners:    map[string]string{},
		builds:    map[string]*activeBuild{},
		cancelled: map[string]bool{},
		now:       time.Now,
	}
}

// IsDeploying reports whether any deploy or cancel holds the project lock.
func (t *Tracker) IsDeploying(projectID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.owners[projectID]
	return ok
}

// IsCancelling reports whether a cancel teardown holds the project lock.
func (t *Tracker) IsCancelling(projectID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.HasPrefix(t.owners[projectID], cancelOwnerPrefix)
}

// Acquire takes the project lock for deploymentID (re-acquiring by the same
// id succeeds). It returns ok=false when another deploy or a cancel owns it;
// otherwise release drops the lock only while deploymentID still owns it, so
// it is safe to defer and to call more than once.
func (t *Tracker) Acquire(projectID, deploymentID string) (release func(), ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur, held := t.owners[projectID]; held && cur != deploymentID {
		return func() {}, false
	}
	t.owners[projectID] = deploymentID
	return func() { t.Release(projectID, deploymentID) }, true
}

// Owner returns the current lock owner (a deployment id or "cancel:<ms>").
func (t *Tracker) Owner(projectID string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	o, ok := t.owners[projectID]
	return o, ok
}

// Owns reports whether deploymentID still holds the project lock (not
// stolen by a cancel or released).
func (t *Tracker) Owns(projectID, deploymentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.owners[projectID] == deploymentID
}

// Release drops the lock only if deploymentID still owns it.
func (t *Tracker) Release(projectID, deploymentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.owners[projectID] == deploymentID {
		delete(t.owners, projectID)
	}
}

// Clear drops the project lock whoever owns it (end of a cancel teardown).
func (t *Tracker) Clear(projectID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.owners, projectID)
}

// BeginCancel makes a cancel teardown own the project lock for its whole
// duration; an in-flight deploy's Release then does nothing. It returns the
// deployment id it took the lock from ("" when the lock was free or held by
// another cancel) and end, which clears the lock (call it when the teardown
// finished or failed).
func (t *Tracker) BeginCancel(projectID string) (stolen string, end func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev := t.owners[projectID]
	t.owners[projectID] = cancelOwnerPrefix + strconv.FormatInt(t.now().UnixMilli(), 10)
	if prev != "" && !strings.HasPrefix(prev, cancelOwnerPrefix) {
		stolen = prev
	}
	return stolen, func() { t.Clear(projectID) }
}

// StartProcess registers the long-running command (pull, build, compose up)
// that deploymentID is about to run for the project and returns the context
// the command must run under, plus finish, which the caller must call once
// the command has exited (it unregisters the command and releases the
// context; calling it more than once is harmless).
//
// Registering kills (cancels) a command still registered by another
// deployment of the project and marks that one cancelled (registerBuild).
// The returned context is already cancelled when deploymentID has been
// marked cancelled, so a cancel that raced ahead of the registration still
// wins.
func (t *Tracker) StartProcess(parent context.Context, projectID, deploymentID string) (ctx context.Context, finish func()) {
	ctx, cancel := context.WithCancel(parent)
	b := &activeBuild{deploymentID: deploymentID, cancel: cancel, done: make(chan struct{})}
	t.mu.Lock()
	if prev := t.builds[projectID]; prev != nil && prev.deploymentID != deploymentID {
		prev.cancelled = true
		prev.cancel()
	}
	t.builds[projectID] = b
	if t.cancelled[deploymentID] {
		b.cancelled = true
		cancel()
	}
	t.mu.Unlock()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			t.mu.Lock()
			if t.builds[projectID] == b {
				delete(t.builds, projectID)
			}
			t.mu.Unlock()
			cancel()
			close(b.done)
		})
	}
}

// ActiveDeployment returns the deployment whose command is registered for
// the project, if any.
func (t *Tracker) ActiveDeployment(projectID string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b := t.builds[projectID]; b != nil {
		return b.deploymentID, true
	}
	return "", false
}

// ClearProcess unregisters deploymentID's command for the project without
// killing it (clearBuild); a newer deployment's registration is kept.
func (t *Tracker) ClearProcess(projectID, deploymentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b := t.builds[projectID]; b != nil && b.deploymentID == deploymentID {
		delete(t.builds, projectID)
	}
}

// KillActive marks the project's registered command cancelled and kills it
// (killComposeProcess). With wait > 0 it then waits up to wait for the
// command to exit. It reports whether a command was registered.
func (t *Tracker) KillActive(projectID string, wait time.Duration) bool {
	t.mu.Lock()
	b := t.builds[projectID]
	if b != nil {
		b.cancelled = true
		b.cancel()
	}
	t.mu.Unlock()
	if b == nil {
		return false
	}
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-b.done:
		case <-timer.C:
		}
	}
	return true
}

// WasBuildCancelled reports whether deploymentID was cancelled: marked
// cancelled, superseded by another deployment's command, or its own command
// killed (wasBuildCancelled).
func (t *Tracker) WasBuildCancelled(projectID, deploymentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled[deploymentID] {
		return true
	}
	b := t.builds[projectID]
	if b == nil {
		return false
	}
	return b.deploymentID != deploymentID || b.cancelled
}

// MarkCancelled records that deploymentID was cancelled (cancelledDeployments.add).
func (t *Tracker) MarkCancelled(deploymentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancelled[deploymentID] = true
}

// IsMarkedCancelled reports whether MarkCancelled was called for deploymentID.
func (t *Tracker) IsMarkedCancelled(deploymentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled[deploymentID]
}

// ForgetCancelled drops the cancelled mark once the deployment finished
// (cancelledDeployments.delete).
func (t *Tracker) ForgetCancelled(deploymentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cancelled, deploymentID)
}
