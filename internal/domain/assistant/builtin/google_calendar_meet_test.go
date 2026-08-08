package builtin

import (
	"strings"
	"testing"
	"time"
)

func TestMapGoogleCalendarMeetPayload(t *testing.T) {
	args := GoogleCalendarMeetArgs{
		AttendeeName:  "Janet",
		AttendeeEmail: "janet@gmail.com",
		StartTime:     "2026-08-09T14:00:00+07:00",
	}
	payload, err := MapGoogleCalendarMeetPayload(args, "UTC")
	if err != nil {
		t.Fatalf("MapGoogleCalendarMeetPayload: %v", err)
	}
	if payload.Summary != "Meeting with Janet" {
		t.Fatalf("summary = %q, want %q", payload.Summary, "Meeting with Janet")
	}
	if payload.StartDatetime != "2026-08-09T14:00:00+07:00" {
		t.Fatalf("start_datetime = %q", payload.StartDatetime)
	}
	start, err := time.Parse(time.RFC3339, payload.StartDatetime)
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	end, err := time.Parse(time.RFC3339, payload.EndDatetime)
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	if end.Sub(start) != time.Hour {
		t.Fatalf("expected 1h duration, got %v", end.Sub(start))
	}
	if len(payload.Attendees) != 1 || payload.Attendees[0] != "janet@gmail.com" {
		t.Fatalf("attendees = %#v", payload.Attendees)
	}
}

func TestMapGoogleCalendarMeetPayloadUsesCustomTitle(t *testing.T) {
	args := GoogleCalendarMeetArgs{
		AttendeeName:  "Janet",
		AttendeeEmail: "janet@gmail.com",
		StartTime:     "2026-08-09T14:00:00Z",
		Title:         "Quarterly sync",
	}
	payload, err := MapGoogleCalendarMeetPayload(args, "UTC")
	if err != nil {
		t.Fatalf("MapGoogleCalendarMeetPayload: %v", err)
	}
	if payload.Summary != "Quarterly sync" {
		t.Fatalf("summary = %q", payload.Summary)
	}
}

func TestMapGoogleCalendarMeetArgsMissingRequired(t *testing.T) {
	_, err := MapGoogleCalendarMeetArgs(map[string]any{
		"attendee_name": "Janet",
	}, "UTC")
	if err == nil || !strings.Contains(err.Error(), "attendee_email") {
		t.Fatalf("expected attendee_email error, got %v", err)
	}
}
