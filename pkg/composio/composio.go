// Package composio is a small, dependency-free client for executing Composio
// tools (actions) over HTTP. Composio brokers authenticated access to third
// party SaaS APIs (Trello, GitHub, Slack, ...) behind a single execute
// endpoint, so callers never handle the upstream credentials directly.
//
// The client is intentionally generic: Execute runs any tool by slug, and the
// Trello helper layered on top maps common board operations to their tool
// slugs. Point BaseURL at your Composio deployment and supply an API key plus
// the connected-account/user identifier that owns the upstream connection.
package composio

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

// DefaultBaseURL is the public Composio API base (v3 tools execute).
const DefaultBaseURL = "https://backend.composio.dev"

// DefaultBaseURLV31 is the public Composio API v3.1 base (sessions, connected accounts).
const DefaultBaseURLV31 = "https://backend.composio.dev/api/v3.1"

// Config configures a Client.
type Config struct {
	APIKey  string
	BaseURL string
	// UserID is the Composio user/entity identifier whose connected account is
	// used to execute tools. Optional when the deployment scopes by API key.
	UserID string
	// ConnectedAccountIDs is an ordered fallback list of connected accounts to
	// pin execution to. Execute tries each in order until one succeeds, which
	// lets a single toolkit span multiple connected accounts (e.g. several
	// GitHub identities). An empty list executes without pinning an account.
	ConnectedAccountIDs []string
	// HTTPClient is optional; a 30s-timeout client is used when nil.
	HTTPClient *http.Client
}

// Client executes Composio tools.
type Client struct {
	apiKey       string
	baseURL      string
	userID       string
	connectedIDs []string
	http         *http.Client
}

// New builds a Client from cfg.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		apiKey:       cfg.APIKey,
		baseURL:      base,
		userID:       cfg.UserID,
		connectedIDs: cfg.ConnectedAccountIDs,
		http:         hc,
	}
}

// WithAPIKey returns a shallow copy of the client using key for x-api-key auth.
func (c *Client) WithAPIKey(key string) *Client {
	clone := *c
	clone.apiKey = key
	return &clone
}

// ForAccounts returns a shallow copy of the client scoped to a specific ordered
// list of connected accounts. It lets per-toolkit helpers (Trello, GitHub, ...)
// target their own accounts without re-creating the underlying client.
func (c *Client) ForAccounts(ids ...string) *Client {
	clone := *c
	clone.connectedIDs = ids
	return &clone
}

// ForEntity returns a shallow copy scoped to a specific Composio entity
// (user_id) and an ordered list of connected accounts owned by that entity.
// Composio resolves a connected account within its owning entity, so when a
// toolkit's connections live under a different entity than the client default
// (e.g. Trello and GitHub connected under separate usernames), pin the matching
// user_id here. An empty userID leaves the client's default user_id unchanged.
func (c *Client) ForEntity(userID string, ids ...string) *Client {
	clone := *c
	if userID != "" {
		clone.userID = userID
	}
	clone.connectedIDs = ids
	return &clone
}

// ExecuteResult is the normalised outcome of a tool execution.
type ExecuteResult struct {
	Successful bool            `json:"successful"`
	Data       json.RawMessage `json:"data"`
	Error      string          `json:"error"`
}

type executeRequest struct {
	UserID             string         `json:"user_id,omitempty"`
	ConnectedAccountID string         `json:"connected_account_id,omitempty"`
	Arguments          map[string]any `json:"arguments"`
}

// Execute runs the tool identified by slug with the given arguments and decodes
// the normalised result. A non-2xx HTTP response, or a response whose
// "successful" flag is false, is returned as an error.
//
// When the client is scoped to multiple connected accounts, Execute tries each
// in order and returns the first success; if every account fails, the last
// error is returned. With no accounts configured it executes once without
// pinning a connected account.
//
// When every pinned account fails with ConnectedAccountNotFound and a Composio
// entity (user_id) is set, Execute falls back once to auto-resolve the active
// connected account for that entity. This keeps deployments working when a stale
// connected_account_id is still present in env after Composio rotates connections.
func (c *Client) Execute(ctx context.Context, slug string, arguments map[string]any) (*ExecuteResult, error) {
	if len(c.connectedIDs) == 0 {
		return c.executeOnce(ctx, "", slug, arguments)
	}
	var lastErr error
	sawNotFound := false
	for _, id := range c.connectedIDs {
		res, err := c.executeOnce(ctx, id, slug, arguments)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if isConnectedAccountNotFound(err) {
			sawNotFound = true
		}
	}
	if sawNotFound && c.userID != "" {
		if res, err := c.executeOnce(ctx, "", slug, arguments); err == nil {
			return res, nil
		}
	}
	return nil, lastErr
}

func isConnectedAccountNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "ConnectedAccountNotFound") ||
		strings.Contains(msg, "No connected account found")
}

// executeOnce performs a single tool execution against one connected account
// (empty means none). It is the building block Execute uses for its fallback.
func (c *Client) executeOnce(ctx context.Context, connectedAccountID, slug string, arguments map[string]any) (*ExecuteResult, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	body, err := json.Marshal(executeRequest{
		UserID:             c.userID,
		ConnectedAccountID: connectedAccountID,
		Arguments:          arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("composio: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v3/tools/execute/%s", c.baseURL, slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("composio: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("composio: execute %s: %w", slug, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("composio: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("composio: execute %s: status %d: %s", slug, resp.StatusCode, string(raw))
	}

	var result ExecuteResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("composio: decode response: %w", err)
	}
	if !result.Successful {
		msg := result.Error
		if msg == "" {
			msg = string(raw)
		}
		return nil, fmt.Errorf("composio: tool %s failed: %s", slug, msg)
	}
	return &result, nil
}
