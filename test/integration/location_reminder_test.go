//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/reminder"
)

const locationReminderSlug = "set-reminder"

func TestLocationReminderPluginVisibleInCatalog(t *testing.T) {
	authed := registerReminderUser(t)
	list := authed.client.get("/api/v1/plugins")
	list.requireStatus(t, http.StatusOK)

	var plugins []plugin.CatalogSummary
	list.decode(t, &plugins)

	var found *plugin.CatalogSummary
	for i := range plugins {
		if plugins[i].Slug == locationReminderSlug {
			found = &plugins[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected %q plugin in catalog, got slugs: %+v", locationReminderSlug, plugins)
	}

	detail := authed.client.get("/api/v1/plugins/" + locationReminderSlug)
	detail.requireStatus(t, http.StatusOK)

	var p plugin.Plugin
	detail.decode(t, &p)

	triggers := strings.Join(p.Manifest.Triggers, " ")
	if !strings.Contains(triggers, "remind me once I") {
		t.Fatalf("expected location triggers in manifest, got %+v", p.Manifest.Triggers)
	}
	adapter, _ := p.Manifest.Executor.Config["builtin_adapter"].(string)
	if adapter != "location_reminder" {
		t.Fatalf("expected location_reminder adapter, got %q", adapter)
	}
}

func installLocationReminderPlugin(t *testing.T, authed *client) {
	t.Helper()
	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": locationReminderSlug,
	})
	install.requireStatus(t, http.StatusCreated)
}

func postAssistantMessage(t *testing.T, authed *client, sessionID, text string) assistant.ProcessMessageResponse {
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

func listLocationReminders(t *testing.T, authed *client) []reminder.Response {
	t.Helper()
	resp := authed.get("/api/v1/reminders")
	resp.requireStatus(t, http.StatusOK)

	var items []reminder.Response
	resp.decode(t, &items)
	return items
}

func TestLocationReminderExactLocationFullFlow(t *testing.T) {
	authedUser := registerReminderUser(t)
	installLocationReminderPlugin(t, authedUser.client)
	sessionID := createAssistantSession(t, authedUser.client)

	turn1 := postAssistantMessage(t, authedUser.client, sessionID, "Remind me to pick my printer once I got home")
	if turn1.Reply.Type != assistant.ReplyTypeFollowUp {
		t.Fatalf("expected follow_up, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}
	if !strings.Contains(strings.ToLower(turn1.Reply.Text), "address") {
		t.Fatalf("expected address follow-up, got %q", turn1.Reply.Text)
	}

	turn2 := postAssistantMessage(t, authedUser.client, sessionID, "Cempaka Putih Tengah 20")
	if turn2.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation, got %q (%s)", turn2.Reply.Type, turn2.Reply.Text)
	}
	if !strings.Contains(turn2.Reply.Text, "pick my printer") {
		t.Fatalf("expected confirmation to mention title, got %q", turn2.Reply.Text)
	}

	turn3 := postAssistantMessage(t, authedUser.client, sessionID, "yes")
	if turn3.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", turn3.Reply.Type, turn3.Reply.Text)
	}
	if turn3.Reply.Action == nil || turn3.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful action, got %+v", turn3.Reply.Action)
	}
	if turn3.Reply.Action.Payload == nil {
		t.Fatal("expected client payload in action")
	}
	if turn3.Reply.Action.Payload["reminder_id"] == nil {
		t.Fatalf("expected reminder_id in payload, got %+v", turn3.Reply.Action.Payload)
	}
	if turn3.Reply.Action.Payload["latitude"] == nil || turn3.Reply.Action.Payload["longitude"] == nil {
		t.Fatalf("expected lat/lng in payload, got %+v", turn3.Reply.Action.Payload)
	}
	if radius, ok := turn3.Reply.Action.Payload["radius_meters"].(float64); !ok || int(radius) != 100 {
		t.Fatalf("expected radius_meters=100, got %+v", turn3.Reply.Action.Payload["radius_meters"])
	}

	items := listLocationReminders(t, authedUser.client)
	if len(items) != 1 {
		t.Fatalf("expected 1 location reminder, got %d: %+v", len(items), items)
	}
	if items[0].Title != "pick my printer" {
		t.Fatalf("expected title pick my printer, got %q", items[0].Title)
	}
	if items[0].Latitude == nil || items[0].Longitude == nil {
		t.Fatalf("expected coordinates on reminder, got %+v", items[0])
	}
}

