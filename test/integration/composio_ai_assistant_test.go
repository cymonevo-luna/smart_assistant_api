//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
)

func setupComposioAIWithCredentials(t *testing.T) (*client, string) {
	t.Helper()
	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, composioAIPluginSlug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", map[string]any{
		"api_key": mockValidComposioAPIKey,
	})
	setup.requireStatus(t, http.StatusOK)

	sessionID := createAssistantSession(t, authed)
	return authed, sessionID
}

func TestComposioAIExecutesTaskSuccessfully(t *testing.T) {
	mockComposioAPI.ResetMCPScenario()
	mockComposioAPI.SetMCPScenario("success")

	authed, sessionID := setupComposioAIWithCredentials(t)

	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "create a github issue",
		"source": "button",
	})
	resp.requireStatus(t, http.StatusOK)

	var out assistant.ProcessMessageResponse
	resp.decode(t, &out)
	if out.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", out.Reply.Type, out.Reply.Text)
	}
	if out.Reply.Action == nil || out.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected success action, got %+v", out.Reply.Action)
	}
	if !strings.Contains(strings.ToLower(out.Reply.Text), "success") {
		t.Fatalf("expected success text, got %q", out.Reply.Text)
	}
	if out.Reply.Action.PluginSlug != composioAIPluginSlug {
		t.Fatalf("expected plugin slug %q, got %q", composioAIPluginSlug, out.Reply.Action.PluginSlug)
	}
}

func TestComposioAIFollowUpResumesSession(t *testing.T) {
	mockComposioAPI.ResetMCPScenario()
	mockComposioAPI.SetMCPScenario("needs_input_first")

	authed, sessionID := setupComposioAIWithCredentials(t)

	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "create a github issue",
		"source": "button",
	})
	first.requireStatus(t, http.StatusOK)

	var turn1 assistant.ProcessMessageResponse
	first.decode(t, &turn1)
	if turn1.Reply.Type != assistant.ReplyTypeFollowUp {
		t.Fatalf("expected follow_up, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}
	if turn1.Reply.Action == nil || turn1.Reply.Action.Status != assistant.ActionStatusPending {
		t.Fatalf("expected pending action, got %+v", turn1.Reply.Action)
	}
	if !strings.Contains(strings.ToLower(turn1.Reply.Text), "repository") {
		t.Fatalf("expected repository prompt, got %q", turn1.Reply.Text)
	}

	second := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "org/app",
		"source": "button",
	})
	second.requireStatus(t, http.StatusOK)

	var turn2 assistant.ProcessMessageResponse
	second.decode(t, &turn2)
	if turn2.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", turn2.Reply.Type, turn2.Reply.Text)
	}
	if turn2.Reply.Action == nil || turn2.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected success action, got %+v", turn2.Reply.Action)
	}
}

func TestComposioAIConfirmationFlow(t *testing.T) {
	mockComposioAPI.ResetMCPScenario()
	mockComposioAPI.SetMCPScenario("needs_confirmation_then_success")

	authed, sessionID := setupComposioAIWithCredentials(t)

	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "create a github issue",
		"source": "button",
	})
	first.requireStatus(t, http.StatusOK)

	var turn1 assistant.ProcessMessageResponse
	first.decode(t, &turn1)
	if turn1.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}

	second := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "yes",
		"source": "button",
	})
	second.requireStatus(t, http.StatusOK)

	var turn2 assistant.ProcessMessageResponse
	second.decode(t, &turn2)
	if turn2.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", turn2.Reply.Type, turn2.Reply.Text)
	}
	if turn2.Reply.Action == nil || turn2.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected success action, got %+v", turn2.Reply.Action)
	}
}

func TestComposioAISetupIncompleteBlocksExecution(t *testing.T) {
	mockComposioAPI.ResetMCPScenario()

	authed := registerAndLogin(t)
	_ = installPlugin(t, authed, composioAIPluginSlug)
	sessionID := createAssistantSession(t, authed)

	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "create a github issue",
		"source": "button",
	})
	resp.requireStatus(t, http.StatusOK)

	var out assistant.ProcessMessageResponse
	resp.decode(t, &out)
	if !strings.Contains(strings.ToLower(out.Reply.Text), "setup") {
		t.Fatalf("expected setup guidance, got %q", out.Reply.Text)
	}
	if out.Reply.Action == nil {
		t.Fatal("expected action payload")
	}
	if out.Reply.Action.Payload["reason"] != "setup_incomplete" {
		t.Fatalf("expected setup_incomplete reason, got %v", out.Reply.Action.Payload["reason"])
	}
}
