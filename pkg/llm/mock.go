package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MockClassifier is a deterministic classifier for tests and CI.
type MockClassifier struct {
	mu       sync.Mutex
	LastText string
	LastReq  *ClassifyRequest
	classify func(req ClassifyRequest) *ClassifyResult
}

// NewMockClassifier returns a mock that matches schedule-meeting intents.
func NewMockClassifier() *MockClassifier {
	return &MockClassifier{
		classify: defaultMockClassify,
	}
}

// WithClassify overrides the classification logic (for unit tests).
func (m *MockClassifier) WithClassify(fn func(req ClassifyRequest) *ClassifyResult) *MockClassifier {
	m.classify = fn
	return m
}

// Classify records the request and returns a deterministic result.
func (m *MockClassifier) Classify(_ context.Context, req ClassifyRequest) (*ClassifyResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastText = req.Text
	reqCopy := req
	m.LastReq = &reqCopy
	if m.classify != nil {
		return m.classify(req), nil
	}
	return defaultMockClassify(req), nil
}

func defaultMockClassify(req ClassifyRequest) *ClassifyResult {
	lower := strings.ToLower(req.Text)
	for _, p := range req.Plugins {
		if result := matchLocationReminderIntent(lower, p); result != nil {
			return result
		}
	}
	for _, p := range req.Plugins {
		if result := matchReminderIntent(lower, p); result != nil {
			return result
		}
	}
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

func matchLocationReminderIntent(lower string, p PluginCandidate) *ClassifyResult {
	isLocationPlugin := p.Slug == "set-reminder"
	for _, trigger := range p.Triggers {
		triggerLower := strings.ToLower(trigger)
		if strings.Contains(lower, triggerLower) {
			isLocationPlugin = true
			break
		}
	}
	if !isLocationPlugin {
		return nil
	}

	if strings.Contains(lower, "nearby") || strings.Contains(lower, "alfamart") {
		return &ClassifyResult{
			Matched:    true,
			PluginSlug: p.Slug,
			Arguments: map[string]any{
				"place_query":   lower,
				"location_mode": "place_keyword",
			},
		}
	}

	if strings.Contains(lower, "pick my printer") && (strings.Contains(lower, "once i") || strings.Contains(lower, "got home")) {
		return &ClassifyResult{
			Matched:    true,
			PluginSlug: p.Slug,
			Arguments: map[string]any{
				"title":         "pick my printer",
				"location_mode": "exact",
				"place_query":   "home",
			},
		}
	}

	if strings.Contains(lower, "buy candy") {
		return &ClassifyResult{
			Matched:    true,
			PluginSlug: p.Slug,
			Arguments: map[string]any{
				"title": "buy candy",
			},
		}
	}

	if strings.Contains(lower, "remind me to ") && !reminderTextHasTime(lower) {
		title := extractLocationReminderTitle(lower)
		if title != "" {
			args := map[string]any{"title": title}
			if strings.Contains(lower, "once i") || strings.Contains(lower, "when i get") || strings.Contains(lower, "when i arrive") {
				args["location_mode"] = "exact"
				if place := extractLocationPlaceFromTrigger(lower); place != "" {
					args["place_query"] = place
				}
			}
			return &ClassifyResult{Matched: true, PluginSlug: p.Slug, Arguments: args}
		}
	}

	return nil
}

func reminderTextHasTime(lower string) bool {
	if strings.Contains(lower, " at ") {
		return true
	}
	for _, token := range []string{"am", "pm", "1pm", "2pm", "3pm", "today", "tomorrow"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func extractLocationReminderTitle(lower string) string {
	for _, prefix := range []string{
		"remind me to ",
		"remind me when i arrive to ",
		"remind me when i get to ",
		"remind me once i got ",
		"remind me once i ",
		"set a location reminder to ",
	} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			rest := strings.TrimSpace(lower[idx+len(prefix):])
			for _, cut := range []string{" once i", " when i", " at "} {
				if at := strings.Index(rest, cut); at >= 0 {
					rest = strings.TrimSpace(rest[:at])
				}
			}
			return rest
		}
	}
	return ""
}

func extractLocationPlaceFromTrigger(lower string) string {
	for _, prefix := range []string{
		"once i got ",
		"once i ",
		"when i get to ",
		"when i get ",
		"when i arrive at ",
		"when i arrive ",
	} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			return strings.TrimSpace(lower[idx+len(prefix):])
		}
	}
	return ""
}

