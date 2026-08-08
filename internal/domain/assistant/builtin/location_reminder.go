package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	assistantsettings "github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/pkg/places"
	"github.com/cymonevo/go_template/pkg/response"
)

// LocationReminderSlug is the catalog slug for the location reminder plugin.
const LocationReminderSlug = "set-reminder"

// AdapterLocationReminder is the executor config key for the location reminder builtin adapter.
const AdapterLocationReminder = "location_reminder"

// Location mode values (mirrors reminder package constants).
const (
	LocationModeExact        = reminder.LocationModeExact
	LocationModePlaceKeyword = reminder.LocationModePlaceKeyword
)

// ExecuteLocationReminder creates a location-based reminder from collected plugin arguments.
func ExecuteLocationReminder(
	ctx context.Context,
	reminders *reminder.Service,
	settings *assistantsettings.Service,
	placesProvider places.Provider,
	userID string,
	args map[string]any,
) (map[string]any, error) {
	title := stringArg(args, "title")
	if title == "" {
		return nil, response.NewValidation(map[string]string{
			"title": "must not be empty",
		})
	}

	mode := stringArg(args, "location_mode")
	if mode != reminder.LocationModeExact && mode != reminder.LocationModePlaceKeyword {
		return nil, response.NewValidation(map[string]string{
			"location_mode": "must be exact or place_keyword",
		})
	}

	placeQuery := stringArg(args, "place_query")
	if placeQuery == "" {
		return nil, response.NewValidation(map[string]string{
			"place_query": "must not be empty",
		})
	}

	userSettings, err := settings.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	radiusMeters := userSettings.LocationReminderThresholdMeters

	var latitude, longitude *float64
	var placeKeyword *string
	placeQueryPtr := &placeQuery

	switch mode {
	case reminder.LocationModeExact:
		if IsVaguePlaceQuery(placeQuery) {
			return nil, response.NewValidation(map[string]string{
				"place_query": "please provide a full street address",
			})
		}
		result, err := placesProvider.Geocode(ctx, placeQuery)
		if err != nil {
			if places.Unprocessable(err) {
				return nil, response.NewValidation(map[string]string{
					"place_query": "I couldn't find that address. Could you provide a clearer address?",
				})
			}
			return nil, err
		}
		lat := result.Latitude
		lng := result.Longitude
		latitude = &lat
		longitude = &lng
		if result.FormattedAddress != "" {
			formatted := result.FormattedAddress
			placeQueryPtr = &formatted
		}
	case reminder.LocationModePlaceKeyword:
		normalized := NormalizePlaceKeyword(placeQuery)
		if normalized == "" {
			return nil, response.NewValidation(map[string]string{
				"place_query": "must not be empty",
			})
		}
		placeKeyword = &normalized
		placeQueryPtr = &normalized
	}

	locationMode := mode
	created, err := reminders.CreateLocation(ctx, userID, reminder.CreateInput{
		Title:        title,
		TriggerType:  reminder.TriggerTypeLocation,
		LocationMode: &locationMode,
		PlaceQuery:   placeQueryPtr,
		Latitude:     latitude,
		Longitude:    longitude,
		PlaceKeyword: placeKeyword,
		RadiusMeters: radiusMeters,
	})
	if err != nil {
		return nil, err
	}

	clientPayload := buildLocationReminderClientPayload(created)

	placeLabel := placeQuery
	if placeKeyword != nil {
		placeLabel = *placeKeyword
	}

	return map[string]any{
		"reply_text":     fmt.Sprintf("Reminder set: I'll remind you to %s when you're within %dm of %s.", title, radiusMeters, placeLabel),
		"client_payload": clientPayload,
	}, nil
}

func buildLocationReminderClientPayload(r *reminder.Reminder) map[string]any {
	payload := map[string]any{
		"reminder_id":   r.ID,
		"title":         r.Title,
		"trigger_type":  r.TriggerType,
		"radius_meters": r.RadiusMeters,
		"latitude":      nil,
		"longitude":     nil,
		"place_keyword": nil,
	}
	if r.LocationMode != nil {
		payload["location_mode"] = *r.LocationMode
	}
	if r.PlaceQuery != nil {
		payload["place_query"] = *r.PlaceQuery
	}
	if r.Latitude != nil {
		payload["latitude"] = *r.Latitude
	}
	if r.Longitude != nil {
		payload["longitude"] = *r.Longitude
	}
	if r.PlaceKeyword != nil {
		payload["place_keyword"] = *r.PlaceKeyword
	}
	return payload
}

// NormalizePlaceKeyword strips filler phrases from a place keyword query.
func NormalizePlaceKeyword(query string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	for {
		lower := strings.ToLower(normalized)
		trimmed := false
		for _, prefix := range []string{
			"any nearby ",
			"nearby ",
			"any ",
			"at any nearby ",
			"at nearby ",
		} {
			if strings.HasPrefix(lower, prefix) {
				normalized = strings.TrimSpace(normalized[len(prefix):])
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return normalized
}

// IsVaguePlaceQuery reports whether a place query is too generic to geocode.
func IsVaguePlaceQuery(query string) bool {
	lower := strings.TrimSpace(strings.ToLower(query))
	switch lower {
	case "home", "my home", "work", "my work", "office", "there", "here":
		return true
	}
	for _, suffix := range []string{" home", " work", " office"} {
		if strings.HasSuffix(lower, suffix) && len(lower) <= len(suffix)+10 {
			return true
		}
	}
	return false
}

// LocationReminderExecutorErrorText returns a user-facing message for location reminder failures.
func LocationReminderExecutorErrorText(err error) string {
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		if msg, ok := appErr.Fields["place_query"]; ok && msg != "" {
			return msg
		}
		if msg, ok := appErr.Fields["title"]; ok && msg != "" {
			return msg
		}
	}
	return ExecutorErrorText(err)
}
