package store_test

import (
	"context"
	"testing"

	"docklift/internal/app/apptest"
	"docklift/internal/store"
)

func TestProjectUsesPort(t *testing.T) {
	d := apptest.NewDeps(t)
	ctx := context.Background()
	if used, err := d.Store.ProjectUsesPort(ctx, 5501); err != nil || used {
		t.Fatalf("empty db: %v %v", used, err)
	}
	if _, err := d.Store.CreateProject(ctx, store.ProjectCreate{Name: "p", Port: store.Ptr(5501)}); err != nil {
		t.Fatal(err)
	}
	if used, err := d.Store.ProjectUsesPort(ctx, 5501); err != nil || !used {
		t.Errorf("5501: %v %v", used, err)
	}
	if used, err := d.Store.ProjectUsesPort(ctx, 5502); err != nil || used {
		t.Errorf("5502: %v %v", used, err)
	}
}
