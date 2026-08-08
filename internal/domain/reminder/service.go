package reminder

import (
	"context"
	"errors"
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

// Create persists a new pending reminder for the given user.
func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (*Reminder, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, response.NewValidation(map[string]string{
			"title": "must not be empty",
		})
	}

	triggerType := strings.TrimSpace(in.TriggerType)
	if triggerType != TriggerTypeTime && triggerType != TriggerTypeLocation {
		return nil, response.NewValidation(map[string]string{
			"trigger_type": "must be time or location",
		})
	}

	if in.RadiusMeters < 1 {
		return nil, response.NewValidation(map[string]string{
			"radius_meters": "must be at least 1",
		})
	}

	var locationMode *string
	if triggerType == TriggerTypeLocation {
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
		locationMode = &mode
	}

	now := time.Now().UTC()
	reminder := &Reminder{
		ID:           uuid.NewString(),
		UserID:       userID,
		Title:        title,
		TriggerType:  triggerType,
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

// ListByUser returns reminders for a user filtered by status (defaults to pending).
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

// GetByID returns a reminder owned by the caller.
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

// Cancel sets status to cancelled. Idempotent when already cancelled.
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

// MarkTriggered sets status to triggered and records triggered_at. Idempotent when already triggered.
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

// CancelAllForUserPlugin is a no-op retained for userplugin.ReminderCleaner compatibility.
// Reminders are not tied to plugin installs in the current schema.
func (s *Service) CancelAllForUserPlugin(_ context.Context, _, _ string) error {
	return nil
}
