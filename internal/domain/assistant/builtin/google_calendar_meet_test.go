package builtin

import (
	"strings"
	"testing"
	"time"
)

func TestMapGoogleCalendarMeet(t *testing.T) {
	payload, err := MapGoogleCalendarMeetArgs(map[string]any{
		"attendee_names":  "Kezia, Albert",
		"attendee_emails": []string{"kezia@example.com", "albert@example.com"},
		"start_time":      "2026-08-14T14:00:00+07:00",
	}, "UTC")
	if err != nil {
		t.Fatalf("MapGoogleCalendarMeetArgs: %v", err)
	}
	if payload["summary"] != "Meeting with Kezia, Albert" {
		t.Fatalf("summary = %v, want %q", payload["summary"], "Meeting with Kezia, Albert")
	}
	attendees, ok := payload["attendees"].([]string)
	if !ok {
		t.Fatalf("attendees type = %T, want []string", payload["attendees"])
	}
	if len(attendees) != 2 || attendees[0] != "kezia@example.com" || attendees[1] != "albert@example.com" {
		t.Fatalf("attendees = %#v", attendees)
	}
	if payload["start_datetime"] != "2026-08-14T14:00:00+07:00" {
		t.Fatalf("start_datetime = %v", payload["start_datetime"])
	}
	start, err := time.Parse(time.RFC3339, payload["start_datetime"].(string))
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	end, err := time.Parse(time.RFC3339, payload["end_datetime"].(string))
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	if end.Sub(start) != time.Hour {
		t.Fatalf("expected 1h duration, got %v", end.Sub(start))
	}
}

func TestMapGoogleCalendarMeetPayloadLegacySingleAttendee(t *testing.T) {
	payload, err := MapGoogleCalendarMeetArgs(map[string]any{
		"attendee_name":  "Janet",
		"attendee_email": "janet@gmail.com",
		"start_time":     "2026-08-09T14:00:00+07:00",
	}, "UTC")
	if err != nil {
		t.Fatalf("MapGoogleCalendarMeetArgs: %v", err)
	}
	if payload["summary"] != "Meeting with Janet" {
		t.Fatalf("summary = %v, want %q", payload["summary"], "Meeting with Janet")
	}
	attendees, ok := payload["attendees"].([]string)
	if !ok {
		t.Fatalf("attendees type = %T, want []string", payload["attendees"])
	}
	if len(attendees) != 1 || attendees[0] != "janet@gmail.com" {
		t.Fatalf("attendees = %#v", attendees)
	}
}

func TestParseGoogleCalendarMeetArgsMismatchedCounts(t *testing.T) {
	_, err := ParseGoogleCalendarMeetArgs(map[string]any{
		"attendee_names":  "Kezia, Albert",
		"attendee_emails": "kezia@example.com",
	})
	if err == nil {
		t.Fatal("expected error for mismatched attendee counts")
	}
	if !strings.Contains(err.Error(), "same number of entries") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMapGoogleCalendarMeetPayloadUsesCustomTitle(t *testing.T) {
	args := GoogleCalendarMeetArgs{
		AttendeeNames:  []string{"Janet"},
		AttendeeEmails: []string{"janet@gmail.com"},
		StartTime:      "2026-08-09T14:00:00Z",
		Title:          "Quarterly sync",
	}
	payload, err := MapGoogleCalendarMeetPayload(args, "UTC")
	if err != nil {
		t.Fatalf("MapGoogleCalendarMeetPayload: %v", err)
	}
	if payload.Summary != "Quarterly sync" {
		t.Fatalf("summary = %q", payload.Summary)
	}
}

func TestParseGoogleCalendarMeetArgsAllowsMissingStartTime(t *testing.T) {
	args, err := ParseGoogleCalendarMeetArgs(map[string]any{
		"attendee_names":  "Janet",
		"attendee_emails": "janet@gmail.com",
	})
	if err != nil {
		t.Fatalf("ParseGoogleCalendarMeetArgs: %v", err)
	}
	if args.StartTime != "" {
		t.Fatalf("start_time = %q, want empty", args.StartTime)
	}
}

func TestMapGoogleCalendarMeetArgsMissingRequired(t *testing.T) {
	_, err := MapGoogleCalendarMeetArgs(map[string]any{
		"attendee_names": "Janet",
	}, "UTC")
	if err == nil || !strings.Contains(err.Error(), "attendee_emails") {
		t.Fatalf("expected attendee_emails error, got %v", err)
	}
}
