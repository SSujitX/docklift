package main

import (
	"reflect"
	"testing"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
)

// TestRegisteredLifecycleHooks pins every startup and shutdown hook the
// process registers, so a route group whose hook is never reached (or is
// registered twice) fails here.
func TestRegisteredLifecycleHooks(t *testing.T) {
	d := apptest.NewDeps(t)
	newHandler(d)

	want := map[app.Phase][]string{
		app.PhaseDockerNetwork: {"docker-network", "backup-restore-recovery"},
		app.PhaseAfterListen:   {"nginx-sync"},
		app.PhaseRecovery:      {"deployment-recovery"},
		app.PhaseWatchers:      {"cert-renew-watcher"},
	}
	for _, phase := range []app.Phase{app.PhaseDockerNetwork, app.PhaseAfterListen, app.PhaseRecovery, app.PhaseWatchers} {
		var got []string
		for _, h := range d.Lifecycle.Hooks(phase) {
			got = append(got, h.Name)
		}
		if !reflect.DeepEqual(got, want[phase]) {
			t.Errorf("%s hooks = %q, want %q", phase, got, want[phase])
		}
	}

	var down []string
	for _, h := range d.Lifecycle.ShutdownHooks() {
		down = append(down, h.Name)
	}
	if want := []string{"terminal sessions", "deployments"}; !reflect.DeepEqual(down, want) {
		t.Errorf("shutdown hooks = %q, want %q", down, want)
	}
}
