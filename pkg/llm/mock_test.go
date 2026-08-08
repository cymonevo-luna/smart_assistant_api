package llm

import (
	"context"
	"testing"
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
}
