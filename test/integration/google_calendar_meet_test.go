//go:build integration

package integration

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/plugin_setup/oauth_google"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
)

const googleCalendarMeetSlug = "google-calendar-meet"

func completeGoogleOAuthSetup(t *testing.T, authed *client, installID string) {
	t.Helper()
	setup := authed.post("/api/v1/users/me/plugins/"+installID+"/setup", nil)
	setup.requireStatus(t, http.StatusOK)

	var initResp oauthgoogle.SetupInitResponse
	setup.decode(t, &initResp)

	callbackURL := server.URL + "/api/v1/plugins/oauth/google/callback?code=test-code&state=" + url.QueryEscape(initResp.State)
	resp, err := httpGetNoRedirect(callbackURL)
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d body %s", resp.StatusCode, body)
	}
}

func TestGoogleCalendarMeetPluginVisibleInCatalog(t *testing.T) {
	authed := registerAndLogin(t)
	list := authed.get("/api/v1/plugins")
	list.requireStatus(t, http.StatusOK)

	var plugins []plugin.CatalogSummary
	list.decode(t, &plugins)

	var meet *plugin.CatalogSummary
	for i := range plugins {
		if plugins[i].Slug == googleCalendarMeetSlug {
			meet = &plugins[i]
			break
		}
	}
	if meet == nil {
		t.Fatalf("expected %q in catalog, got %+v", googleCalendarMeetSlug, plugins)
	}
	if !meet.RequiredSetup {
		t.Fatal("expected required_setup true")
	}
}

func TestGoogleCalendarMeetPluginFlow(t *testing.T) {
	mockComposioAPI.SetFail(false)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": googleCalendarMeetSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	completeGoogleOAuthSetup(t, authed, installed.ID)

	sessionID := createAssistantSession(t, authed)

	first := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "Schedule a meeting with Janet at 2 PM tomorrow",
		"source": "button",
	})
	first.requireStatus(t, http.StatusOK)

	var turn1 assistant.ProcessMessageResponse
	first.decode(t, &turn1)
	if turn1.Reply.Type != assistant.ReplyTypeFollowUp {
		t.Fatalf("expected follow_up, got %q (%s)", turn1.Reply.Type, turn1.Reply.Text)
	}
	if !strings.Contains(turn1.Reply.Text, "Janet") || !strings.Contains(strings.ToLower(turn1.Reply.Text), "email") {
		t.Fatalf("expected attendee email follow-up, got %q", turn1.Reply.Text)
	}

	second := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "janet@gmail.com",
		"source": "button",
	})
	second.requireStatus(t, http.StatusOK)

	var turn2 assistant.ProcessMessageResponse
	second.decode(t, &turn2)
	if turn2.Reply.Type != assistant.ReplyTypeConfirmation {
		t.Fatalf("expected confirmation, got %q (%s)", turn2.Reply.Type, turn2.Reply.Text)
	}
	if !strings.Contains(turn2.Reply.Text, "janet@gmail.com") {
		t.Fatalf("expected email in confirmation, got %q", turn2.Reply.Text)
	}

	third := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
		"text":   "yes",
		"source": "button",
	})
	third.requireStatus(t, http.StatusOK)

	var turn3 assistant.ProcessMessageResponse
	third.decode(t, &turn3)
	if turn3.Reply.Type != assistant.ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q (%s)", turn3.Reply.Type, turn3.Reply.Text)
	}
	if turn3.Reply.Action == nil || turn3.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful action, got %+v", turn3.Reply.Action)
	}

	last := mockComposioAPI.LastRequest()
	if last == nil {
		t.Fatal("expected composio execute request")
	}
	args, _ := last["arguments"].(map[string]any)
	if args == nil {
		t.Fatalf("expected arguments in composio request, got %+v", last)
	}
	if args["summary"] != "Meeting with Janet" {
		t.Fatalf("summary = %v", args["summary"])
	}
	attendees, _ := args["attendees"].([]any)
	if len(attendees) != 1 || attendees[0] != "janet@gmail.com" {
		t.Fatalf("attendees = %#v", args["attendees"])
	}
}

func TestGoogleCalendarMeetComposioFailureSurfacesError(t *testing.T) {
	mockComposioAPI.SetFail(true)
	t.Cleanup(func() { mockComposioAPI.SetFail(false) })

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": googleCalendarMeetSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	completeGoogleOAuthSetup(t, authed, installed.ID)

	sessionID := createAssistantSession(t, authed)

	steps := []string{
		"Schedule a meeting with Janet at 2 PM tomorrow",
		"janet@gmail.com",
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
	if strings.TrimSpace(last.Reply.Text) == "" {
		t.Fatal("expected non-empty error reply text")
	}
}