func matchReminderIntent(lower string, p PluginCandidate) *ClassifyResult {
	isReminderPlugin := p.Slug == "reminder" || strings.HasPrefix(p.Slug, "reminder-")
	for _, trigger := range p.Triggers {
		if strings.Contains(lower, strings.ToLower(trigger)) {
			isReminderPlugin = true
			break
		}
	}
	if !isReminderPlugin {
		return nil
	}

	if strings.Contains(lower, "list") && strings.Contains(lower, "reminder") {
		args := map[string]any{"operation": "list"}
		switch {
		case strings.Contains(lower, "tomorrow"):
			args["filter"] = "tomorrow"
		case strings.Contains(lower, "for today") || (strings.Contains(lower, "today") && !strings.Contains(lower, "all reminders")):
			args["filter"] = "today"
		case strings.Contains(lower, "all"):
			args["filter"] = "all"
		default:
			args["filter"] = "today"
		}
		return &ClassifyResult{Matched: true, PluginSlug: p.Slug, Arguments: args}
	}

	if (strings.Contains(lower, "delete") || strings.Contains(lower, "remove")) && strings.Contains(lower, "reminder") {
		return &ClassifyResult{
			Matched:    true,
			PluginSlug: p.Slug,
			Arguments: map[string]any{
				"operation": "delete",
				"message":   extractReminderDeleteMessage(lower),
			},
		}
	}

	if strings.Contains(lower, "remind") {
		args := map[string]any{
			"operation": "create",
			"message":   extractReminderCreateMessage(lower),
		}
		if strings.Contains(lower, "past reminder test") {
			args["remind_at"] = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		} else {
			args["remind_at"] = parseReminderTimeFromText(lower)
		}
		return &ClassifyResult{Matched: true, PluginSlug: p.Slug, Arguments: args}
	}

	return nil
}

func extractReminderCreateMessage(lower string) string {
	for _, prefix := range []string{"remind me to ", "remind me "} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			rest := strings.TrimSpace(lower[idx+len(prefix):])
			if atIdx := strings.Index(rest, " at "); atIdx >= 0 {
				rest = strings.TrimSpace(rest[:atIdx])
			}
			return rest
		}
	}
	return ""
}

func extractReminderDeleteMessage(lower string) string {
	for _, prefix := range []string{
		"delete my reminder for ",
		"delete reminder for ",
		"remove my reminder for ",
		"remove reminder for ",
	} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			return strings.TrimSpace(lower[idx+len(prefix):])
		}
	}
	return ""
}

func parseReminderTimeFromText(lower string) string {
	hour := 14
	minute := 0

	if idx := strings.Index(lower, " at "); idx >= 0 {
		segment := lower[idx+4:]
		for _, suffix := range []string{" today", " tomorrow"} {
			if cut := strings.Index(segment, suffix); cut >= 0 {
				segment = segment[:cut]
			}
		}
		segment = strings.TrimSpace(segment)
		if parsedHour, parsedMinute, ok := parseClock(segment); ok {
			hour, minute = parsedHour, parsedMinute
		}
	}

	now := time.Now().UTC()
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if strings.Contains(lower, "tomorrow") {
		target = target.Add(24 * time.Hour)
	} else if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Format(time.RFC3339)
}

func parseClock(segment string) (hour, minute int, ok bool) {
	segment = strings.TrimSpace(strings.ToLower(segment))
	segment = strings.ReplaceAll(segment, ".", "")
	isPM := strings.Contains(segment, "pm")
	isAM := strings.Contains(segment, "am")
	segment = strings.ReplaceAll(segment, "pm", "")
	segment = strings.ReplaceAll(segment, "am", "")
	segment = strings.TrimSpace(segment)
	segment = strings.ReplaceAll(segment, " ", "")

	if strings.Contains(segment, ":") {
		parts := strings.SplitN(segment, ":", 2)
		if len(parts) != 2 {
			return 0, 0, false
		}
		var h, m int
		if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
			return 0, 0, false
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
			return 0, 0, false
		}
		hour = h
		minute = m
	} else {
		if _, err := fmt.Sscanf(segment, "%d", &hour); err != nil {
			return 0, 0, false
		}
	}

	if isPM && hour < 12 {
		hour += 12
	}
	if isAM && hour == 12 {
		hour = 0
	}
	return hour, minute, true
}
