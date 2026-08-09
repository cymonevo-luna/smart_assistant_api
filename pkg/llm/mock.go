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
	agent    *PluginDelegationAgent
	classify func(req ClassifyRequest) *ClassifyResult
}

// NewMockClassifier returns a mock that semantically routes common intents.
func NewMockClassifier() *MockClassifier {
	return &MockClassifier{
		agent:    &PluginDelegationAgent{Provider: "mock"},
		classify: mockSemanticClassify,
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
	return mockSemanticClassify(req), nil
}

func mockSemanticClassify(req ClassifyRequest) *ClassifyResult {
	lower := strings.ToLower(req.Text)
	if isClearlyUnrelated(lower) {
		return &ClassifyResult{Matched: false}
	}

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
		if result := matchCalendarIntent(lower, p); result != nil {
			return result
		}
	}
	return &ClassifyResult{Matched: false}
}

func isClearlyUnrelated(lower string) bool {
	for _, phrase := range []string{
		"what is the weather",
		"what's the weather",
		"weather today",
		"weather like",
		"weather in",
		"turn on lights",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func matchLocationReminderIntent(lower string, p PluginCandidate) *ClassifyResult {
	if p.Slug != "set-reminder" {
		return nil
	}

	locationCues := []string{
		"nearby", "alfamart", "supermarket",
		"when i'm near", "when i am near", "when i'm at", "when i arrive",
		"alert me when i'm near", "alert me when i am near",
	}
	hasLocationCue := false
	for _, cue := range locationCues {
		if strings.Contains(lower, cue) {
			hasLocationCue = true
			break
		}
	}

	if hasLocationCue {
		args := map[string]any{
			"place_query":   lower,
			"location_mode": "place_keyword",
		}
		if title := extractLocationReminderTitle(lower); title != "" {
			args["title"] = title
		}
		return &ClassifyResult{
			Matched:    true,
			PluginSlug: p.Slug,
			Arguments:  args,
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
	for _, token := range []string{
		"am", "pm", "1pm", "2pm", "3pm", "4pm", "5pm",
		"today", "tomorrow",
		"morning", "afternoon", "evening", "noon", "tonight",
	} {
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
		"alert me when i'm near ",
		"alert me when i am near ",
	} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			rest := strings.TrimSpace(lower[idx+len(prefix):])
			for _, cut := range []string{" once i", " when i", " at ", " near "} {
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
	if !isReminderPlugin {
		return nil
	}

	if isLocationReminderIntent(lower) {
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

	if isReminderCreateIntent(lower) {
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

func isLocationReminderIntent(lower string) bool {
	for _, cue := range []string{
		"when i'm near", "when i am near", "near a supermarket", "nearby",
	} {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

func isReminderCreateIntent(lower string) bool {
	if strings.Contains(lower, "remind") {
		return true
	}
	for _, phrase := range []string{
		"ping me",
		"don't let me forget",
		"nudge me",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func extractReminderCreateMessage(lower string) string {
	for _, prefix := range []string{
		"remind me to ",
		"remind me ",
		"ping me about ",
		"ping me to ",
		"don't let me forget to ",
		"don't let me forget ",
		"nudge me to ",
		"nudge me about ",
	} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			rest := strings.TrimSpace(lower[idx+len(prefix):])
			if atIdx := strings.Index(rest, " at "); atIdx >= 0 {
				rest = strings.TrimSpace(rest[:atIdx])
			}
			return stripTrailingPeriodOfDay(rest)
		}
	}
	return ""
}

// stripTrailingPeriodOfDay removes trailing period-of-day tokens from a reminder message.
func stripTrailingPeriodOfDay(rest string) string {
	periodTokens := []string{"morning", "afternoon", "evening", "noon", "tonight"}
	for _, token := range periodTokens {
		for _, prefix := range []string{"this ", "today "} {
			suffix := prefix + token
			if strings.HasSuffix(rest, " "+suffix) {
				return strings.TrimSpace(strings.TrimSuffix(rest, " "+suffix))
			}
		}
		if strings.HasSuffix(rest, " "+token) {
			return strings.TrimSpace(strings.TrimSuffix(rest, " "+token))
		}
	}
	return rest
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

func matchCalendarIntent(lower string, p PluginCandidate) *ClassifyResult {
	if !isCalendarMeetSlug(p.Slug) {
		return nil
	}

	calendarCues := []string{
		"schedule", "book time", "book a call", "meeting with", "set up a call", "calendar",
	}
	hasCue := false
	for _, cue := range calendarCues {
		if strings.Contains(lower, cue) {
			hasCue = true
			break
		}
	}
	if !hasCue {
		return nil
	}

	args := map[string]any{}
	if strings.Contains(lower, "kezia") && strings.Contains(lower, "albert") {
		args["attendee_names"] = "Kezia, Albert"
	} else if strings.Contains(lower, "janet") {
		args["attendee_names"] = "Janet"
	}
	if strings.Contains(lower, "2pm") || strings.Contains(lower, "2 pm") {
		args["start_time"] = "2026-08-09T14:00:00+07:00"
	} else if strings.Contains(lower, " at 2") || strings.Contains(lower, " at 2pm") {
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

func isCalendarMeetSlug(slug string) bool {
	return slug == "google-calendar-meet" || strings.HasPrefix(slug, "google-calendar-meet-")
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
	} else if periodHour, ok := periodOfDayHour(lower); ok {
		hour = periodHour
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

// periodOfDayHour maps period-of-day tokens in text to a default hour (UTC).
func periodOfDayHour(lower string) (hour int, ok bool) {
	for _, entry := range []struct {
		token string
		hour  int
	}{
		{"afternoon", 14},
		{"tonight", 18},
		{"evening", 18},
		{"morning", 9},
		{"noon", 12},
	} {
		if strings.Contains(lower, entry.token) {
			return entry.hour, true
		}
	}
	return 0, false
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
