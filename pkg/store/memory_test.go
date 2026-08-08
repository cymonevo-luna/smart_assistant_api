package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memItem struct {
	ID        string    `bson:"_id"`
	Name      string    `bson:"name"`
	Status    string    `bson:"status"`
	Enabled   bool      `bson:"enabled"`
	CreatedAt time.Time `bson:"created_at"`
}

func TestMemoryStore_CRUD(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[memItem]()

	if err := s.Create(ctx, &memItem{ID: "1", Name: "a", Status: "idle"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.FindByID(ctx, "1")
	if err != nil || got.Name != "a" {
		t.Fatalf("find by id: %v %v", got, err)
	}

	got.Name = "b"
	if err := s.Update(ctx, "1", got); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := s.FindByID(ctx, "1")
	if reloaded.Name != "b" {
		t.Errorf("update not persisted: %v", reloaded)
	}

	if err := s.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindByID(ctx, "1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemoryStore_QueryAndCount(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[memItem]()
	_ = s.Create(ctx, &memItem{ID: "1", Status: "idle", Enabled: true})
	_ = s.Create(ctx, &memItem{ID: "2", Status: "busy", Enabled: true})
	_ = s.Create(ctx, &memItem{ID: "3", Status: "idle", Enabled: false})

	idle, err := s.Find(ctx, NewQuery().Eq("status", "idle"))
	if err != nil || len(idle) != 2 {
		t.Fatalf("expected 2 idle, got %d (%v)", len(idle), err)
	}
	n, _ := s.Count(ctx, NewQuery().Eq("enabled", true))
	if n != 2 {
		t.Errorf("expected 2 enabled, got %d", n)
	}
}

func TestMemoryStore_FindOneAndUpdate_Claims(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[memItem]()
	_ = s.Create(ctx, &memItem{ID: "1", Status: "idle", CreatedAt: time.Now().Add(-2 * time.Hour)})
	_ = s.Create(ctx, &memItem{ID: "2", Status: "idle", CreatedAt: time.Now().Add(-1 * time.Hour)})

	// Claim the oldest idle item atomically.
	claimed, err := s.FindOneAndUpdate(ctx,
		NewQuery().Eq("status", "idle").OrderBy("created_at", false),
		map[string]any{"status": "busy"})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != "1" || claimed.Status != "busy" {
		t.Fatalf("unexpected claim %+v", claimed)
	}

	// Only one idle remains.
	idle, _ := s.Find(ctx, NewQuery().Eq("status", "idle"))
	if len(idle) != 1 || idle[0].ID != "2" {
		t.Fatalf("expected only item 2 idle, got %+v", idle)
	}

	// No match returns ErrNotFound.
	if _, err := s.FindOneAndUpdate(ctx, NewQuery().Eq("status", "missing"), map[string]any{"status": "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
