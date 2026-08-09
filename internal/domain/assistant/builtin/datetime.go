package builtin

import (
	"fmt"
	"strings"
	"time"
)

// ParseDateTime parses a datetime string in the given location.
// Supports RFC3339 and common structured layouts, plus natural-language reminder
// phrases (e.g. "this afternoon", "tomorrow morning", "3pm today").
func ParseDateTime(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("datetime is required")
	}
	if loc == nil {
		loc = time.UTC
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}

	if t, ok := parseNaturalDateTime(raw, loc); ok {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("could not parse datetime %q", raw)
}

func parseNaturalDateTime(raw string, loc *time.Location) (time.Time, bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return time.Time{}, false
	}

	hour := 14
	minute := 0
	hasTime := false

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
			hasTime = true
		}
	} else if parsedHour, parsedMinute, ok := parseClockFromText(lower); ok {
		hour, minute = parsedHour, parsedMinute
		hasTime = true
	} else if periodHour, ok := periodOfDayHour(lower); ok {
		hour = periodHour
		hasTime = true
	}

	if !hasTime {
		return time.Time{}, false
	}

	now := time.Now().In(loc)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if strings.Contains(lower, "tomorrow") {
		target = target.Add(24 * time.Hour)
	} else if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target, true
}

// parseClockFromText extracts a clock time from text that may include day qualifiers.
func parseClockFromText(lower string) (hour, minute int, ok bool) {
	segment := lower
	for _, token := range []string{"today", "tomorrow", "this"} {
		segment = strings.ReplaceAll(segment, token, "")
	}
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return 0, 0, false
	}
	return parseClock(segment)
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
