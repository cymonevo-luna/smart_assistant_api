// Package slack is a small client for posting messages to Slack via Incoming
// Webhooks. Callers supply the webhook URL per use case; empty URLs are treated
// as a no-op so unconfigured deployments start safely.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebhookPrefix is the required URL prefix for Slack Incoming Webhooks.
const WebhookPrefix = "https://hooks.slack.com/services/"

const slackSuccessBody = "ok"

// Config configures a Client.
type Config struct {
	HTTPClient *http.Client
}

// Client posts messages to Slack Incoming Webhooks.
type Client struct {
	http *http.Client
}

// New builds a Client from cfg.
func New(cfg Config) *Client {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{http: hc}
}

// Message is the JSON body for an Incoming Webhook POST. Text is required unless
// Blocks are set.
type Message struct {
	Text      string            `json:"text,omitempty"`
	Blocks    []json.RawMessage `json:"blocks,omitempty"`
	Username  string            `json:"username,omitempty"`
	IconEmoji string            `json:"icon_emoji,omitempty"`
	IconURL   string            `json:"icon_url,omitempty"`
	Channel   string            `json:"channel,omitempty"`
}

// PostMessage sends msg to webhookURL. An empty webhookURL is a no-op.
func (c *Client) PostMessage(ctx context.Context, webhookURL string, msg Message) error {
	if webhookURL == "" {
		return nil
	}
	if !strings.HasPrefix(webhookURL, WebhookPrefix) {
		return fmt.Errorf("slack: invalid webhook URL %s", RedactWebhookURL(webhookURL))
	}
	if msg.Text == "" && len(msg.Blocks) == 0 {
		return fmt.Errorf("slack: message must have text or blocks")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("slack: marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack: post to %s: %w", RedactWebhookURL(webhookURL), err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("slack: read response from %s: %w", RedactWebhookURL(webhookURL), err)
	}
	responseBody := strings.TrimSpace(string(raw))

	if resp.StatusCode != http.StatusOK || responseBody != slackSuccessBody {
		return fmt.Errorf("slack: post to %s: status %d: %s", RedactWebhookURL(webhookURL), resp.StatusCode, responseBody)
	}
	return nil
}

// PostText sends a plain-text message to webhookURL.
func (c *Client) PostText(ctx context.Context, webhookURL string, text string) error {
	return c.PostMessage(ctx, webhookURL, Message{Text: text})
}

// RedactWebhookURL returns a safe suffix for logs and errors (never the full secret URL).
func RedactWebhookURL(webhookURL string) string {
	if webhookURL == "" {
		return ""
	}
	const suffixLen = 8
	if len(webhookURL) <= suffixLen {
		return "…"
	}
	return "…" + webhookURL[len(webhookURL)-suffixLen:]
}
