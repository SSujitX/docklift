package deploy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTrackerLockOwnership(t *testing.T) {
	tr := NewTracker()
	release, ok := tr.Acquire("p", "d1")
	if !ok || !tr.IsDeploying("p") || tr.IsCancelling("p") || !tr.Owns("p", "d1") {
		t.Fatal("acquire")
	}
	if _, ok := tr.Acquire("p", "d2"); ok {
		t.Fatal("second deploy must not acquire")
	}
	if _, ok := tr.Acquire("p", "d1"); !ok {
		t.Fatal("re-acquire by the same deployment succeeds")
	}
	tr.Release("p", "d2")
	if !tr.Owns("p", "d1") {
		t.Fatal("a non-owner release must be a no-op")
	}
	release()
	release()
	if tr.IsDeploying("p") {
		t.Fatal("release")
	}
	if owner, ok := tr.Owner("p"); ok || owner != "" {
		t.Fatal("owner after release")
	}
}

func TestTrackerCancelStealsLock(t *testing.T) {
	tr := NewTracker()
	tr.now = func() time.Time { return time.UnixMilli(1700000000123) }
	release, _ := tr.Acquire("p", "d1")
	stolen, end := tr.BeginCancel("p")
	if stolen != "d1" || !tr.IsCancelling("p") || tr.Owns("p", "d1") {
		t.Fatalf("stolen=%q", stolen)
	}
	if owner, _ := tr.Owner("p"); owner != "cancel:1700000000123" {
		t.Fatalf("owner %q", owner)
	}
	// The cancelled deploy's cleanup must not unlock the teardown.
	release()
	if !tr.IsCancelling("p") {
		t.Fatal("cancelled deploy released the cancel lock")
	}
	if _, ok := tr.Acquire("p", "d2"); ok {
		t.Fatal("redeploy during cancel")
	}
	// A second cancel does not report a deployment as stolen.
	if again, _ := tr.BeginCancel("p"); again != "" {
		t.Fatalf("stolen from cancel %q", again)
	}
	end()
	if tr.IsDeploying("p") {
		t.Fatal("end must clear the lock")
	}
	if stolen, end := tr.BeginCancel("idle"); stolen != "" {
		t.Fatal("idle cancel steals nothing")
	} else {
		end()
	}
	tr.Acquire("q", "x")
	tr.Clear("q")
	if tr.IsDeploying("q") {
		t.Fatal("clear")
	}
}

func TestTrackerAcquireIsExclusiveUnderContention(t *testing.T) {
	tr := NewTracker()
	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 64 {
		wg.Go(func() {
			<-start
			if _, ok := tr.Acquire("p", "d"+itoa(i)); ok {
				wins.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d deployments acquired the lock", wins.Load())
	}
}

func TestTrackerProcessRegistry(t *testing.T) {
	tr := NewTracker()
	ctx1, finish1 := tr.StartProcess(context.Background(), "p", "d1")
	if id, ok := tr.ActiveDeployment("p"); !ok || id != "d1" || tr.WasBuildCancelled("p", "d1") {
		t.Fatal("registered")
	}
	// A newer deployment's command kills the older one.
	ctx2, finish2 := tr.StartProcess(context.Background(), "p", "d2")
	select {
	case <-ctx1.Done():
	default:
		t.Fatal("older command not killed")
	}
	if !tr.WasBuildCancelled("p", "d1") || tr.WasBuildCancelled("p", "d2") {
		t.Fatal("superseded deployment must read as cancelled")
	}
	// The old command's finish must not unregister the newer one.
	finish1()
	if id, _ := tr.ActiveDeployment("p"); id != "d2" {
		t.Fatalf("active %q", id)
	}
	tr.ClearProcess("p", "d1")
	if id, _ := tr.ActiveDeployment("p"); id != "d2" {
		t.Fatal("ClearProcess by a non-owner")
	}
	if ctx2.Err() != nil {
		t.Fatal("newer command cancelled")
	}
	finish2()
	finish2()
	if _, ok := tr.ActiveDeployment("p"); ok || ctx2.Err() == nil {
		t.Fatal("finish must unregister and release the context")
	}
	if tr.KillActive("p", 0) {
		t.Fatal("nothing to kill")
	}
}

func TestTrackerCancelBeforeRegistrationWins(t *testing.T) {
	tr := NewTracker()
	tr.MarkCancelled("d1")
	ctx, finish := tr.StartProcess(context.Background(), "p", "d1")
	defer finish()
	if ctx.Err() == nil || !tr.WasBuildCancelled("p", "d1") || !tr.IsMarkedCancelled("d1") {
		t.Fatal("a command registered after cancel must start cancelled")
	}
	tr.ForgetCancelled("d1")
	if tr.IsMarkedCancelled("d1") {
		t.Fatal("forget")
	}
}

func TestTrackerKillActiveWaitsForExit(t *testing.T) {
	tr := NewTracker()
	ctx, finish := tr.StartProcess(context.Background(), "p", "d1")
	exited := make(chan struct{})
	go func() {
		<-ctx.Done() // the "command" exits when killed
		finish()
		close(exited)
	}()
	if !tr.KillActive("p", 10*time.Second) {
		t.Fatal("kill")
	}
	select {
	case <-exited:
	default:
		t.Fatal("KillActive returned before the command exited")
	}

	// A command that ignores the kill: KillActive gives up after wait.
	_, finishStuck := tr.StartProcess(context.Background(), "p", "d2")
	defer finishStuck()
	start := time.Now()
	tr.KillActive("p", 50*time.Millisecond)
	if time.Since(start) > 5*time.Second {
		t.Fatal("wait not bounded")
	}
	if !tr.WasBuildCancelled("p", "d2") {
		t.Fatal("killed command must read as cancelled")
	}
}
