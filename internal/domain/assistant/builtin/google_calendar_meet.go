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
	AttendeeNames  []string
	AttendeeEmails []string
	StartTime      string
	Title          string
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
		StartTime: stringArg(args, "start_time"),
		Title:     stringArg(args, "title"),
	}

	names, err := stringSliceArg(args, "attendee_names")
	if err != nil {
		return out, err
	}
	if len(names) == 0 {
		if legacy := stringArg(args, "attendee_name"); legacy != "" {
			names = []string{legacy}
		}
	}
	out.AttendeeNames = names

	emails, err := stringSliceArg(args, "attendee_emails")
	if err != nil {
		return out, err
	}
	if len(emails) == 0 {
		if legacy := stringArg(args, "attendee_email"); legacy != "" {
			emails = []string{legacy}
		}
	}
	out.AttendeeEmails = emails

	if len(out.AttendeeNames) == 0 {
		return out, fmt.Errorf("attendee_names is required")
	}
	if len(out.AttendeeEmails) == 0 {
		return out, fmt.Errorf("attendee_emails is required")
	}
	if len(out.AttendeeNames) != len(out.AttendeeEmails) {
		return out, fmt.Errorf("attendee_names and attendee_emails must have the same number of entries")
	}
	return out, nil
}

// MapGoogleCalendarMeetPayload converts extracted args into the Composio tool payload.
// Times are emitted in RFC3339; when timezone is empty, UTC is used.
func MapGoogleCalendarMeetPayload(args GoogleCalendarMeetArgs, timezone string) (GoogleCalendarMeetPayload, error) {
	if strings.TrimSpace(args.StartTime) == "" {
		return GoogleCalendarMeetPayload{}, fmt.Errorf("start_time is required")
	}

	loc := time.UTC
	if strings.TrimSpace(timezone) != "" {
		parsed, err := time.LoadLocation(timezone)
		if err != nil {
			return GoogleCalendarMeetPayload{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
		loc = parsed
	}

	start, err := ParseDateTime(args.StartTime, loc)
	if err != nil {
		return GoogleCalendarMeetPayload{}, err
	}
	end := start.Add(defaultMeetingDuration)

	title := strings.TrimSpace(args.Title)
	if title == "" {
		title = fmt.Sprintf("Meeting with %s", strings.Join(args.AttendeeNames, ", "))
	}

	return GoogleCalendarMeetPayload{
		Summary:       title,
		StartDatetime: start.Format(time.RFC3339),
		EndDatetime:   end.Format(time.RFC3339),
		Attendees:     args.AttendeeEmails,
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

func stringSliceArg(args map[string]any, key string) ([]string, error) {
	if args == nil {
		return nil, nil
	}
	val, ok := args[key]
	if !ok || val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case string:
		return splitCommaSeparated(v), nil
	case []string:
		return trimStrings(v), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return splitCommaSeparated(fmt.Sprint(v)), nil
	}
}

func splitCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
