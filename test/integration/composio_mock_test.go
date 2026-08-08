//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

const findFreeSlotsTool = "GOOGLECALENDAR_FIND_FREE_SLOTS"

// mockComposio records execute calls and can be toggled to return HTTP 500.
type mockComposio struct {
	mu      sync.Mutex
	server  *httptest.Server
	fail    bool
	lastReq map[string]any
}

func newMockComposio() *mockComposio {
	m := &mockComposio{}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockComposio) URL() string { return m.server.URL }

func (m *mockComposio) Close() { m.server.Close() }

func (m *mockComposio) SetFail(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fail = v
}

func (m *mockComposio) LastRequest() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastReq == nil {
		return nil
	}
	out := make(map[string]any, len(m.lastReq))
	for k, v := range m.lastReq {
		out[k] = v
	}
	return out
}

func (m *mockComposio) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(body, &req)

	m.mu.Lock()
	m.lastReq = req
	fail := m.fail
	m.mu.Unlock()

	if fail {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream failure"}`))
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/api/v3/tools/execute/")
	if slug == findFreeSlotsTool {
		m.writeFindFreeSlotsResponse(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
}

func (m *mockComposio) writeFindFreeSlotsResponse(w http.ResponseWriter) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	now := time.Now().In(loc)
	var friday time.Time
	for d := 0; d < 14; d++ {
		candidate := now.AddDate(0, 0, d)
		if candidate.Weekday() == time.Friday {
			friday = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 14, 0, 0, 0, loc)
			break
		}
	}
	if friday.IsZero() {
		friday = now.Add(24 * time.Hour)
	}
	end := friday.Add(time.Hour)

	payload, _ := json.Marshal(map[string]any{
		"free_slots": []map[string]string{
			{
				"start": friday.Format(time.RFC3339),
				"end":   end.Format(time.RFC3339),
			},
		},
	})
	resp, _ := json.Marshal(map[string]any{
		"successful": true,
		"data":       json.RawMessage(payload),
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}
