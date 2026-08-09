//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
)

func TestSemanticDelegationParaphrasedReminder(t *testing.T) {
	authed, _ := registerReminderJourneyUser(t)
	installReminderPluginFromCatalog(t, authed)
	sessionID := createAssistantSession(t, authed)

	turn := sendAssistantMessage(t, authed, sessionID, "don't let me forget to water plants at 6pm tomorrow")
	switch turn.Reply.Type {
	case assistant.ReplyTypeFollowUp, assistant.ReplyTypeActionResult, assistant.ReplyTypeConfirmation:
		// routed to reminder plugin
	default:
		t.Fatalf("expected follow_up, confirmation, or action_result, got %q (%s)", turn.Reply.Type, turn.Reply.Text)
	}
	if strings.Contains(strings.ToLower(turn.Reply.Text), "don't know how to do that yet") {
		t.Fatalf("expected reminder routing, got no-match text: %q", turn.Reply.Text)
	}
}

func TestSemanticDelegationUnrelatedQuery(t *testing.T) {
	authed, _ := registerReminderJourneyUser(t)
	installReminderPluginFromCatalog(t, authed)
	sessionID := createAssistantSession(t, authed)

	turn := sendAssistantMessage(t, authed, sessionID, "what's the weather in Jakarta")
	if turn.Reply.Type != assistant.ReplyTypeText {
		t.Fatalf("expected text reply, got %q", turn.Reply.Type)
	}
	if !strings.Contains(turn.Reply.Text, "don't know how to do that yet") {
		t.Fatalf("expected no-match text, got %q", turn.Reply.Text)
	}
}

func TestSemanticDelegationParaphrasedCalendar(t *testing.T) {
	mockComposioAPI.ResetRequests()
	mockComposioAPI.SetFail(false)
	mockComposioAPI.SetEmptyFreeSlots(false)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": googleCalendarMeetSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed struct {
		ID string `json:"id"`
	}
	install.decode(t, &installed)
	completeGoogleOAuthSetup(t, authed, installed.ID)

	sessionID := createAssistantSession(t, authed)
	turn := sendAssistantMessage(t, authed, sessionID, "book a call with Janet next Tuesday at 2")

	switch turn.Reply.Type {
	case assistant.ReplyTypeFollowUp, assistant.ReplyTypeConfirmation, assistant.ReplyTypeActionResult:
		// routed to calendar plugin
	default:
		t.Fatalf("expected plugin match (follow_up/confirmation/action_result), got %q (%s)", turn.Reply.Type, turn.Reply.Text)
	}
	if strings.Contains(strings.ToLower(turn.Reply.Text), "don't know how to do that yet") {
		t.Fatalf("expected calendar routing, got no-match text: %q", turn.Reply.Text)
	}
}

func TestSemanticDelegationPendingActionSkipsReclassification(t *testing.T) {
	authed, _ := registerReminderJourneyUser(t)
	installed := installReminderPluginFromCatalog(t, authed)
	sessionID := createAssistantSession(t, authed)

	seedReminderPendingRemindAt(t, sessionID, installed)

	second := sendAssistantMessage(t, authed, sessionID, "6pm tomorrow")
	switch second.Reply.Type {
	case assistant.ReplyTypeFollowUp, assistant.ReplyTypeConfirmation, assistant.ReplyTypeActionResult:
		// continued same plugin flow
	default:
		t.Fatalf("expected continued reminder flow, got %q (%s)", second.Reply.Type, second.Reply.Text)
	}
	if strings.Contains(strings.ToLower(second.Reply.Text), "don't know how to do that yet") {
		t.Fatalf("pending action should skip re-delegation, got no-match: %q", second.Reply.Text)
	}
}
