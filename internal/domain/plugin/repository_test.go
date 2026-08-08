package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

func TestRepositoryCRUD(t *testing.T) {
	repo := NewRepository(store.NewMemoryStore[Plugin]())
	ctx := context.Background()

	manifest := validManifest()
	now := time.Now().UTC()
	p := &Plugin{
		ID:          uuid.NewString(),
		Slug:        "google-calendar-meet",
		Name:        "Google Meet Scheduler",
		Description: "Schedule calendar meetings from voice commands",
		Version:     "1.0.0",
		Manifest:    manifest,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	byID, err := repo.FindByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if byID.Slug != p.Slug {
		t.Fatalf("expected slug %q, got %q", p.Slug, byID.Slug)
	}

	bySlug, err := repo.FindBySlug(ctx, p.Slug)
	if err != nil {
		t.Fatalf("find by slug: %v", err)
	}
	if bySlug.ID != p.ID {
		t.Fatalf("expected id %q, got %q", p.ID, bySlug.ID)
	}

	p.Name = "Updated Scheduler"
	p.UpdatedAt = time.Now().UTC()
	if err := repo.Update(ctx, p.ID, p); err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, err := repo.FindBySlug(ctx, p.Slug)
	if err != nil {
		t.Fatalf("find updated: %v", err)
	}
	if updated.Name != "Updated Scheduler" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}

	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.FindBySlug(ctx, p.Slug); err == nil {
		t.Fatal("expected not found after delete")
	} else if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
