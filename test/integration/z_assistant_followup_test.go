//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
	"github.com/google/uuid"
)

func seedCalendarPlugin(t *testing.T, slug string) {
	t.Helper()
	admin := newClient(t).authed(adminAccessToken(t))
	resp := admin.post("/api/admin/plugins", map[string]any{
		"slug":        slug,
		"name":        "Google Calendar Meet",
		"description": "Schedule meetings",
		"version":     "1.0.0",
		"manifest": map[string]any{
			"triggers":       []string{"schedule meeting"},
			"required_setup": false,
			"setup_type":     "none",
			"arguments": []any{
				map[string]any{
					"name":        "attendee_email",
					"type":        "email",
					"required":    true,
					"description": "Attendee email",
					"prompt":      "What is Janet's email address?",
				},
			},
			"confirmation_required": false,
			"executor": map[string]any{
				"type": "composio",
				"config": map[string]any{
					"tool_slug": "GOOGLECALENDAR_CREATE_EVENT",
				},
			},
		},
	})
	resp.requireStatus(t, http.StatusCreated)
}

// Runs in z_* file so plugin catalog tests that expect an empty database execute first.
func TestAssistantFollowUpForMissingArgument(t *testing.T) {
	slug := fmt.Sprintf("google-calendar-meet-%s", uuid.NewString())
	seedCalendarPlugin(t, slug)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": slug,
	})
	install.requireStatus(t, http.StatusCreated)

	sessionID := createAssistantSession(t, authed)

	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Schedule meeting with Janet at 2pm tomorrow",
		"source": "button",
	})
	first.requireStatus(t, http.StatusOK)

	var turn1 assistant.ProcessMessageResponse
	first.decode(t, &turn1)
	if turn1.Reply.Type != assistant.ReplyTypeFollowUp {
		t.Fatalf("expected follow_up, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}
	if !strings.Contains(turn1.Reply.Text, "email") {
		t.Fatalf("expected email follow-up, got %q", turn1.Reply.Text)
	}

	second := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "janet@example.com",
		"source": "button",
	})
	second.requireStatus(t, http.StatusOK)

	var turn2 assistant.ProcessMessageResponse
	second.decode(t, &turn2)
	if turn2.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result after follow-up, got %q (%s)", turn2.Reply.Type, turn2.Reply.Text)
	}
	if turn2.Reply.Action == nil || turn2.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful action, got %+v", turn2.Reply.Action)
	}
}
