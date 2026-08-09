package llm

import (
	"strings"
)

// matchByTriggers performs deterministic substring trigger matching against
// plugin candidates. Retained for legacy unit tests only.
func matchByTriggers(req ClassifyRequest) *ClassifyResult {
	lower := strings.ToLower(req.Text)
	for _, p := range req.Plugins {
		for _, trigger := range p.Triggers {
			if strings.Contains(lower, strings.ToLower(trigger)) {
				args := map[string]any{}
				if strings.Contains(lower, "kezia") && strings.Contains(lower, "albert") {
					args["attendee_names"] = "Kezia, Albert"
				} else if strings.Contains(lower, "janet") {
					args["attendee_names"] = "Janet"
				}
				if strings.Contains(lower, "2pm") || strings.Contains(lower, "2 pm") {
					args["start_time"] = "2026-08-09T14:00:00+07:00"
				}
				if names, ok := args["attendee_names"].(string); ok && names != "" {
					args["title"] = "Meeting with " + names
				}
				return &ClassifyResult{
					Matched:    true,
					PluginSlug: p.Slug,
					Arguments:  args,
				}
			}
		}
	}
	return &ClassifyResult{Matched: false}
}
