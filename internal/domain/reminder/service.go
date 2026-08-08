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

// Create schedules a new pending time reminder. remindAt must not be in the past (UTC).
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
		TriggerType:  TriggerTypeTime,
		UserPluginID: userPluginID,
		Message:      message,
		RemindAt:     &remindAt,
		Status:       StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, reminder); err != nil {
		return nil, response.NewInternal("failed to create reminder").Wrap(err)
	}
	return reminder, nil
}

// CreateLocation persists a new pending location reminder for the given user.
func (s *Service) CreateLocation(ctx context.Context, userID string, in CreateInput) (*Reminder, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, response.NewValidation(map[string]string{
			"title": "must not be empty",
		})
	}

	triggerType := strings.TrimSpace(in.TriggerType)
	if triggerType != TriggerTypeLocation {
		return nil, response.NewValidation(map[string]string{
			"trigger_type": "must be location",
		})
	}

	if in.RadiusMeters < 1 {
		return nil, response.NewValidation(map[string]string{
			"radius_meters": "must be at least 1",
		})
	}

	if in.LocationMode == nil || strings.TrimSpace(*in.LocationMode) == "" {
		return nil, response.NewValidation(map[string]string{
			"location_mode": "is required when trigger_type is location",
		})
	}
	mode := strings.TrimSpace(*in.LocationMode)
	if mode != LocationModeExact && mode != LocationModePlaceKeyword {
		return nil, response.NewValidation(map[string]string{
			"location_mode": "must be exact or place_keyword",
		})
	}
	locationMode := &mode

	now := time.Now().UTC()
	reminder := &Reminder{
		ID:           uuid.NewString(),
		UserID:       userID,
		TriggerType:  TriggerTypeLocation,
		Title:        title,
		LocationMode: locationMode,
		PlaceQuery:   in.PlaceQuery,
		Latitude:     in.Latitude,
		Longitude:    in.Longitude,
		PlaceKeyword: in.PlaceKeyword,
		RadiusMeters: in.RadiusMeters,
		Status:       StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, reminder); err != nil {
		return nil, response.NewInternal("failed to create reminder").Wrap(err)
	}
	return reminder, nil
}

// List returns active (pending or notified) time reminders for a user, filtered by UTC date.
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

// ListByUser returns location reminders for a user filtered by status (defaults to pending).
func (s *Service) ListByUser(ctx context.Context, userID, status string) ([]Reminder, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = StatusPending
	}

	items, err := s.repo.FindByUserAndStatus(ctx, userID, status)
	if err != nil {
		return nil, response.NewInternal("failed to list reminders").Wrap(err)
	}
	if items == nil {
		return []Reminder{}, nil
	}
	return items, nil
}

// GetByID returns a location reminder owned by the caller.
func (s *Service) GetByID(ctx context.Context, userID, id string) (*Reminder, error) {
	reminder, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("reminder not found")
		}
		return nil, response.NewInternal("failed to load reminder").Wrap(err)
	}
	if reminder.UserID != userID {
		return nil, response.NewNotFound("reminder not found")
	}
	return reminder, nil
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

// DeleteByMessage removes a single pending time reminder matched by case-insensitive
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
			at := ""
			if matches[i].RemindAt != nil {
				at = matches[i].RemindAt.UTC().Format(time.RFC3339)
			}
			candidates = append(candidates, fmt.Sprintf("%q (%s)", matches[i].Message, at))
		}
		return response.NewConflict("ambiguous reminder match: " + strings.Join(candidates, "; "))
	}
}

// FindDue returns pending time reminders due on or before the given instant.
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

// MarkNotified transitions a time reminder to notified and records notified_at.
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

// MarkDelivered records client delivery ack for an owned notified time reminder.
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

// ListPendingDelivery returns notified time reminders awaiting client delivery ack.
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

// Cancel sets status to cancelled for a location reminder. Idempotent when already cancelled.
func (s *Service) Cancel(ctx context.Context, userID, id string) (*Reminder, error) {
	reminder, err := s.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if reminder.Status == StatusCancelled {
		return reminder, nil
	}

	now := time.Now().UTC()
	reminder.Status = StatusCancelled
	reminder.UpdatedAt = now

	if err := s.repo.Update(ctx, id, reminder); err != nil {
		return nil, response.NewInternal("failed to cancel reminder").Wrap(err)
	}
	return reminder, nil
}

// MarkTriggered sets status to triggered and records triggered_at for a location reminder.
// Idempotent when already triggered.
func (s *Service) MarkTriggered(ctx context.Context, userID, id string) (*Reminder, error) {
	reminder, err := s.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if reminder.Status == StatusTriggered {
		return reminder, nil
	}

	now := time.Now().UTC()
	reminder.Status = StatusTriggered
	reminder.TriggeredAt = &now
	reminder.UpdatedAt = now

	if err := s.repo.Update(ctx, id, reminder); err != nil {
		return nil, response.NewInternal("failed to mark reminder triggered").Wrap(err)
	}
	return reminder, nil
}

// CancelAllForUserPlugin cancels pending time reminders for a plugin install (e.g. on uninstall).
func (s *Service) CancelAllForUserPlugin(ctx context.Context, userID, userPluginID string) error {
	if err := s.repo.CancelPendingForUserPlugin(ctx, userID, userPluginID); err != nil {
		return response.NewInternal("failed to cancel reminders").Wrap(err)
	}
	return nil
}
