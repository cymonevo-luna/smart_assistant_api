package reminder

import (
	"context"

	"github.com/cymonevo/go_template/pkg/store"
)

// Repository is the persistence contract for reminders.
type Repository interface {
	store.Store[Reminder]
	FindByUserAndStatus(ctx context.Context, userID, status string) ([]Reminder, error)
}

type repository struct {
	store.Store[Reminder]
}

// NewRepository wraps any store.Store[Reminder] as a reminder Repository.
func NewRepository(s store.Store[Reminder]) Repository {
	return &repository{Store: s}
}

// FindByUserAndStatus returns reminders for a user filtered by status.
func (r *repository) FindByUserAndStatus(ctx context.Context, userID, status string) ([]Reminder, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("status", status).
		OrderBy("created_at", true))
}
