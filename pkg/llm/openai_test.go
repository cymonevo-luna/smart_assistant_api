package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

func TestOpenAIClassifierMissingAPIKey(t *testing.T) {
	c := &OpenAIClassifier{}
	_, err := c.Classify(context.Background(), ClassifyRequest{Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "api key not configured") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestOpenAIClassifierSuccessfulMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Fatalf("unexpected auth header %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		var req openAIChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "gpt-4o-mini" {
			t.Fatalf("model = %q, want gpt-4o-mini", req.Model)
		}
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"matched\":true,\"plugin_slug\":\"google-calendar-meet\",\"arguments\":{\"attendee_name\":\"Janet\",\"attendee_email\":\"janet@example.com\",\"start_time\":\"2026-08-09T14:00:00Z\"}}"}}]
		}`))
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		Model:      "gpt-4o-mini",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	result, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "Schedule a meeting with Janet at 2 PM tomorrow",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Name:     "Google Calendar Meet",
			Triggers: []string{"schedule a meeting"},
			Arguments: []plugin.ManifestArgument{
				{Name: "attendee_name", Type: "string"},
				{Name: "attendee_email", Type: "email"},
				{Name: "start_time", Type: "datetime"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "google-calendar-meet" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Arguments["attendee_name"] != "Janet" {
		t.Fatalf("attendee_name = %v", result.Arguments["attendee_name"])
	}
	if result.Arguments["attendee_email"] != "janet@example.com" {
		t.Fatalf("attendee_email = %v", result.Arguments["attendee_email"])
	}
	if result.Arguments["start_time"] != "2026-08-09T14:00:00Z" {
		t.Fatalf("start_time = %v", result.Arguments["start_time"])
	}
}

func TestOpenAIClassifierNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"matched\":false}"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	result, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "what is the weather",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule a meeting"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched {
		t.Fatalf("expected no match, got %+v", result)
	}
}

func TestOpenAIClassifierMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	_, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "schedule a meeting",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule a meeting"},
		}},
	})
	if err == nil {
		t.Fatal("expected error for malformed classifier json")
	}
}

func TestOpenAIClassifierHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	_, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "schedule a meeting",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule a meeting"},
		}},
	})
	if err == nil {
		t.Fatal("expected error for http 500")
	}
}

func TestOpenAIClassifierTriggerFallbackOnNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"matched\":false}"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	result, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "schedule a meeting with Bob",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Name:     "Google Calendar Meet",
			Triggers: []string{"schedule a meeting", "schedule meeting"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "google-calendar-meet" {
		t.Fatalf("expected trigger fallback match, got %+v", result)
	}
}

func TestOpenAIClassifierDropsUnknownArgumentKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"matched\":true,\"plugin_slug\":\"google-calendar-meet\",\"arguments\":{\"attendee_name\":\"Bob\",\"unknown\":\"x\"}}"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	result, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "schedule a meeting with Bob",
		Plugins: []PluginCandidate{{
			Slug: "google-calendar-meet",
			Arguments: []plugin.ManifestArgument{
				{Name: "attendee_name", Type: "string"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Arguments["unknown"] != nil {
		t.Fatalf("expected unknown key dropped, got %+v", result.Arguments)
	}
	if result.Arguments["attendee_name"] != "Bob" {
		t.Fatalf("attendee_name = %v", result.Arguments["attendee_name"])
	}
}

func TestOpenAIClassifierRejectsInvalidPluginSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"matched\":true,\"plugin_slug\":\"unknown-plugin\"}"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClassifier{
		APIKey:     "test-key",
		BaseURL:    srv.URL + "/v1",
		HTTPClient: srv.Client(),
	}

	result, err := c.Classify(context.Background(), ClassifyRequest{
		Text: "schedule a meeting with Bob",
		Plugins: []PluginCandidate{{
			Slug:     "google-calendar-meet",
			Triggers: []string{"schedule a meeting"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.PluginSlug != "google-calendar-meet" {
		t.Fatalf("expected trigger fallback after invalid slug, got %+v", result)
	}
}
