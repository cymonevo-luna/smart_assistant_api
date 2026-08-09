package llm

import (
	"context"
	"testing"
	"time"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

func TestMockClassifierRecordsInput(t *testing.T) {
	mock := NewMockClassifier()
	_, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "turn on lights",
		Plugins: []PluginCandidate{{
			Slug:     "lights",
			Triggers: []string{"turn on lights"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mock.LastText != "turn on lights" {
		t.Fatalf("LastText = %q, want %q", mock.LastText, "turn on lights")
	}
}

func TestMockClassifierMatchesScheduleMeeting(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "book time with Janet at 2pm tomorrow",
		Plugins: []PluginCandidate{{
			Slug:        "google-calendar-meet",
			Description: "Schedule calendar meetings and video calls",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "google-calendar-meet" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Arguments["attendee_names"] != "Janet" {
		t.Fatalf("attendee_names = %v", result.Arguments["attendee_names"])
	}
	if result.Arguments["start_time"] == "" {
		t.Fatal("expected start_time for Janet explicit-time case")
	}
}

func TestMockClassifierMatchesMultiAttendeeNoTime(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "schedule a meeting with kezia and albert",
		Plugins: []PluginCandidate{{
			Slug:        "google-calendar-meet",
			Description: "Schedule calendar meetings and video calls",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "google-calendar-meet" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Arguments["attendee_names"] != "Kezia, Albert" {
		t.Fatalf("attendee_names = %v", result.Arguments["attendee_names"])
	}
	if _, ok := result.Arguments["start_time"]; ok {
		t.Fatalf("expected no start_time, got %v", result.Arguments["start_time"])
	}
}

func TestMockClassifierParaphrasedReminder(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "ping me about groceries at 5pm",
		Plugins: []PluginCandidate{{
			Slug:        "reminder",
			Name:        "Reminder",
			Description: "Create, list, and delete time-based reminders",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "reminder" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Arguments["operation"] != "create" {
		t.Fatalf("operation = %v", result.Arguments["operation"])
	}
	if result.Arguments["message"] != "groceries" {
		t.Fatalf("message = %v", result.Arguments["message"])
	}
}

func TestMockClassifierMatchesReminderIntents(t *testing.T) {
	mock := NewMockClassifier()
	plugins := []PluginCandidate{{
		Slug:        "reminder",
		Description: "Create, list, and delete time-based reminders",
	}}

	createResult, err := mock.Classify(context.Background(), ClassifyRequest{
		Text:    "don't let me forget to call mom at 2 pm today",
		Plugins: plugins,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !createResult.Matched || createResult.PluginSlug != "reminder" {
		t.Fatalf("create match: %+v", createResult)
	}
	if createResult.Arguments["operation"] != "create" {
		t.Fatalf("operation = %v", createResult.Arguments["operation"])
	}
	if createResult.Arguments["message"] != "call mom" {
		t.Fatalf("message = %v", createResult.Arguments["message"])
	}
	if createResult.Arguments["remind_at"] == "" {
		t.Fatal("expected remind_at")
	}

	listResult, err := mock.Classify(context.Background(), ClassifyRequest{
		Text:    "list all reminders for today",
		Plugins: plugins,
	})
	if err != nil {
		t.Fatal(err)
	}
	if listResult.Arguments["operation"] != "list" || listResult.Arguments["filter"] != "today" {
		t.Fatalf("list args: %+v", listResult.Arguments)
	}

	deleteResult, err := mock.Classify(context.Background(), ClassifyRequest{
		Text:    "delete my reminder for call mom",
		Plugins: plugins,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deleteResult.Arguments["operation"] != "delete" || deleteResult.Arguments["message"] != "call mom" {
		t.Fatalf("delete args: %+v", deleteResult.Arguments)
	}
}

func TestMockClassifierAfternoonReminder(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "remind me to buy groceries afternoon",
		Plugins: []PluginCandidate{{
			Slug:        "reminder",
			Description: "Create, list, and delete time-based reminders",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "reminder" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Arguments["operation"] != "create" {
		t.Fatalf("operation = %v", result.Arguments["operation"])
	}
	if result.Arguments["message"] != "buy groceries" {
		t.Fatalf("message = %v, want %q", result.Arguments["message"], "buy groceries")
	}
	remindAt, ok := result.Arguments["remind_at"].(string)
	if !ok || remindAt == "" {
		t.Fatal("expected remind_at")
	}
	parsed, err := time.Parse(time.RFC3339, remindAt)
	if err != nil {
		t.Fatalf("remind_at not valid RFC3339: %v", err)
	}
	if parsed.Hour() != 14 {
		t.Fatalf("remind_at hour = %d, want 14", parsed.Hour())
	}
}

func TestMockClassifierAfternoonNotRoutedToLocationReminder(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "remind me to buy groceries afternoon",
		Plugins: []PluginCandidate{
			{
				Slug:        "reminder",
				Description: "Create, list, and delete time-based reminders",
			},
			{
				Slug:        "set-reminder",
				Description: "Create location-based reminders",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "reminder" {
		t.Fatalf("expected reminder plugin, got: %+v", result)
	}
	if result.Arguments["message"] != "buy groceries" {
		t.Fatalf("message = %v, want %q", result.Arguments["message"], "buy groceries")
	}
	if _, hasTitle := result.Arguments["title"]; hasTitle {
		t.Fatalf("expected message (not title), got title: %v", result.Arguments["title"])
	}
}

func TestMockClassifierLocationReminderStillRoutes(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "alert me when I'm near a supermarket",
		Plugins: []PluginCandidate{{
			Slug:        "set-reminder",
			Description: "Create location-based reminders when arriving near a place",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "set-reminder" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMockClassifierReturnsNoMatchForWeather(t *testing.T) {
	mock := NewMockClassifier()
	result, err := mock.Classify(context.Background(), ClassifyRequest{
		Text: "what is the weather today",
		Plugins: []PluginCandidate{{
			Slug:        "google-calendar-meet",
			Description: "Schedule calendar meetings",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched {
		t.Fatalf("expected no match for weather, got %+v", result)
	}
}

func TestPluginDelegationAgentMockProvider(t *testing.T) {
	agent := &PluginDelegationAgent{Provider: "mock"}
	result, err := agent.Classify(context.Background(), ClassifyRequest{
		Text: "ping me about groceries at 5pm",
		Plugins: []PluginCandidate{{
			Slug:        "reminder",
			Description: "Time-based reminders",
			Arguments: []plugin.ManifestArgument{
				{Name: "operation", Type: "string"},
				{Name: "message", Type: "string"},
				{Name: "remind_at", Type: "datetime"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "reminder" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
