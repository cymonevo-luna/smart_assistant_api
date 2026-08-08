package availability

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/composio"
)

func TestRecommendSlot_RecommendsWeekdayJKTSlot(t *testing.T) {
	loc := mustLoadLocation(t, MeetingTimezone)
	friday := nextWeekdayAt(loc, time.Friday, 14, 0)
	end := friday.Add(time.Hour)

	svc := newTestService(t, mockFreeSlotsResponse(t, []slotSpec{
		{start: friday, end: end},
	}), time.Date(2026, 8, 7, 10, 0, 0, 0, loc))

	got, err := svc.RecommendSlot(context.Background(), "user-1", RecommendSlotInput{
		AttendeeEmails: []string{"invitee@example.com"},
	})
	if err != nil {
		t.Fatalf("RecommendSlot: %v", err)
	}
	if !got.Equal(friday) {
		t.Fatalf("got %v, want %v", got, friday)
	}
}

func TestRecommendSlot_SkipsWeekendSlots(t *testing.T) {
	loc := mustLoadLocation(t, MeetingTimezone)
	saturday := nextWeekdayAt(loc, time.Saturday, 10, 0)
	monday := nextWeekdayAt(loc, time.Monday, 11, 0)

	svc := newTestService(t, mockFreeSlotsResponse(t, []slotSpec{
		{start: saturday, end: saturday.Add(2 * time.Hour)},
		{start: monday, end: monday.Add(time.Hour)},
	}), time.Date(2026, 8, 7, 10, 0, 0, 0, loc))

	got, err := svc.RecommendSlot(context.Background(), "user-1", RecommendSlotInput{})
	if err != nil {
		t.Fatalf("RecommendSlot: %v", err)
	}
	if got.Weekday() == time.Saturday || got.Weekday() == time.Sunday {
		t.Fatalf("expected weekday slot, got %v", got)
	}
	if !got.Equal(monday) {
		t.Fatalf("got %v, want %v", got, monday)
	}
}

func TestRecommendSlot_EnforcesWorkHourStartWindow(t *testing.T) {
	loc := mustLoadLocation(t, MeetingTimezone)
	day := nextWeekdayAt(loc, time.Wednesday, 0, 0)
	early := time.Date(day.Year(), day.Month(), day.Day(), 8, 30, 0, 0, loc)
	late := time.Date(day.Year(), day.Month(), day.Day(), 17, 30, 0, 0, loc)
	valid := time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, loc)

	svc := newTestService(t, mockFreeSlotsResponse(t, []slotSpec{
		{start: early, end: early.Add(time.Hour)},
		{start: late, end: late.Add(time.Hour)},
		{start: valid, end: valid.Add(time.Hour)},
	}), time.Date(2026, 8, 7, 10, 0, 0, 0, loc))

	got, err := svc.RecommendSlot(context.Background(), "user-1", RecommendSlotInput{})
	if err != nil {
		t.Fatalf("RecommendSlot: %v", err)
	}
	if !got.Equal(valid) {
		t.Fatalf("got %v, want %v", got, valid)
	}
}

func TestRecommendSlot_NoSlotReturnsErrNoAvailableSlot(t *testing.T) {
	loc := mustLoadLocation(t, MeetingTimezone)
	saturday := nextWeekdayAt(loc, time.Saturday, 10, 0)
	sunday := nextWeekdayAt(loc, time.Sunday, 10, 0)
	weekday := nextWeekdayAt(loc, time.Tuesday, 0, 0)
	early := time.Date(weekday.Year(), weekday.Month(), weekday.Day(), 7, 0, 0, 0, loc)
	late := time.Date(weekday.Year(), weekday.Month(), weekday.Day(), 18, 0, 0, 0, loc)

	svc := newTestService(t, mockFreeSlotsResponse(t, []slotSpec{
		{start: saturday, end: saturday.Add(2 * time.Hour)},
		{start: sunday, end: sunday.Add(2 * time.Hour)},
		{start: early, end: early.Add(time.Hour)},
		{start: late, end: late.Add(time.Hour)},
	}), time.Date(2026, 8, 7, 10, 0, 0, 0, loc))

	_, err := svc.RecommendSlot(context.Background(), "user-1", RecommendSlotInput{})
	if !errors.Is(err, ErrNoAvailableSlot) {
		t.Fatalf("expected ErrNoAvailableSlot, got %v", err)
	}
}

func TestRecommendSlot_PropagatesComposioFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"successful":false,"error":"upstream failure"}`))
	}))
	t.Cleanup(srv.Close)

	client := composio.New(composio.Config{APIKey: "key", BaseURL: srv.URL})
	svc := NewService(client)

	_, err := svc.RecommendSlot(context.Background(), "user-1", RecommendSlotInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNoAvailableSlot) {
		t.Fatal("expected composio error, not ErrNoAvailableSlot")
	}
}

type slotSpec struct {
	start time.Time
	end   time.Time
}

func newTestService(t *testing.T, responseBody string, now time.Time) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		args, _ := req["arguments"].(map[string]any)
		if args == nil {
			t.Errorf("missing arguments in composio request")
		}
		items, _ := args["items"].([]any)
		if len(items) == 0 || items[0] != "primary" {
			t.Errorf("expected primary calendar in items, got %v", items)
		}
		if args["timezone"] != MeetingTimezone {
			t.Errorf("timezone = %v, want %s", args["timezone"], MeetingTimezone)
		}
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)

	client := composio.New(composio.Config{APIKey: "key", BaseURL: srv.URL})
	svc := NewService(client)
	svc.now = func() time.Time { return now }
	return svc
}

func mockFreeSlotsResponse(t *testing.T, slots []slotSpec) string {
	t.Helper()
	type slotJSON struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	payload := make([]slotJSON, len(slots))
	for i, s := range slots {
		payload[i] = slotJSON{
			Start: s.start.Format(time.RFC3339),
			End:   s.end.Format(time.RFC3339),
		}
	}
	data, err := json.Marshal(map[string]any{"free_slots": payload})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	envelope, err := json.Marshal(map[string]any{
		"successful": true,
		"data":       json.RawMessage(data),
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(envelope)
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%s): %v", name, err)
	}
	return loc
}

func nextWeekdayAt(loc *time.Location, weekday time.Weekday, hour, minute int) time.Time {
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, loc)
	for d := 0; d < 14; d++ {
		candidate := now.AddDate(0, 0, d)
		if candidate.Weekday() == weekday {
			return time.Date(candidate.Year(), candidate.Month(), candidate.Day(), hour, minute, 0, 0, loc)
		}
	}
	panic("no matching weekday in horizon")
}
