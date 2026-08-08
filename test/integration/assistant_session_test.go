//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
)

func createAssistantSession(t *testing.T, c *client) string {
	t.Helper()
	resp := c.post("/api/v1/assistant/sessions", nil)
	resp.requireStatus(t, http.StatusCreated)
	var out assistant.CreateSessionResponse
	resp.decode(t, &out)
	if out.SessionID == "" {
		t.Fatal("expected session_id")
	}
	return out.SessionID
}

func TestAssistantNoPluginsReturnsAcknowledgment(t *testing.T) {
	authed := registerAndLogin(t)
	sessionID := createAssistantSession(t, authed)

	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Jarvis what's the weather",
		"source": "wake_word",
	})
	resp.requireStatus(t, http.StatusOK)

	var out assistant.ProcessMessageResponse
	resp.decode(t, &out)
	if out.Reply.Type != assistant.ReplyTypeText {
		t.Fatalf("expected reply type text, got %q", out.Reply.Type)
	}
	if !strings.Contains(out.Reply.Text, "no action is configured") {
		t.Fatalf("unexpected reply: %q", out.Reply.Text)
	}
	if out.Reply.Action != nil && out.Reply.Action.Status == assistant.ActionStatusSuccess {
		t.Fatal("did not expect successful action")
	}
}

func TestAssistantSessionIsolationBetweenUsers(t *testing.T) {
	userA := registerAndLogin(t)
	sessionID := createAssistantSession(t, userA)

	userB := registerAndLogin(t)
	resp := userB.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "hello",
		"source": "button",
	})
	resp.requireStatus(t, http.StatusForbidden)
}

func TestAssistantMessageHistoryRetrievable(t *testing.T) {
	authed := registerAndLogin(t)
	sessionID := createAssistantSession(t, authed)

	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "hello assistant",
		"source": "button",
	})
	first.requireStatus(t, http.StatusOK)

	second := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "another message",
		"source": "button",
	})
	second.requireStatus(t, http.StatusOK)

	history := authed.get("/api/v1/assistant/sessions/" + sessionID + "/messages")
	history.requireStatus(t, http.StatusOK)

	var out assistant.MessageHistoryResponse
	history.decode(t, &out)
	if len(out.Messages) != 4 {
		t.Fatalf("expected 4 messages (2 user + 2 assistant), got %d", len(out.Messages))
	}
	if out.Messages[0].Role != assistant.MessageRoleUser || out.Messages[0].Content != "hello assistant" {
		t.Fatalf("unexpected first message: %+v", out.Messages[0])
	}
	if out.Messages[2].Role != assistant.MessageRoleUser || out.Messages[2].Content != "another message" {
		t.Fatalf("unexpected third message: %+v", out.Messages[2])
	}
}
