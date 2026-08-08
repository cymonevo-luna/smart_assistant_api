//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

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

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"successful":true,"data":{}}`))
}