func TestLocationReminderPlaceKeywordFullFlow(t *testing.T) {
	authedUser := registerReminderUser(t)
	installLocationReminderPlugin(t, authedUser.client)
	sessionID := createAssistantSession(t, authedUser.client)

	turn1 := postAssistantMessage(t, authedUser.client, sessionID, "Remind me to buy candy")
	if turn1.Reply.Type != assistant.ReplyTypeFollowUp {
		t.Fatalf("expected follow_up, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}

	turn2 := postAssistantMessage(t, authedUser.client, sessionID, "any nearby Alfamart")
	if turn2.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation, got %q (%s)", turn2.Reply.Type, turn2.Reply.Text)
	}
	if !strings.Contains(turn2.Reply.Text, "buy candy") {
		t.Fatalf("expected confirmation to mention title, got %q", turn2.Reply.Text)
	}

	turn3 := postAssistantMessage(t, authedUser.client, sessionID, "yes")
	if turn3.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", turn3.Reply.Type, turn3.Reply.Text)
	}
	if turn3.Reply.Action == nil || turn3.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful action, got %+v", turn3.Reply.Action)
	}
	payload := turn3.Reply.Action.Payload
	if payload == nil {
		t.Fatal("expected client payload")
	}
	if payload["location_mode"] != "place_keyword" {
		t.Fatalf("expected place_keyword mode, got %+v", payload["location_mode"])
	}
	if payload["place_keyword"] != "Alfamart" {
		t.Fatalf("expected place_keyword Alfamart, got %+v", payload["place_keyword"])
	}
	if payload["latitude"] != nil || payload["longitude"] != nil {
		t.Fatalf("expected null coordinates, got lat=%v lng=%v", payload["latitude"], payload["longitude"])
	}

	items := listLocationReminders(t, authedUser.client)
	if len(items) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(items))
	}
	if items[0].PlaceKeyword == nil || *items[0].PlaceKeyword != "Alfamart" {
		t.Fatalf("expected place_keyword Alfamart, got %+v", items[0].PlaceKeyword)
	}
	if items[0].Latitude != nil || items[0].Longitude != nil {
		t.Fatalf("expected null coordinates on reminder, got lat=%v lng=%v", items[0].Latitude, items[0].Longitude)
	}
}

func TestLocationReminderConfirmationDeclined(t *testing.T) {
	authedUser := registerReminderUser(t)
	installLocationReminderPlugin(t, authedUser.client)
	sessionID := createAssistantSession(t, authedUser.client)

	postAssistantMessage(t, authedUser.client, sessionID, "Remind me to buy candy")
	postAssistantMessage(t, authedUser.client, sessionID, "any nearby Alfamart")

	turn := postAssistantMessage(t, authedUser.client, sessionID, "no")
	if turn.Reply.Type != assistant.ReplyTypeText {
		t.Fatalf("expected text reply, got %q", turn.Reply.Type)
	}
	if !strings.Contains(strings.ToLower(turn.Reply.Text), "won't") {
		t.Fatalf("expected cancellation text, got %q", turn.Reply.Text)
	}

	items := listLocationReminders(t, authedUser.client)
	if len(items) != 0 {
		t.Fatalf("expected no reminders, got %d", len(items))
	}
}

func TestLocationReminderGeocodeFailure(t *testing.T) {
	authedUser := registerReminderUser(t)
	installLocationReminderPlugin(t, authedUser.client)
	sessionID := createAssistantSession(t, authedUser.client)

	postAssistantMessage(t, authedUser.client, sessionID, "Remind me to pick my printer once I got home")
	postAssistantMessage(t, authedUser.client, sessionID, "unknown ambiguous street 99")
	turn := postAssistantMessage(t, authedUser.client, sessionID, "yes")

	if turn.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", turn.Reply.Type, turn.Reply.Text)
	}
	if turn.Reply.Action == nil || turn.Reply.Action.Status != assistant.ActionStatusFailed {
		t.Fatalf("expected failed action, got %+v", turn.Reply.Action)
	}
	if !strings.Contains(strings.ToLower(turn.Reply.Text), "address") {
		t.Fatalf("expected address error text, got %q", turn.Reply.Text)
	}

	items := listLocationReminders(t, authedUser.client)
	if len(items) != 0 {
		t.Fatalf("expected no reminders after geocode failure, got %d", len(items))
	}
}
