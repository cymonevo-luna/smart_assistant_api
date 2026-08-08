package reminder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

// Service holds business logic for reminders.
type Service struct {
	repo Repository
}

// NewService constructs a reminder Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create schedules a new pending reminder. remindAt must not be in the past (UTC).
func (s *Service) Create(ctx context.Context, userID string, userPluginID *string, message string, remindAt time.Time) (*Reminder, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, response.NewValidation(map[string]string{
			"message": "must not be empty",
		})
	}

	remindAt = remindAt.UTC()
	if !remindAt.After(time.Now().UTC()) {
		return nil, response.NewValidation(map[string]string{
			"remind_at": "must be in the future",
		})
	}

	now := time.Now().UTC()
	reminder := &Reminder{
		ID:           uuid.NewString(),
		UserID:       userID,
		UserPluginID: userPluginID,
		Message:      message,
		RemindAt:     remindAt,
		Status:       StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, reminder); err != nil {
		return nil, response.NewInternal("failed to create reminder").Wrap(err)
	}
	return reminder, nil
}

// List returns active (pending or notified) reminders for a user, filtered by UTC date.
func (s *Service) List(ctx context.Context, userID string, filter ListFilter) ([]Reminder, error) {
	items, err := s.repo.FindActiveByUser(ctx, userID, filter)
	if err != nil {
		return nil, response.NewInternal("failed to list reminders").Wrap(err)
	}
	if items == nil {
		return []Reminder{}, nil
	}
	return items, nil
}

// FindOwnedByID returns a reminder when it belongs to the given user.
func (s *Service) FindOwnedByID(ctx context.Context, userID, id string) (*Reminder, error) {
	reminder, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("reminder not found")
		}
		return nil, response.NewInternal("failed to load reminder").Wrap(err)
	}
	if reminder.UserID != userID {
		return nil, response.NewForbidden("cannot access another user's reminder")
	}
	return reminder, nil
}

// DeleteByMessage removes a single pending reminder matched by case-insensitive
// substring on message.
func (s *Service) DeleteByMessage(ctx context.Context, userID, messageQuery string) error {
	messageQuery = strings.TrimSpace(messageQuery)
	if messageQuery == "" {
		return response.NewValidation(map[string]string{
			"message": "must not be empty",
		})
	}

	matches, err := s.repo.FindPendingByUserAndMessage(ctx, userID, messageQuery)
	if err != nil {
		return response.NewInternal("failed to find reminders").Wrap(err)
	}
	switch len(matches) {
	case 0:
		return response.NewNotFound("reminder not found")
	case 1:
		now := time.Now().UTC()
		matches[0].Status = StatusCancelled
		matches[0].UpdatedAt = now
		if err := s.repo.Update(ctx, matches[0].ID, &matches[0]); err != nil {
			return response.NewInternal("failed to delete reminder").Wrap(err)
		}
		return nil
	default:
		candidates := make([]string, 0, len(matches))
		for i := range matches {
			candidates = append(candidates, fmt.Sprintf("%q (%s)", matches[i].Message, matches[i].RemindAt.UTC().Format(time.RFC3339)))
		}
		return response.NewConflict("ambiguous reminder match: " + strings.Join(candidates, "; "))
	}
}

// FindDue returns pending reminders due on or before the given instant.
func (s *Service) FindDue(ctx context.Context, before time.Time) ([]Reminder, error) {
	items, err := s.repo.FindDue(ctx, before)
	if err != nil {
		return nil, response.NewInternal("failed to find due reminders").Wrap(err)
	}
	if items == nil {
		return []Reminder{}, nil
	}
	return items, nil
}

// MarkNotified transitions a reminder to notified and records notified_at.
func (s *Service) MarkNotified(ctx context.Context, id string) error {
	reminder, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return response.NewNotFound("reminder not found")
		}
		return response.NewInternal("failed to load reminder").Wrap(err)
	}
	if reminder.Status != StatusPending {
		return response.NewConflict("reminder is not pending")
	}

	now := time.Now().UTC()
	reminder.Status = StatusNotified
	reminder.NotifiedAt = &now
	reminder.UpdatedAt = now

	if err := s.repo.Update(ctx, id, reminder); err != nil {
		return response.NewInternal("failed to mark reminder notified").Wrap(err)
	}
	return nil
}

// MarkDelivered records client delivery ack for an owned notified reminder.
func (s *Service) MarkDelivered(ctx context.Context, userID, id string) error {
	reminder, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return response.NewNotFound("reminder not found")
		}
		return response.NewInternal("failed to load reminder").Wrap(err)
	}
	if reminder.UserID != userID {
		return response.NewForbidden("cannot modify another user's reminder")
	}
	if reminder.Status != StatusNotified {
		return response.NewConflict("reminder is not notified")
	}

	now := time.Now().UTC()
	reminder.DeliveredAt = &now
	reminder.UpdatedAt = now

	if err := s.repo.Update(ctx, id, reminder); err != nil {
		return response.NewInternal("failed to mark reminder delivered").Wrap(err)
	}
	return nil
}

// ListPendingDelivery returns notified reminders awaiting client delivery ack.
func (s *Service) ListPendingDelivery(ctx context.Context, userID string) ([]Reminder, error) {
	items, err := s.repo.FindPendingDeliveryByUser(ctx, userID)
	if err != nil {
		return nil, response.NewInternal("failed to list pending delivery reminders").Wrap(err)
	}
	if items == nil {
		return []Reminder{}, nil
	}
	return items, nil
}

// CancelAllForUserPlugin cancels pending reminders for a plugin install (e.g. on uninstall).
func (s *Service) CancelAllForUserPlugin(ctx context.Context, userID, userPluginID string) error {
	if err := s.repo.CancelPendingForUserPlugin(ctx, userID, userPluginID); err != nil {
		return response.NewInternal("failed to cancel reminders").Wrap(err)
	}
	return nil
}
