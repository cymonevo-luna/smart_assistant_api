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

const (
	findFreeSlotsTool         = "GOOGLECALENDAR_FIND_FREE_SLOTS"
	createEventTool           = "GOOGLECALENDAR_CREATE_EVENT"
	mockValidComposioAPIKey   = "mock-valid-composio-key"
	mockInvalidComposioAPIKey = "invalid-composio-key"
)

// mockComposio records execute calls and can be toggled to return HTTP 500.
type mockComposio struct {
	mu             sync.Mutex
	server         *httptest.Server
	fail           bool
	emptyFreeSlots bool
	lastReq        map[string]any
	requests       []map[string]any
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

func (m *mockComposio) SetEmptyFreeSlots(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emptyFreeSlots = v
}

func (m *mockComposio) ResetRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
	m.lastReq = nil
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

func (m *mockComposio) ToolCallCount(tool string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, req := range m.requests {
		if slug, _ := req["slug"].(string); slug == tool {
			count++
		}
	}
	return count
}

func (m *mockComposio) handle(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v3.1/connected_accounts") {
		m.handleConnectedAccounts(w, r)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(body, &req)

	slug := strings.TrimPrefix(r.URL.Path, "/api/v3/tools/execute/")
	req["slug"] = slug

	m.mu.Lock()
	m.lastReq = req
	m.requests = append(m.requests, req)
	fail := m.fail
	emptyFreeSlots := m.emptyFreeSlots
	m.mu.Unlock()

	if fail {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream failure"}`))
		return
	}

	if slug == findFreeSlotsTool {
		if emptyFreeSlots {
			m.writeEmptyFreeSlotsResponse(w)
			return
		}
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

func (m *mockComposio) handleConnectedAccounts(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("x-api-key")
	if apiKey == mockInvalidComposioAPIKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","status":401}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{
		"items": [
			{"id":"ca_github","status":"ACTIVE","toolkit":{"slug":"github"},"alias":"work"},
			{"id":"ca_gmail","status":"ACTIVE","toolkit":{"slug":"gmail"}}
		],
		"next_cursor": null
	}`))
}

func (m *mockComposio) writeEmptyFreeSlotsResponse(w http.ResponseWriter) {
	payload, _ := json.Marshal(map[string]any{
		"free_slots": []map[string]string{},
	})
	resp, _ := json.Marshal(map[string]any{
		"successful": true,
		"data":       json.RawMessage(payload),
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}
