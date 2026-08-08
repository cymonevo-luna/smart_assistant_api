package llm

import (
	"context"
	"strings"
	"sync"
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
