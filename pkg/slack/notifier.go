package slack

import "context"

// Notifier binds a Client to a single webhook URL for dependency injection.
type Notifier struct {
	client     *Client
	webhookURL string
}

// NewNotifier returns a notifier that posts to webhookURL via client.
func NewNotifier(client *Client, webhookURL string) *Notifier {
	return &Notifier{client: client, webhookURL: webhookURL}
}

// Enabled reports whether a webhook URL is configured.
func (n *Notifier) Enabled() bool {
	return n.webhookURL != ""
}

// Notify sends text to the bound webhook. An empty URL is a no-op.
func (n *Notifier) Notify(ctx context.Context, text string) error {
	return n.client.PostText(ctx, n.webhookURL, text)
}

// NotifyMessage sends a richer payload to the bound webhook.
func (n *Notifier) NotifyMessage(ctx context.Context, msg Message) error {
	return n.client.PostMessage(ctx, n.webhookURL, msg)
}
