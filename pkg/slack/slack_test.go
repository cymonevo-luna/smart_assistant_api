package slack_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/pkg/slack"
)

func testHTTPClient(srv *httptest.Server) *http.Client {
	return &http.Client{
		Transport: &rewriteHostTransport{
			base:    http.DefaultTransport,
			rewrite: srv.URL,
		},
	}
}

type rewriteHostTransport struct {
	base    http.RoundTripper
	rewrite string
}

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(t.rewrite)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return t.base.RoundTrip(req)
}

func TestPostMessage_Success(t *testing.T) {
	t.Parallel()

	var gotBody slack.Message
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content-type %q, want application/json", ct)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	webhookURL := slack.WebhookPrefix + "T/B/secret"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})

	err := client.PostMessage(context.Background(), webhookURL, slack.Message{
		Text:      "Hello from Luna",
		Username:  "luna",
		IconEmoji: ":robot_face:",
	})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if gotBody.Text != "Hello from Luna" {
		t.Fatalf("text %q, want Hello from Luna", gotBody.Text)
	}
	if gotBody.Username != "luna" {
		t.Fatalf("username %q, want luna", gotBody.Username)
	}
}

func TestPostText_Success(t *testing.T) {
	t.Parallel()

	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg slack.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Fatal(err)
		}
		gotText = msg.Text
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	webhookURL := slack.WebhookPrefix + "T/B/secret"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})

	if err := client.PostText(context.Background(), webhookURL, "ping"); err != nil {
		t.Fatalf("PostText: %v", err)
	}
	if gotText != "ping" {
		t.Fatalf("text %q, want ping", gotText)
	}
}

func TestPostMessage_EmptyWebhookURL(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})
	if err := client.PostMessage(context.Background(), "", slack.Message{Text: "noop"}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if called {
		t.Fatal("expected no HTTP call for empty webhook URL")
	}
}

func TestPostMessage_InvalidWebhookPrefix(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	webhookURL := "https://example.com/hook"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})
	err := client.PostMessage(context.Background(), webhookURL, slack.Message{Text: "hi"})
	if err == nil {
		t.Fatal("expected error for invalid webhook prefix")
	}
	if !strings.Contains(err.Error(), "invalid webhook URL") {
		t.Fatalf("error %q, want invalid webhook URL mention", err)
	}
	if strings.Contains(err.Error(), webhookURL) {
		t.Fatalf("error %q must not contain full webhook URL", err)
	}
	if called {
		t.Fatal("expected no HTTP call for invalid webhook prefix")
	}
}

func TestPostMessage_MissingTextAndBlocks(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	webhookURL := slack.WebhookPrefix + "T/B/secret"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})
	err := client.PostMessage(context.Background(), webhookURL, slack.Message{})
	if err == nil {
		t.Fatal("expected error for empty message")
	}
	if !strings.Contains(err.Error(), "text or blocks") {
		t.Fatalf("error %q, want text or blocks mention", err)
	}
	if called {
		t.Fatal("expected no HTTP call for empty message")
	}
}

func TestPostMessage_BlocksOnly(t *testing.T) {
	t.Parallel()

	var gotBlocks int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg slack.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Fatal(err)
		}
		gotBlocks = len(msg.Blocks)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	webhookURL := slack.WebhookPrefix + "T/B/secret"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})

	err := client.PostMessage(context.Background(), webhookURL, slack.Message{
		Blocks: []json.RawMessage{json.RawMessage(`{"type":"section"}`)},
	})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if gotBlocks != 1 {
		t.Fatalf("blocks %d, want 1", gotBlocks)
	}
}

func TestPostMessage_NonOKResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_payload"))
	}))
	defer srv.Close()

	webhookURL := slack.WebhookPrefix + "T/B/top-secret-token"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})

	err := client.PostMessage(context.Background(), webhookURL, slack.Message{Text: "hi"})
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("error %q, want status 400 mention", err)
	}
	if !strings.Contains(err.Error(), "invalid_payload") {
		t.Fatalf("error %q, want response body mention", err)
	}
	if strings.Contains(err.Error(), "top-secret-token") {
		t.Fatalf("error %q must not contain webhook secret", err)
	}
}

func TestPostMessage_OKWithWrongBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	webhookURL := slack.WebhookPrefix + "T/B/secret"
	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})

	err := client.PostMessage(context.Background(), webhookURL, slack.Message{Text: "hi"})
	if err == nil {
		t.Fatal("expected error when body is not ok")
	}
}

func TestRedactWebhookURL(t *testing.T) {
	t.Parallel()

	full := slack.WebhookPrefix + "T000/B000/XXXXXXXXXXXXXXXX"
	redacted := slack.RedactWebhookURL(full)
	if strings.Contains(redacted, "T000") {
		t.Fatalf("redacted %q must not contain webhook prefix segments", redacted)
	}
	if !strings.HasPrefix(redacted, "…") {
		t.Fatalf("redacted %q, want ellipsis prefix", redacted)
	}
}

func TestNotifier_EnabledAndNotify(t *testing.T) {
	t.Parallel()

	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg slack.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Fatal(err)
		}
		gotText = msg.Text
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})
	webhookURL := slack.WebhookPrefix + "T/B/secret"
	n := slack.NewNotifier(client, webhookURL)

	if !n.Enabled() {
		t.Fatal("expected notifier to be enabled")
	}
	if err := n.Notify(context.Background(), "alert"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotText != "alert" {
		t.Fatalf("text %q, want alert", gotText)
	}
}

func TestNotifier_Disabled(t *testing.T) {
	t.Parallel()

	client := slack.New(slack.Config{})
	n := slack.NewNotifier(client, "")
	if n.Enabled() {
		t.Fatal("expected notifier to be disabled")
	}
	if err := n.Notify(context.Background(), "ignored"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestNotifier_NotifyMessage(t *testing.T) {
	t.Parallel()

	var gotUsername string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg slack.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Fatal(err)
		}
		gotUsername = msg.Username
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := slack.New(slack.Config{HTTPClient: testHTTPClient(srv)})
	webhookURL := slack.WebhookPrefix + "T/B/secret"
	n := slack.NewNotifier(client, webhookURL)

	err := n.NotifyMessage(context.Background(), slack.Message{
		Text:     "rich alert",
		Username: "luna-bot",
	})
	if err != nil {
		t.Fatalf("NotifyMessage: %v", err)
	}
	if gotUsername != "luna-bot" {
		t.Fatalf("username %q, want luna-bot", gotUsername)
	}
}
