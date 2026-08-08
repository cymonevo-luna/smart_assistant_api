//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cymonevo/go_template/internal/domain/assistant"
	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/internal/domain/user"
	"github.com/google/uuid"
)

func seedReminderPlugin(t *testing.T, slug string) {
	t.Helper()
	admin := newClient(t).authed(adminAccessToken(t))
	resp := admin.post("/api/admin/plugins", map[string]any{
		"slug":        slug,
		"name":        "Reminder",
		"description": "Set reminders",
		"version":     "1.0.0",
		"manifest": map[string]any{
			"triggers": []string{
				"remind me",
				"list reminders",
				"delete reminder",
			},
			"required_setup": false,
			"setup_type":     "none",
			"arguments": []any{
				map[string]any{
					"name":     "message",
					"type":     "string",
					"required": false,
					"prompt":   "What should I remind you about?",
				},
				map[string]any{
					"name":     "remind_at",
					"type":     "datetime",
					"required": false,
					"prompt":   "When should I remind you?",
				},
			},
			"confirmation_required": false,
			"executor": map[string]any{
				"type": "builtin",
				"config": map[string]any{
					"builtin_adapter": "reminder",
				},
			},
		},
	})
	resp.requireStatus(t, http.StatusCreated)
}

func registerLoginAndUserID(t *testing.T) (*client, string) {
	t.Helper()
	email := fmt.Sprintf("reminder-%s@integration.test", uuid.NewString())
	const password = "supersecret123"

	pub := newClient(t)
	reg := pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Reminder User",
		"password": password,
	})
	reg.requireStatus(t, http.StatusCreated)

	var created user.Response
	reg.decode(t, &created)

	login := pub.post("/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	login.requireStatus(t, http.StatusOK)

	var loginData struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	login.decode(t, &loginData)
	return pub.authed(loginData.Tokens.AccessToken), created.ID
}

func installReminderPlugin(t *testing.T, authed *client, slug string) {
	t.Helper()
	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": slug,
	})
	install.requireStatus(t, http.StatusCreated)
}

func seedReminderForUser(t *testing.T, userID, message string, remindAt time.Time) *reminder.Reminder {
	t.Helper()
	rem, err := application.Container().ReminderService.Create(context.Background(), userID, nil, message, remindAt)
	if err != nil {
		t.Fatalf("seed reminder: %v", err)
	}
	return rem
}

func TestAssistantCreateReminderWithConfirmation(t *testing.T) {
	slug := fmt.Sprintf("reminder-%s", uuid.NewString())
	seedReminderPlugin(t, slug)

	authed, userID := registerLoginAndUserID(t)
	installReminderPlugin(t, authed, slug)
	sessionID := createAssistantSession(t, authed)

	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Remind me to water plants at 3pm today",
		"source": "button",
	})
	first.requireStatus(t, http.StatusOK)

	var turn1 assistant.ProcessMessageResponse
	first.decode(t, &turn1)
	if turn1.Reply.Type != assistant.ReplyTypeConfirmation && turn1.Reply.Type != assistant.ReplyTypeFollowUp {
		t.Fatalf("expected confirmation or follow_up, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}
	if !strings.Contains(strings.ToLower(turn1.Reply.Text), "water plants") {
		t.Fatalf("expected water plants in reply, got %q", turn1.Reply.Text)
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
		t.Fatalf("expected successful action, got %+v", turn2.Reply.Action)
	}
	if !strings.Contains(turn2.Reply.Text, "water plants") {
		t.Fatalf("expected success reply to mention water plants, got %q", turn2.Reply.Text)
	}

	items, err := application.Container().ReminderService.List(context.Background(), userID, reminder.ListFilterToday)
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	found := false
	for _, item := range items {
		if strings.Contains(item.Message, "water plants") && item.Status == reminder.StatusPending {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pending reminder for water plants, got %+v", items)
	}
}

func TestAssistantListRemindersForToday(t *testing.T) {
	slug := fmt.Sprintf("reminder-%s", uuid.NewString())
	seedReminderPlugin(t, slug)

	authed, userID := registerLoginAndUserID(t)
	installReminderPlugin(t, authed, slug)
	seedReminderForUser(t, userID, "buy groceries", time.Now().UTC().Add(2*time.Hour))

	sessionID := createAssistantSession(t, authed)
	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "List all reminders for today",
		"source": "button",
	})
	resp.requireStatus(t, http.StatusOK)

	var turn assistant.ProcessMessageResponse
	resp.decode(t, &turn)
	if turn.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected immediate action_result, got %q (%s)", turn.Reply.Type, turn.Reply.Text)
	}
	if turn.Reply.Action == nil || turn.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful action, got %+v", turn.Reply.Action)
	}
	if !strings.Contains(turn.Reply.Text, "buy groceries") {
		t.Fatalf("expected reminder listed, got %q", turn.Reply.Text)
	}
}

func TestAssistantDeleteReminderByMessage(t *testing.T) {
	slug := fmt.Sprintf("reminder-%s", uuid.NewString())
	seedReminderPlugin(t, slug)

	authed, userID := registerLoginAndUserID(t)
	installReminderPlugin(t, authed, slug)
	seeded := seedReminderForUser(t, userID, "buy milk", time.Now().UTC().Add(2*time.Hour))

	sessionID := createAssistantSession(t, authed)
	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Delete my reminder for buy milk",
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
		t.Fatalf("expected successful action, got %+v", turn2.Reply.Action)
	}

	got, err := application.Container().ReminderService.FindOwnedByID(context.Background(), userID, seeded.ID)
	if err != nil {
		t.Fatalf("FindOwnedByID: %v", err)
	}
	if got.Status != reminder.StatusCancelled {
		t.Fatalf("expected status cancelled, got %q", got.Status)
	}
}

func TestAssistantCreateReminderRejectsPastTime(t *testing.T) {
	slug := fmt.Sprintf("reminder-%s", uuid.NewString())
	seedReminderPlugin(t, slug)

	authed, _ := registerLoginAndUserID(t)
	installReminderPlugin(t, authed, slug)
	sessionID := createAssistantSession(t, authed)

	steps := []string{
		"Remind me about past reminder test at 1pm today",
		"yes",
	}
	var last assistant.ProcessMessageResponse
	for _, text := range steps {
		resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
			"text":   text,
			"source": "button",
		})
		resp.requireStatus(t, http.StatusOK)
		resp.decode(t, &last)
	}

	if last.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q", last.Reply.Type)
	}
	if last.Reply.Action == nil || last.Reply.Action.Status != assistant.ActionStatusFailed {
		t.Fatalf("expected failed action, got %+v", last.Reply.Action)
	}
	if !strings.Contains(strings.ToLower(last.Reply.Text), "past") {
		t.Fatalf("expected past-time error text, got %q", last.Reply.Text)
	}
}
