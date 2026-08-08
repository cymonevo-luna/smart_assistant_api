package assistantsettings

import "time"

// UpdateInput is the validated payload for updating assistant settings.
type UpdateInput struct {
	WakeWord                        string `json:"wake_word" validate:"max=32"`
	ActiveListeningEnabled          bool   `json:"active_listening_enabled"`
	LocationReminderThresholdMeters *int   `json:"location_reminder_threshold_meters,omitempty"`
}

// Response is the public representation of assistant settings.
type Response struct {
	WakeWord                        string    `json:"wake_word"`
	ActiveListeningEnabled          bool      `json:"active_listening_enabled"`
	LocationReminderThresholdMeters int       `json:"location_reminder_threshold_meters"`
	UpdatedAt                       time.Time `json:"updated_at"`
}

// ToResponse maps a domain entity to its public representation.
func ToResponse(s *Settings) Response {
	return Response{
		WakeWord:                        s.WakeWord,
		ActiveListeningEnabled:          s.ActiveListeningEnabled,
		LocationReminderThresholdMeters: s.LocationReminderThresholdMeters,
		UpdatedAt:                       s.UpdatedAt,
	}
}
