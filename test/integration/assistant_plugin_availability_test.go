//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
)

func TestAssistantMeetPluginWithoutOAuthReturnsSetupGuidance(t *testing.T) {
	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": googleCalendarMeetSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	if installed.SetupStatus == userplugin.SetupStatusCompleted {
		t.Fatal("expected setup to be incomplete before OAuth")
	}

	sessionID := createAssistantSession(t, authed)

	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Schedule a meeting with Janet at 2 PM tomorrow",
		"source": "button",
	})
	resp.requireStatus(t, http.StatusOK)

	var out assistant.ProcessMessageResponse
	resp.decode(t, &out)
	if out.Reply.Type != assistant.ReplyTypeText {
		t.Fatalf("expected text reply, got %q", out.Reply.Type)
	}
	if !strings.Contains(strings.ToLower(out.Reply.Text), "setup") {
		t.Fatalf("expected setup guidance, got %q", out.Reply.Text)
	}
	if strings.Contains(strings.ToLower(out.Reply.Text), "no action is configured") {
		t.Fatalf("did not expect no-action text, got %q", out.Reply.Text)
	}
	if out.Reply.Action == nil {
		t.Fatal("expected action in reply")
	}
	if out.Reply.Action.Status != assistant.ActionStatusPending {
		t.Fatalf("expected pending action status, got %q", out.Reply.Action.Status)
	}
	if out.Reply.Action.Payload["reason"] != "setup_incomplete" {
		t.Fatalf("expected setup_incomplete reason, got %v", out.Reply.Action.Payload["reason"])
	}
	if out.Reply.Action.Payload["install_id"] != installed.ID {
		t.Fatalf("expected install_id %q, got %v", installed.ID, out.Reply.Action.Payload["install_id"])
	}
	if out.Reply.Action.Payload["plugin_slug"] != googleCalendarMeetSlug {
		t.Fatalf("expected plugin_slug %q, got %v", googleCalendarMeetSlug, out.Reply.Action.Payload["plugin_slug"])
	}
}

func TestAssistantDisabledReminderPluginReturnsEnableGuidance(t *testing.T) {
	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": reminderPluginSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)

	patch := authed.patch("/api/v1/users/me/plugins/"+installed.ID, map[string]any{
		"enabled": false,
	})
	patch.requireStatus(t, http.StatusOK)

	sessionID := createAssistantSession(t, authed)

	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Remind me to call mom at 3pm tomorrow",
		"source": "button",
	})
	resp.requireStatus(t, http.StatusOK)

	var out assistant.ProcessMessageResponse
	resp.decode(t, &out)
	if out.Reply.Type != assistant.ReplyTypeText {
		t.Fatalf("expected text reply, got %q", out.Reply.Type)
	}
	if !strings.Contains(strings.ToLower(out.Reply.Text), "disabled") {
		t.Fatalf("expected disabled guidance, got %q", out.Reply.Text)
	}
	if out.Reply.Action == nil {
		t.Fatal("expected action in reply")
	}
	if out.Reply.Action.Status != assistant.ActionStatusPending {
		t.Fatalf("expected pending action status, got %q", out.Reply.Action.Status)
	}
	if out.Reply.Action.Payload["reason"] != "plugin_disabled" {
		t.Fatalf("expected plugin_disabled reason, got %v", out.Reply.Action.Payload["reason"])
	}
	if out.Reply.Action.Payload["install_id"] != installed.ID {
		t.Fatalf("expected install_id %q, got %v", installed.ID, out.Reply.Action.Payload["install_id"])
	}
	if out.Reply.Action.Payload["plugin_slug"] != reminderPluginSlug {
		t.Fatalf("expected plugin_slug %q, got %v", reminderPluginSlug, out.Reply.Action.Payload["plugin_slug"])
	}
}
