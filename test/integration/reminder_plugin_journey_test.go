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
	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/google/uuid"
)

const reminderPluginSlug = "reminder"

func registerReminderJourneyUser(t *testing.T) (*client, string) {
	t.Helper()
	email := fmt.Sprintf("reminder-journey-%s@integration.test", uuid.NewString())
	const password = "supersecret123"

	pub := newClient(t)
	reg := pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Reminder Journey User",
		"password": password,
	})
	reg.requireStatus(t, http.StatusCreated)

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
	token := loginData.Tokens.AccessToken
	if token == "" {
		t.Fatal("expected an access token after login")
	}

	claims, err := application.Container().Tokens.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	return pub.authed(token), claims.UserID
}

func installReminderPluginFromCatalog(t *testing.T, authed *client) userplugin.InstalledResponse {
	t.Helper()
	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": reminderPluginSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	if installed.Plugin.Slug != reminderPluginSlug {
		t.Fatalf("expected slug %q, got %q", reminderPluginSlug, installed.Plugin.Slug)
	}
	return installed
}

func sendAssistantMessage(t *testing.T, authed *client, sessionID, text string) assistant.ProcessMessageResponse {
	t.Helper()
	resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   text,
		"source": "button",
	})
	resp.requireStatus(t, http.StatusOK)

	var turn assistant.ProcessMessageResponse
	resp.decode(t, &turn)
	return turn
}

func assistantConfirmYes(t *testing.T, authed *client, sessionID string) assistant.ProcessMessageResponse {
	t.Helper()
	turn := sendAssistantMessage(t, authed, sessionID, "yes")
	if turn.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result after confirmation, got %q (%s)", turn.Reply.Type, turn.Reply.Text)
	}
	return turn
}

func findPendingReminderByMessage(t *testing.T, userID, message string) *reminder.Reminder {
	t.Helper()
	items, err := application.Container().ReminderService.List(context.Background(), userID, reminder.ListFilterAll)
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	for i := range items {
		if strings.Contains(items[i].Message, message) && items[i].Status == reminder.StatusPending {
			return &items[i]
		}
	}
	t.Fatalf("expected pending reminder containing %q, got %+v", message, items)
	return nil
}

// setReminderRemindAtInPast updates remind_at directly in the repository so dispatch
// can be triggered without waiting for real time to pass.
func setReminderRemindAtInPast(t *testing.T, reminderID string, past time.Time) {
	t.Helper()
	ctx := context.Background()
	rem, err := application.Container().ReminderRepo.FindByID(ctx, reminderID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	at := past.UTC()
	rem.RemindAt = &at
	rem.UpdatedAt = time.Now().UTC()
	if err := application.Container().ReminderRepo.Update(ctx, reminderID, rem); err != nil {
		t.Fatalf("update remind_at: %v", err)
	}
}

func TestReminderPluginInCatalog(t *testing.T) {
	authed := registerAndLogin(t)

	detail := authed.get("/api/v1/plugins/" + reminderPluginSlug)
	detail.requireStatus(t, http.StatusOK)

	var pluginDetail plugin.DetailResponse
	detail.decode(t, &pluginDetail)
	if pluginDetail.Slug != reminderPluginSlug {
		t.Fatalf("expected slug %q, got %q", reminderPluginSlug, pluginDetail.Slug)
	}
	if pluginDetail.Name != "Reminder" {
		t.Fatalf("expected name %q, got %q", "Reminder", pluginDetail.Name)
	}
}

func TestInstallReminderPluginWithoutSetup(t *testing.T) {
	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": reminderPluginSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	if installed.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected setup_status completed, got %q", installed.SetupStatus)
	}
	if !installed.Enabled {
		t.Fatal("expected enabled true")
	}
	if installed.Plugin.Slug != reminderPluginSlug {
		t.Fatalf("expected slug reminder, got %q", installed.Plugin.Slug)
	}
}

func TestUninstallReminderPlugin(t *testing.T) {
	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": reminderPluginSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)

	del := authed.delete("/api/v1/users/me/plugins/" + installed.ID)
	del.requireStatus(t, http.StatusNoContent)

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	var items []userplugin.InstalledResponse
	list.decode(t, &items)
	for _, item := range items {
		if item.Plugin.Slug == reminderPluginSlug {
			t.Fatalf("expected reminder plugin omitted after uninstall, got %+v", items)
		}
	}
}

