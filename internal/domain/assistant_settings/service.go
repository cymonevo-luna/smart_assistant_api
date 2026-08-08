package assistantsettings

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

// Service holds business logic for assistant settings.
type Service struct {
	repo Repository
}

// NewService constructs an assistant settings Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetOrCreate returns the caller's settings, creating defaults on first access.
func (s *Service) GetOrCreate(ctx context.Context, userID string) (*Settings, error) {
	settings, err := s.repo.FindByID(ctx, userID)
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, response.NewInternal("failed to load assistant settings").Wrap(err)
	}

	now := time.Now().UTC()
	defaults := &Settings{
		UserID:                          userID,
		WakeWord:                        DefaultWakeWord,
		ActiveListeningEnabled:          false,
		LocationReminderThresholdMeters: DefaultLocationReminderThresholdMeters,
		UpdatedAt:                       now,
	}
	if err := s.repo.Create(ctx, defaults); err != nil {
		// Another request may have created the row concurrently.
		settings, findErr := s.repo.FindByID(ctx, userID)
		if findErr == nil {
			return settings, nil
		}
		return nil, response.NewInternal("failed to create assistant settings").Wrap(err)
	}
	return defaults, nil
}

// Update modifies the caller's assistant settings.
func (s *Service) Update(ctx context.Context, userID string, in UpdateInput) (*Settings, error) {
	wakeWord := strings.TrimSpace(in.WakeWord)
	if wakeWord == "" {
		return nil, response.NewValidation(map[string]string{
			"wake_word": "must not be empty",
		})
	}
	if utf8.RuneCountInString(wakeWord) > 32 {
		return nil, response.NewValidation(map[string]string{
			"wake_word": "must be at most 32 characters",
		})
	}

	settings, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}

	if in.LocationReminderThresholdMeters != nil {
		if err := validateLocationReminderThreshold(*in.LocationReminderThresholdMeters); err != nil {
			return nil, err
		}
		settings.LocationReminderThresholdMeters = *in.LocationReminderThresholdMeters
	}

	settings.WakeWord = wakeWord
	settings.ActiveListeningEnabled = in.ActiveListeningEnabled
	settings.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, userID, settings); err != nil {
		return nil, response.NewInternal("failed to update assistant settings").Wrap(err)
	}
	return settings, nil
}

func validateLocationReminderThreshold(meters int) error {
	if meters < 10 || meters > 5000 {
		return response.NewValidation(map[string]string{
			"location_reminder_threshold_meters": "must be between 10 and 5000 meters",
		})
	}
	return nil
}
