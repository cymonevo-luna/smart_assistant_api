//go:build integration

package integration

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

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
	mockComposioAPI.ResetRequests()
	mockComposioAPI.SetFail(false)
	mockComposioAPI.SetEmptyFreeSlots(false)

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
	if !strings.Contains(turn2.Reply.Text, "2 PM") {
		t.Fatalf("expected JKT time in confirmation, got %q", turn2.Reply.Text)
	}
	if mockComposioAPI.ToolCallCount(findFreeSlotsTool) != 0 {
		t.Fatalf("expected no FIND_FREE_SLOTS calls for explicit-time flow, got %d", mockComposioAPI.ToolCallCount(findFreeSlotsTool))
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
	if last["slug"] != createEventTool {
		t.Fatalf("expected %s, got %v", createEventTool, last["slug"])
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

func TestGoogleCalendarMeetRecommendedTimeFlow(t *testing.T) {
	mockComposioAPI.ResetRequests()
	mockComposioAPI.SetFail(false)
	mockComposioAPI.SetEmptyFreeSlots(false)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": googleCalendarMeetSlug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	completeGoogleOAuthSetup(t, authed, installed.ID)

	sessionID := createAssistantSession(t, authed)

	steps := []struct {
		text       string
		wantType   assistant.ReplyType
		wantSubstr []string
	}{
		{
			text:       "Schedule a meeting with Kezia and Albert",
			wantType:   assistant.ReplyTypeFollowUp,
			wantSubstr: []string{"Kezia", "email"},
		},
		{
			text:       "kezia@example.com",
			wantType:   assistant.ReplyTypeFollowUp,
			wantSubstr: []string{"Albert", "email"},
		},
		{
			text:       "albert@example.com",
			wantType:   assistant.ReplyTypeFollowUp,
			wantSubstr: []string{"When should I set the schedule?"},
		},
		{
			text:       "Find a time",
			wantType:   assistant.ReplyTypeConfirmation,
			wantSubstr: []string{"Friday", "2 PM", "okay"},
		},
		{
			text:     "yes",
			wantType: assistant.ReplyTypeActionResult,
		},
	}

	var last assistant.ProcessMessageResponse
	for i, step := range steps {
		resp := authed.post("/api/v1/assistant/sessions/"+sessionID+"/messages", map[string]any{
			"text":   step.text,
			"source": "button",
		})
		resp.requireStatus(t, http.StatusOK)
		resp.decode(t, &last)

		if last.Reply.Type != step.wantType {
			t.Fatalf("step %d: expected %q, got %q (%s)", i+1, step.wantType, last.Reply.Type, last.Reply.Text)
		}
		for _, substr := range step.wantSubstr {
			if !strings.Contains(last.Reply.Text, substr) {
				t.Fatalf("step %d: expected reply to contain %q, got %q", i+1, substr, last.Reply.Text)
			}
		}
	}

	if last.Reply.Action == nil || last.Reply.Action.Status != assistant.ActionStatusSuccess {
		t.Fatalf("expected successful action, got %+v", last.Reply.Action)
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Now().In(loc)
	var expectedFriday time.Time
	for d := 0; d < 14; d++ {
		candidate := now.AddDate(0, 0, d)
		if candidate.Weekday() == time.Friday {
			expectedFriday = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 14, 0, 0, 0, loc)
			break
		}
	}

	lastReq := mockComposioAPI.LastRequest()
	if lastReq == nil {
		t.Fatal("expected composio execute request")
	}
	if lastReq["slug"] != createEventTool {
		t.Fatalf("expected %s, got %v", createEventTool, lastReq["slug"])
	}
	args, _ := lastReq["arguments"].(map[string]any)
	if args == nil {
		t.Fatalf("expected arguments, got %+v", lastReq)
	}
	startDT, _ := args["start_datetime"].(string)
	if startDT != expectedFriday.Format(time.RFC3339) {
		t.Fatalf("start_datetime = %q, want %q", startDT, expectedFriday.Format(time.RFC3339))
	}
	attendees, _ := args["attendees"].([]any)
	if len(attendees) != 2 {
		t.Fatalf("attendees = %#v", attendees)
	}
	emails := map[string]bool{attendees[0].(string): true, attendees[1].(string): true}
	if !emails["kezia@example.com"] || !emails["albert@example.com"] {
		t.Fatalf("unexpected attendees: %#v", attendees)
	}
}

func TestGoogleCalendarMeetNoAvailability(t *testing.T) {
	mockComposioAPI.ResetRequests()
	mockComposioAPI.SetFail(false)
	mockComposioAPI.SetEmptyFreeSlots(true)
	t.Cleanup(func() {
		mockComposioAPI.SetEmptyFreeSlots(false)
	})

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
		"Schedule a meeting with Kezia and Albert",
		"kezia@example.com",
		"albert@example.com",
		"pick a time",
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

	if last.Reply.Type != assistant.ReplyTypeText {
		t.Fatalf("expected text reply, got %q (%s)", last.Reply.Type, last.Reply.Text)
	}
	if !strings.Contains(strings.ToLower(last.Reply.Text), "couldn't find") {
		t.Fatalf("expected no-availability message, got %q", last.Reply.Text)
	}
	if mockComposioAPI.ToolCallCount(createEventTool) != 0 {
		t.Fatalf("expected no CREATE_EVENT call, got %d", mockComposioAPI.ToolCallCount(createEventTool))
	}
}

var confirmationPattern = regexp.MustCompile(`^Is \w+ at \d{1,2} (AM|PM) okay\?$`)

func TestGoogleCalendarMeetConfirmationPattern(t *testing.T) {
	if !confirmationPattern.MatchString("Is Friday at 2 PM okay?") {
		t.Fatal("expected confirmation pattern to match sample text")
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
