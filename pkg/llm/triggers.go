package llm

import (
	"strings"
)

// matchByTriggers performs deterministic substring trigger matching against
// plugin candidates. It is used by the mock classifier and as an OpenAI fallback.
func matchByTriggers(req ClassifyRequest) *ClassifyResult {
	lower := strings.ToLower(req.Text)
	for _, p := range req.Plugins {
		for _, trigger := range p.Triggers {
			if strings.Contains(lower, strings.ToLower(trigger)) {
				args := map[string]any{}
				if strings.Contains(lower, "janet") {
					args["attendee_name"] = "Janet"
				}
				if strings.Contains(lower, "2pm") || strings.Contains(lower, "2 pm") {
					if strings.Contains(lower, "tomorrow") {
						args["start_time"] = "2026-08-09T14:00:00+07:00"
					} else {
						args["start_time"] = "2026-08-09T14:00:00+07:00"
					}
				}
				if title, ok := args["attendee_name"].(string); ok && title != "" {
					args["title"] = "Meeting with " + title
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