func TestReminderPluginJourney(t *testing.T) {
	authed, userID := registerReminderJourneyUser(t)
	installed := installReminderPluginFromCatalog(t, authed)
	sessionID := createAssistantSession(t, authed)

	const notifyMessage = "integration journey notify"
	createTurn := sendAssistantMessage(t, authed, sessionID, "Remind me to "+notifyMessage+" at 4pm today")
	if createTurn.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation, got %q (%s)", createTurn.Reply.Type, createTurn.Reply.Text)
	}
	if !strings.Contains(strings.ToLower(createTurn.Reply.Text), notifyMessage) {
		t.Fatalf("expected confirmation to mention %q, got %q", notifyMessage, createTurn.Reply.Text)
	}

	createResult := assistantConfirmYes(t, authed, sessionID)
	if createResult.Reply.Action == nil || createResult.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful create, got %+v", createResult.Reply.Action)
	}

	notifyReminder := findPendingReminderByMessage(t, userID, notifyMessage)

	listTurn := sendAssistantMessage(t, authed, sessionID, "List all reminders for today")
	if listTurn.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result for list, got %q (%s)", listTurn.Reply.Type, listTurn.Reply.Text)
	}
	if listTurn.Reply.Action == nil || listTurn.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful list, got %+v", listTurn.Reply.Action)
	}
	if !strings.Contains(listTurn.Reply.Text, notifyMessage) {
		t.Fatalf("expected list reply to include %q, got %q", notifyMessage, listTurn.Reply.Text)
	}

	setReminderRemindAtInPast(t, notifyReminder.ID, time.Now().UTC().Add(-time.Minute))
	runReminderDispatch(t)

	ctx := context.Background()
	dispatched, err := application.Container().ReminderRepo.FindByID(ctx, notifyReminder.ID)
	if err != nil {
		t.Fatalf("FindByID after dispatch: %v", err)
	}
	if dispatched.Status != reminder.StatusNotified {
		t.Fatalf("expected status notified after dispatch, got %q", dispatched.Status)
	}
	if dispatched.NotifiedAt == nil {
		t.Fatal("expected notified_at to be set after dispatch")
	}

	pending := authed.get("/api/v1/users/me/reminders/notifications/pending")
	pending.requireStatus(t, http.StatusOK)
	var pendingItems []reminder.Response
	pending.decode(t, &pendingItems)
	if len(pendingItems) != 1 || pendingItems[0].ID != notifyReminder.ID {
		t.Fatalf("expected one pending notification for %q, got %+v", notifyReminder.ID, pendingItems)
	}

	delivered := authed.post(fmt.Sprintf("/api/v1/users/me/reminders/%s/delivered", notifyReminder.ID), nil)
	delivered.requireStatus(t, http.StatusNoContent)

	const deleteMessage = "integration journey delete"
	deleteCreate := sendAssistantMessage(t, authed, sessionID, "Remind me to "+deleteMessage+" at 5pm today")
	if deleteCreate.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation for delete target create, got %q", deleteCreate.Reply.Type)
	}
	deleteCreateResult := assistantConfirmYes(t, authed, sessionID)
	if deleteCreateResult.Reply.Action == nil || deleteCreateResult.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful create for delete target, got %+v", deleteCreateResult.Reply.Action)
	}
	deleteTarget := findPendingReminderByMessage(t, userID, deleteMessage)

	deleteTurn := sendAssistantMessage(t, authed, sessionID, "Delete my reminder for "+deleteMessage)
	if deleteTurn.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation for delete, got %q (%s)", deleteTurn.Reply.Type, deleteTurn.Reply.Text)
	}
	deleteResult := assistantConfirmYes(t, authed, sessionID)
	if deleteResult.Reply.Action == nil || deleteResult.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful delete, got %+v", deleteResult.Reply.Action)
	}

	cancelled, err := application.Container().ReminderService.FindOwnedByID(ctx, userID, deleteTarget.ID)
	if err != nil {
		t.Fatalf("FindOwnedByID after delete: %v", err)
	}
	if cancelled.Status != reminder.StatusCancelled {
		t.Fatalf("expected status cancelled after assistant delete, got %q", cancelled.Status)
	}

	const orphanMessage = "integration journey orphan"
	orphanCreate := sendAssistantMessage(t, authed, sessionID, "Remind me to "+orphanMessage+" at 6pm today")
	if orphanCreate.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation for orphan create, got %q", orphanCreate.Reply.Type)
	}
	orphanCreateResult := assistantConfirmYes(t, authed, sessionID)
	if orphanCreateResult.Reply.Action == nil || orphanCreateResult.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful orphan create, got %+v", orphanCreateResult.Reply.Action)
	}
	orphanReminder := findPendingReminderByMessage(t, userID, orphanMessage)

	uninstall := authed.delete("/api/v1/users/me/plugins/" + installed.ID)
	uninstall.requireStatus(t, http.StatusNoContent)

	orphanAfter, err := application.Container().ReminderService.FindOwnedByID(ctx, userID, orphanReminder.ID)
	if err != nil {
		t.Fatalf("FindOwnedByID after uninstall: %v", err)
	}
	if orphanAfter.Status != reminder.StatusCancelled {
		t.Fatalf("expected orphan reminder cancelled on uninstall, got %q", orphanAfter.Status)
	}
}

func TestReminderPluginDeleteNotFound(t *testing.T) {
	authed, _ := registerReminderJourneyUser(t)
	installReminderPluginFromCatalog(t, authed)
	sessionID := createAssistantSession(t, authed)

	deleteTurn := sendAssistantMessage(t, authed, sessionID, "Delete my reminder for does-not-exist-message")
	if deleteTurn.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation before delete attempt, got %q (%s)", deleteTurn.Reply.Type, deleteTurn.Reply.Text)
	}

	result := assistantConfirmYes(t, authed, sessionID)
	if result.Reply.Action == nil || result.Reply.Action.Status != assistant.ActionStatusFailed {
		t.Fatalf("expected failed delete action, got %+v", result.Reply.Action)
	}
	if !strings.Contains(strings.ToLower(result.Reply.Text), "couldn't find") {
		t.Fatalf("expected not-found reply text, got %q", result.Reply.Text)
	}
}
