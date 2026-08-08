package availability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cymonevo/go_template/pkg/composio"
)

const (
	// MeetingTimezone is the IANA timezone used for scheduling recommendations.
	MeetingTimezone = "Asia/Jakarta"
	// WorkDayStartHour is the earliest hour (inclusive) for a meeting start.
	WorkDayStartHour = 9
	// WorkDayEndHour is the exclusive end of the work day for slot start times;
	// a 1-hour meeting may start at 17:00 and end at 18:00.
	WorkDayEndHour = 18
	// DefaultMeetingDuration is the default meeting length when none is specified.
	DefaultMeetingDuration = time.Hour
	// SearchHorizonDays is how many days ahead to search for availability.
	SearchHorizonDays = 14

	findFreeSlotsTool = "GOOGLECALENDAR_FIND_FREE_SLOTS"
)

// ErrNoAvailableSlot is returned when no slot matches constraints within the horizon.
var ErrNoAvailableSlot = errors.New("no available meeting slot found within search horizon")

// RecommendSlotInput holds invitee emails; the organizer calendar is implicit via Composio.
type RecommendSlotInput struct {
	AttendeeEmails []string
}

// AvailabilityService recommends meeting slots based on calendar availability.
type AvailabilityService interface {
	RecommendSlot(ctx context.Context, userID string, input RecommendSlotInput) (time.Time, error)
}

// Service finds recommended meeting slots via Composio Google Calendar free/busy.
type Service struct {
	client *composio.Client
	now    func() time.Time
}

// NewService constructs an availability Service.
func NewService(client *composio.Client) *Service {
	return &Service{
		client: client,
		now:    time.Now,
	}
}

// RecommendSlot returns the earliest recommended start time for a meeting where the
// organizer and all invitees are available within working-hour constraints.
func (s *Service) RecommendSlot(ctx context.Context, userID string, input RecommendSlotInput) (time.Time, error) {
	loc, err := time.LoadLocation(MeetingTimezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("availability: load timezone: %w", err)
	}

	now := s.now().In(loc)
	timeMin := now.Format(time.RFC3339)
	timeMax := now.Add(SearchHorizonDays * 24 * time.Hour).Format(time.RFC3339)

	items := calendarItems(input.AttendeeEmails)
	args := map[string]any{
		"time_min": timeMin,
		"time_max": timeMax,
		"timezone": MeetingTimezone,
		"items":    items,
	}

	res, err := s.client.ForEntity(userID).Execute(ctx, findFreeSlotsTool, args)
	if err != nil {
		return time.Time{}, fmt.Errorf("availability: %w", err)
	}

	slots, err := parseFreeSlots(res.Data)
	if err != nil {
		return time.Time{}, fmt.Errorf("availability: parse free slots: %w", err)
	}

	starts := filterSlotStarts(slots, loc, DefaultMeetingDuration)
	if len(starts) == 0 {
		return time.Time{}, ErrNoAvailableSlot
	}

	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	return starts[0], nil
}

func calendarItems(attendeeEmails []string) []string {
	items := make([]string, 0, 1+len(attendeeEmails))
	items = append(items, "primary")
	seen := map[string]struct{}{"primary": {}}
	for _, email := range attendeeEmails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		lower := strings.ToLower(email)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		items = append(items, email)
	}
	return items
}

type timeInterval struct {
	start time.Time
	end   time.Time
}

func filterSlotStarts(slots []timeInterval, loc *time.Location, minDuration time.Duration) []time.Time {
	var starts []time.Time
	for _, slot := range slots {
		if slot.end.Sub(slot.start) < minDuration {
			continue
		}
		if !isValidSlotStart(slot.start.In(loc), minDuration, loc) {
			continue
		}
		starts = append(starts, slot.start.In(loc))
	}
	return starts
}

func isValidSlotStart(start time.Time, duration time.Duration, loc *time.Location) bool {
	start = start.In(loc)
	switch start.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}

	dayStart := time.Date(start.Year(), start.Month(), start.Day(), WorkDayStartHour, 0, 0, 0, loc)
	dayEnd := time.Date(start.Year(), start.Month(), start.Day(), WorkDayEndHour, 0, 0, 0, loc)
	if start.Before(dayStart) {
		return false
	}
	if !start.Add(duration).Before(dayEnd) && !start.Add(duration).Equal(dayEnd) {
		return false
	}
	return true
}

func parseFreeSlots(data json.RawMessage) ([]timeInterval, error) {
	if len(data) == 0 {
		return nil, nil
	}

	payload := unwrapEnvelope(data)
	slots, ok := extractSlotArrays(payload)
	if !ok {
		return nil, fmt.Errorf("unrecognized free slots payload")
	}

	out := make([]timeInterval, 0, len(slots))
	for _, raw := range slots {
		start, end, err := parseSlotInterval(raw)
		if err != nil {
			continue
		}
		if end.After(start) {
			out = append(out, timeInterval{start: start, end: end})
		}
	}
	return out, nil
}

func unwrapEnvelope(data json.RawMessage) json.RawMessage {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return data
	}
	for _, key := range []string{"response_data", "details", "data"} {
		if inner, ok := envelope[key]; ok && len(inner) > 0 {
			return unwrapEnvelope(inner)
		}
	}
	return data
}

func extractSlotArrays(payload json.RawMessage) ([]map[string]any, bool) {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, false
	}

	for _, key := range []string{"free_slots", "freeSlots", "free"} {
		if arr, ok := root[key].([]any); ok {
			return toSlotMaps(arr), true
		}
	}

	// Some responses nest slots under calendars.
	for _, key := range []string{"calendars", "calendar"} {
		if nested, ok := root[key].(map[string]any); ok {
			for _, calVal := range nested {
				if calMap, ok := calVal.(map[string]any); ok {
					for _, slotKey := range []string{"free_slots", "freeSlots", "free"} {
						if arr, ok := calMap[slotKey].([]any); ok {
							return toSlotMaps(arr), true
						}
					}
				}
			}
		}
	}

	if arr, ok := root["slots"].([]any); ok {
		return toSlotMaps(arr), true
	}

	return nil, false
}

func toSlotMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func parseSlotInterval(slot map[string]any) (time.Time, time.Time, error) {
	startRaw := firstString(slot, "start", "start_time", "startTime", "time_min", "timeMin")
	endRaw := firstString(slot, "end", "end_time", "endTime", "time_max", "timeMax")
	if startRaw == "" || endRaw == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("missing start or end")
	}
	start, err := parseFlexibleTime(startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseFlexibleTime(endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			switch s := v.(type) {
			case string:
				if strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			default:
				str := strings.TrimSpace(fmt.Sprint(v))
				if str != "" {
					return str
				}
			}
		}
	}
	return ""
}

func parseFlexibleTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", raw)
}
