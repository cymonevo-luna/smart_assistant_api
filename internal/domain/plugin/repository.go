package plugin

import (
	"context"

	"github.com/cymonevo/go_template/pkg/store"
)

// Repository is the persistence contract for plugins.
type Repository interface {
	store.Store[Plugin]
	FindBySlug(ctx context.Context, slug string) (*Plugin, error)
}

type repository struct {
	store.Store[Plugin]
}

// NewRepository wraps any store.Store[Plugin] and exposes it as a plugin Repository.
func NewRepository(s store.Store[Plugin]) Repository {
	return &repository{Store: s}
}

// FindBySlug returns a plugin by its unique slug.
func (r *repository) FindBySlug(ctx context.Context, slug string) (*Plugin, error) {
	return r.FindOne(ctx, store.NewQuery().Eq("slug", slug))
}
