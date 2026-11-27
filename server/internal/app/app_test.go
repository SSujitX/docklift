package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLifecycleOrderAndErrors(t *testing.T) {
	l := NewLifecycle()
	var calls []string
	record := func(name string, err error) func(context.Context) error {
		return func(context.Context) error {
			calls = append(calls, name)
			return err
		}
	}
	l.On(PhaseRecovery, "first", record("first", nil))
	l.On(PhaseRecovery, "second", record("second", errors.New("boom")))
	l.On(PhaseRecovery, "third", record("third", nil))
	l.On(PhaseWatchers, "other", record("other", nil))

	err := l.Run(context.Background(), PhaseRecovery)
	if err == nil || !strings.Contains(err.Error(), `recovery hook "second": boom`) {
		t.Errorf("err = %v", err)
	}
	if want := []string{"first", "second", "third"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want every hook of the phase in order", calls)
	}
	if err := l.Run(context.Background(), PhaseDockerNetwork); err != nil {
		t.Errorf("empty phase = %v", err)
	}
	if len(l.Hooks(PhaseWatchers)) != 1 {
		t.Error("Hooks")
	}

	var down []string
	l.OnShutdown("a", func(context.Context) { down = append(down, "a") })
	l.OnShutdown("b", func(context.Context) { down = append(down, "b") })
	l.Shutdown(context.Background())
	if !reflect.DeepEqual(down, []string{"a", "b"}) {
		t.Errorf("shutdown order = %v", down)
	}
	for p, want := range map[Phase]string{PhaseDockerNetwork: "docker-network", PhaseAfterListen: "after-listen", PhaseRecovery: "recovery", PhaseWatchers: "watchers", Phase(9): "phase(9)"} {
		if p.String() != want {
			t.Errorf("Phase(%d) = %s", p, p.String())
		}
	}
}
