package user

import (
	"context"

	"github.com/cymonevo/go_template/pkg/store"
)

// Repository is the persistence contract for users. It embeds the generic
// store.Store[User] for standard CRUD and adds domain specific queries.
//
// IMPORTANT: this interface — and the implementation below — are written ONCE
// and are completely database agnostic. To move from PostgreSQL to MongoDB you
// only change which store.Store[User] is injected in the wiring layer; nothing
// in this file changes.
type Repository interface {
	store.Store[User]
	FindByEmail(ctx context.Context, email string) (*User, error)
}

type repository struct {
	store.Store[User]
}

// NewRepository wraps any store.Store[User] (Postgres, Mongo, or a future
// backend) and exposes it as a user Repository.
func NewRepository(s store.Store[User]) Repository {
	return &repository{Store: s}
}

// FindByEmail demonstrates a domain query expressed purely through the abstract
// query builder, so it works identically across all backends.
func (r *repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.FindOne(ctx, store.NewQuery().Eq("email", email))
}
