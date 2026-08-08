package userplugin

import (
	"context"
	"errors"
	"testing"

	"github.com/cymonevo/go_template/pkg/store"
)

func TestRepository_FindByUserID(t *testing.T) {
	repo := NewRepository(store.NewMemoryStore[UserPlugin]())
	ctx := context.Background()

	first := &UserPlugin{ID: "1", UserID: "user-1", PluginID: "p1", Enabled: true, SetupStatus: SetupStatusCompleted, Config: map[string]any{}}
	second := &UserPlugin{ID: "2", UserID: "user-1", PluginID: "p2", Enabled: true, SetupStatus: SetupStatusCompleted, Config: map[string]any{}}
	other := &UserPlugin{ID: "3", UserID: "user-2", PluginID: "p1", Enabled: true, SetupStatus: SetupStatusCompleted, Config: map[string]any{}}

	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if err := repo.Create(ctx, other); err != nil {
		t.Fatalf("create other: %v", err)
	}

	got, err := repo.FindByUserID(ctx, "user-1")
	if err != nil {
		t.Fatalf("FindByUserID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 installs for user-1, got %d", len(got))
	}
}

func TestRepository_FindByUserIDAndPluginID(t *testing.T) {
	repo := NewRepository(store.NewMemoryStore[UserPlugin]())
	ctx := context.Background()

	up := &UserPlugin{ID: "1", UserID: "user-1", PluginID: "p1", Enabled: true, SetupStatus: SetupStatusCompleted, Config: map[string]any{}}
	if err := repo.Create(ctx, up); err != nil {
		t.Fatalf("create: %v", err)
	}

	found, err := repo.FindByUserIDAndPluginID(ctx, "user-1", "p1")
	if err != nil {
		t.Fatalf("FindByUserIDAndPluginID: %v", err)
	}
	if found.ID != "1" {
		t.Fatalf("expected id 1, got %q", found.ID)
	}

	_, err = repo.FindByUserIDAndPluginID(ctx, "user-1", "missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
