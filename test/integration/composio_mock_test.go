//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
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
	sessionCounter int
	sessions       map[string]*mockComposioSession
	mcpScenario    string
}

type mockComposioSession struct {
	id         string
	metaCalls  int
	needsInput bool
}

func newMockComposio() *mockComposio {
	m := &mockComposio{sessions: map[string]*mockComposioSession{}}
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

func (m *mockComposio) SetMCPScenario(scenario string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpScenario = scenario
}

func (m *mockComposio) ResetMCPScenario() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpScenario = ""
	m.sessions = map[string]*mockComposioSession{}
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
	if strings.HasPrefix(r.URL.Path, "/api/v3.1/tool_router/session") {
		m.handleToolRouterSession(w, r)
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

func (m *mockComposio) handleToolRouterSession(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	fail := m.fail
	scenario := m.mcpScenario
	m.mu.Unlock()

	if fail {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream failure"}`))
		return
	}

	path := r.URL.Path
	switch {
	case path == "/api/v3.1/tool_router/session" && r.Method == http.MethodPost:
		m.mu.Lock()
		m.sessionCounter++
		sessionID := fmt.Sprintf("trs_mock_%d", m.sessionCounter)
		m.sessions[sessionID] = &mockComposioSession{id: sessionID}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"` + sessionID + `"}`))
		return
	case strings.HasSuffix(path, "/attach"):
		parts := strings.Split(path, "/")
		sessionID := parts[len(parts)-2]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"` + sessionID + `"}`))
		return
	case strings.HasSuffix(path, "/search"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"results":[]}`))
		return
	case strings.HasSuffix(path, "/execute_meta"):
		parts := strings.Split(path, "/")
		sessionID := parts[len(parts)-2]
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		slug, _ := req["slug"].(string)

		m.mu.Lock()
		sess := m.sessions[sessionID]
		if sess == nil {
			sess = &mockComposioSession{id: sessionID}
			m.sessions[sessionID] = sess
		}
		sess.metaCalls++
		metaCalls := sess.metaCalls
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch scenario {
		case "needs_input_first":
			if slug == "COMPOSIO_RUN_TASK" || metaCalls == 1 {
				_, _ = w.Write([]byte(`{"data":{"status":"needs_input","prompt":"Which repository should I use?"},"error":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"status":"completed","reply_text":"GitHub issue created successfully."},"error":null}`))
		case "needs_confirmation":
			_, _ = w.Write([]byte(`{"data":{"status":"needs_confirmation","prompt":"Create issue titled Bug in org/app?"},"error":null}`))
		case "needs_confirmation_then_success":
			if slug == "COMPOSIO_CONFIRM" {
				_, _ = w.Write([]byte(`{"data":{"status":"completed","reply_text":"GitHub issue created successfully."},"error":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"status":"needs_confirmation","prompt":"Create issue titled Bug in org/app?"},"error":null}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"status":"completed","reply_text":"GitHub issue created successfully."},"error":null}`))
		}
		return
	case strings.HasSuffix(path, "/execute"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"completed","reply_text":"done"},"error":null}`))
		return
	default:
		w.WriteHeader(http.StatusNotFound)
	}
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
