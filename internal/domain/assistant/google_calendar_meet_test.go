package assistant

import (
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

func TestNormalizeAttendeeNames(t *testing.T) {
	args := map[string]any{"attendee_names": "Kezia and Albert"}
	normalizeGoogleCalendarMeetArgs(args)
	names := stringSliceFromArg(args, "attendee_names")
	if len(names) != 2 || names[0] != "Kezia" || names[1] != "Albert" {
		t.Fatalf("names = %#v", names)
	}
	emails := stringSliceFromArg(args, "attendee_emails")
	if len(emails) != 0 {
		t.Fatalf("expected empty emails slice, got %#v", emails)
	}
}

func TestNormalizeAttendeeNamesArray(t *testing.T) {
	args := map[string]any{"attendee_names": []any{"Kezia", "Albert"}}
	normalizeGoogleCalendarMeetArgs(args)
	names := stringSliceFromArg(args, "attendee_names")
	if len(names) != 2 {
		t.Fatalf("names = %#v", names)
	}
}

func TestIsGoogleCalendarMeetPlugin(t *testing.T) {
	p := &plugin.Plugin{Slug: "google-calendar-meet"}
	if !isGoogleCalendarMeetPlugin(p) {
		t.Fatal("expected slug match")
	}
	p = &plugin.Plugin{
		Slug: "custom-meet",
		Manifest: plugin.PluginManifest{
			Executor: plugin.Executor{
				Config: map[string]any{"builtin_adapter": "google_calendar_meet"},
			},
		},
	}
	if !isGoogleCalendarMeetPlugin(p) {
		t.Fatal("expected adapter match")
	}
}
