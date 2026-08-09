package llm

import (
	"context"
	"testing"
	"time"
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
		Text: "schedule meeting with Janet at 2pm tomorrow",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule meeting"},
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
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule a meeting"},
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

func TestMockClassifierMatchesReminderIntents(t *testing.T) {
	mock := NewMockClassifier()
	plugins := []PluginCandidate{{
		Slug:     "reminder",
		Triggers: []string{"remind me", "list reminders", "delete reminder"},
	}}

	createResult, err := mock.Classify(context.Background(), ClassifyRequest{
		Text:    "remind me to call mom at 2 pm today",
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
			Slug:     "reminder",
			Triggers: []string{"remind me", "list reminders", "delete reminder"},
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
				Slug:     "reminder",
				Triggers: []string{"remind me", "list reminders", "delete reminder"},
			},
			{
				Slug:     "set-reminder",
				Triggers: []string{"remind me", "set a location reminder"},
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
		Text: "remind me to buy candy",
		Plugins: []PluginCandidate{{
			Slug:     "set-reminder",
			Triggers: []string{"remind me", "set a location reminder"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "set-reminder" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Arguments["title"] != "buy candy" {
		t.Fatalf("title = %v, want %q", result.Arguments["title"], "buy candy")
	}
}
