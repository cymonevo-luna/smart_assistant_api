package builtin

import (
	"fmt"
	"strings"
	"time"
)

// GoogleCalendarMeetSlug is the catalog slug for the reference Google Meet plugin.
const GoogleCalendarMeetSlug = "google-calendar-meet"

// AdapterGoogleCalendarMeet is the executor config key for the Google Calendar Meet adapter.
const AdapterGoogleCalendarMeet = "google_calendar_meet"

// RequiredComposioGoogleCalendarScopes documents the OAuth scopes a Composio
// connected Google account must grant for GOOGLECALENDAR_CREATE_EVENT:
//   - https://www.googleapis.com/auth/calendar
//   - https://www.googleapis.com/auth/calendar.events
const RequiredComposioGoogleCalendarScopes = "https://www.googleapis.com/auth/calendar, https://www.googleapis.com/auth/calendar.events"

const defaultMeetingDuration = time.Hour

// GoogleCalendarMeetArgs are the extracted plugin arguments before mapping.
type GoogleCalendarMeetArgs struct {
	AttendeeName  string
	AttendeeEmail string
	StartTime     string
	Title         string
}

// GoogleCalendarMeetPayload is the Composio GOOGLECALENDAR_CREATE_EVENT payload.
type GoogleCalendarMeetPayload struct {
	Summary       string   `json:"summary"`
	StartDatetime string   `json:"start_datetime"`
	EndDatetime   string   `json:"end_datetime"`
	Attendees     []string `json:"attendees"`
}

// ParseGoogleCalendarMeetArgs extracts typed arguments from the orchestrator map.
func ParseGoogleCalendarMeetArgs(args map[string]any) (GoogleCalendarMeetArgs, error) {
	out := GoogleCalendarMeetArgs{
		AttendeeName:  stringArg(args, "attendee_name"),
		AttendeeEmail: stringArg(args, "attendee_email"),
		StartTime:     stringArg(args, "start_time"),
		Title:         stringArg(args, "title"),
	}
	if out.AttendeeName == "" {
		return out, fmt.Errorf("attendee_name is required")
	}
	if out.AttendeeEmail == "" {
		return out, fmt.Errorf("attendee_email is required")
	}
	if out.StartTime == "" {
		return out, fmt.Errorf("start_time is required")
	}
	return out, nil
}

// MapGoogleCalendarMeetPayload converts extracted args into the Composio tool payload.
// Times are emitted in RFC3339; when timezone is empty, UTC is used.
func MapGoogleCalendarMeetPayload(args GoogleCalendarMeetArgs, timezone string) (GoogleCalendarMeetPayload, error) {
	loc := time.UTC
	if strings.TrimSpace(timezone) != "" {
		parsed, err := time.LoadLocation(timezone)
		if err != nil {
			return GoogleCalendarMeetPayload{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
		loc = parsed
	}

	start, err := parseStartTime(args.StartTime, loc)
	if err != nil {
		return GoogleCalendarMeetPayload{}, err
	}
	end := start.Add(defaultMeetingDuration)

	title := strings.TrimSpace(args.Title)
	if title == "" {
		title = fmt.Sprintf("Meeting with %s", args.AttendeeName)
	}

	return GoogleCalendarMeetPayload{
		Summary:       title,
		StartDatetime: start.Format(time.RFC3339),
		EndDatetime:   end.Format(time.RFC3339),
		Attendees:     []string{args.AttendeeEmail},
	}, nil
}

// MapGoogleCalendarMeetArgs is a convenience wrapper for executor integration.
func MapGoogleCalendarMeetArgs(args map[string]any, timezone string) (map[string]any, error) {
	parsed, err := ParseGoogleCalendarMeetArgs(args)
	if err != nil {
		return nil, err
	}
	payload, err := MapGoogleCalendarMeetPayload(parsed, timezone)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"summary":        payload.Summary,
		"start_datetime": payload.StartDatetime,
		"end_datetime":   payload.EndDatetime,
		"attendees":      payload.Attendees,
	}, nil
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	val, ok := args[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func parseStartTime(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("start_time is required")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse start_time %q", raw)
}
