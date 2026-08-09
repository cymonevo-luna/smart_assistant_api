package llm

import (
	"testing"
)

func TestMatchByTriggersLegacy(t *testing.T) {
	result := matchByTriggers(ClassifyRequest{
		Text: "schedule a meeting with Janet at 2pm",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule a meeting"},
		}},
	})
	if !result.Matched || result.PluginSlug != "google-calendar-meet" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
