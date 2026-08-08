package reminder

import (
	"context"
	"strings"
	"time"

	"github.com/cymonevo/go_template/pkg/store"
)

// Repository is the persistence contract for reminders.
type Repository interface {
	store.Store[Reminder]
	FindDue(ctx context.Context, before time.Time) ([]Reminder, error)
	FindActiveByUser(ctx context.Context, userID string, filter ListFilter) ([]Reminder, error)
	FindPendingByUserAndMessage(ctx context.Context, userID, messageQuery string) ([]Reminder, error)
	FindPendingDeliveryByUser(ctx context.Context, userID string) ([]Reminder, error)
	CancelPendingForUserPlugin(ctx context.Context, userID, userPluginID string) error
	FindByUserAndStatus(ctx context.Context, userID, status string) ([]Reminder, error)
}

type repository struct {
	store.Store[Reminder]
}

// NewRepository wraps any store.Store[Reminder] as a reminder Repository.
func NewRepository(s store.Store[Reminder]) Repository {
	return &repository{Store: s}
}

// FindDue returns pending time reminders due on or before the given instant (UTC).
func (r *repository) FindDue(ctx context.Context, before time.Time) ([]Reminder, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("trigger_type", TriggerTypeTime).
		Eq("status", StatusPending).
		Lte("remind_at", before.UTC()).
		OrderBy("remind_at", false))
}

// FindActiveByUser returns pending and notified time reminders for a user, optionally
// scoped to today or tomorrow in UTC.
func (r *repository) FindActiveByUser(ctx context.Context, userID string, filter ListFilter) ([]Reminder, error) {
	q := store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		In("status", []string{StatusPending, StatusNotified}).
		OrderBy("remind_at", false)

	switch filter {
	case ListFilterToday:
		start, end := utcDayBounds(time.Now().UTC())
		q = q.Gte("remind_at", start).Lt("remind_at", end)
	case ListFilterTomorrow:
		start, end := utcDayBounds(time.Now().UTC().Add(24 * time.Hour))
		q = q.Gte("remind_at", start).Lt("remind_at", end)
	case ListFilterAll:
		// no date bounds
	default:
		// unknown filters behave like "all"
	}

	return r.Find(ctx, q)
}

// FindPendingByUserAndMessage returns pending time reminders whose message contains
// messageQuery (case-insensitive substring).
func (r *repository) FindPendingByUserAndMessage(ctx context.Context, userID, messageQuery string) ([]Reminder, error) {
	pattern := "%" + strings.TrimSpace(messageQuery) + "%"
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		Eq("status", StatusPending).
		Like("message", pattern).
		OrderBy("remind_at", false))
}

// FindPendingDeliveryByUser returns notified time reminders awaiting client delivery ack.
func (r *repository) FindPendingDeliveryByUser(ctx context.Context, userID string) ([]Reminder, error) {
	items, err := r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		Eq("status", StatusNotified).
		OrderBy("remind_at", false))
	if err != nil {
		return nil, err
	}

	out := make([]Reminder, 0, len(items))
	for i := range items {
		if items[i].DeliveredAt == nil {
			out = append(out, items[i])
		}
	}
	return out, nil
}

// CancelPendingForUserPlugin marks pending time reminders for a plugin install as cancelled.
func (r *repository) CancelPendingForUserPlugin(ctx context.Context, userID, userPluginID string) error {
	items, err := r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		Eq("user_plugin_id", userPluginID).
		Eq("status", StatusPending))
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for i := range items {
		items[i].Status = StatusCancelled
		items[i].UpdatedAt = now
		if err := r.Update(ctx, items[i].ID, &items[i]); err != nil {
			return err
		}
	}
	return nil
}

// FindByUserAndStatus returns location reminders for a user filtered by status.
func (r *repository) FindByUserAndStatus(ctx context.Context, userID, status string) ([]Reminder, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeLocation).
		Eq("status", status).
		OrderBy("created_at", true))
}

func utcDayBounds(day time.Time) (start, end time.Time) {
	y, m, d := day.UTC().Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	end = start.Add(24 * time.Hour)
	return start, end
}
