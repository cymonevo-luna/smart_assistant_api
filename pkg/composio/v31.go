package composio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ConnectedAccount is a normalized connected-account record from Composio v3.1.
type ConnectedAccount struct {
	ID          string `json:"id"`
	ToolkitSlug string `json:"toolkit_slug"`
	Status      string `json:"status"`
	Alias       string `json:"alias,omitempty"`
	WordID      string `json:"word_id,omitempty"`
}

// ListConnectedAccountsOpts filters the connected-accounts listing.
type ListConnectedAccountsOpts struct {
	// Statuses filters by account status (e.g. "ACTIVE").
	Statuses []string
	Limit    int
}

// SessionCreateOpts configures a new tool-router session.
type SessionCreateOpts struct {
	ManageConnections struct {
		Enable bool `json:"enable,omitempty"`
	} `json:"manage_connections,omitempty"`
	// ConnectedAccounts pins toolkit → account ID lists for the session.
	ConnectedAccounts map[string][]string `json:"connected_accounts,omitempty"`
	// MCP requests MCP server details in the response when supported.
	MCP bool `json:"mcp,omitempty"`
}

// SessionMCP holds MCP endpoint details returned by session create/attach.
type SessionMCP struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Session is the normalized tool-router session payload.
type Session struct {
	SessionID string      `json:"session_id"`
	MCP       *SessionMCP `json:"mcp,omitempty"`
}

// SessionExecuteResult is the outcome of a session tool or meta execution.
type SessionExecuteResult struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error,omitempty"`
	LogID string          `json:"log_id,omitempty"`
}

// SearchToolsResult is the outcome of a session tool search.
type SearchToolsResult struct {
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Raw     json.RawMessage `json:"-"`
}

func (c *Client) v31URL(path string) string {
	base := strings.TrimRight(c.baseURL, "/")
	if base == "" {
		base = strings.TrimRight(DefaultBaseURL, "/")
	}
	return base + "/api/v3.1" + path
}

func (c *Client) doV31(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("composio: marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.v31URL(path), reader)
	if err != nil {
		return fmt.Errorf("composio: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("composio: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("composio: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("composio: %s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("composio: decode response: %w", err)
		}
	}
	return nil
}

type connectedAccountsPage struct {
	Items []connectedAccountItem `json:"items"`
	Next  *string                `json:"next_cursor"`
}

type connectedAccountItem struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Alias   string `json:"alias"`
	WordID  string `json:"word_id"`
	Toolkit struct {
		Slug string `json:"slug"`
	} `json:"toolkit"`
}

func normalizeConnectedAccount(item connectedAccountItem) ConnectedAccount {
	return ConnectedAccount{
		ID:          item.ID,
		ToolkitSlug: item.Toolkit.Slug,
		Status:      item.Status,
		Alias:       item.Alias,
		WordID:      item.WordID,
	}
}

func (c *Client) listConnectedAccountsPage(ctx context.Context, statuses []string, limit int, cursor string) ([]ConnectedAccount, string, error) {
	q := url.Values{}
	for _, s := range statuses {
		q.Add("statuses", s)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}

	path := "/connected_accounts"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	var page connectedAccountsPage
	if err := c.doV31(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, "", err
	}

	out := make([]ConnectedAccount, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, normalizeConnectedAccount(item))
	}
	next := ""
	if page.Next != nil {
		next = *page.Next
	}
	return out, next, nil
}

// ListConnectedAccounts returns all connected accounts matching opts, paginating
// through next_cursor until exhausted.
func (c *Client) ListConnectedAccounts(ctx context.Context, opts ListConnectedAccountsOpts) ([]ConnectedAccount, error) {
	var all []ConnectedAccount
	cursor := ""
	for {
		page, next, err := c.listConnectedAccountsPage(ctx, opts.Statuses, opts.Limit, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		cursor = next
	}
	return all, nil
}

// ErrUnauthorized is returned by ValidateAPIKey when the API key is invalid.
var ErrUnauthorized = errors.New("composio: unauthorized")

// ValidateAPIKey performs a lightweight connected-accounts list (limit 1) to
// verify the client's API key.
func (c *Client) ValidateAPIKey(ctx context.Context) error {
	_, _, err := c.listConnectedAccountsPage(ctx, nil, 1, "")
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "status 401") {
		return fmt.Errorf("%w: %s", ErrUnauthorized, err)
	}
	return err
}

type sessionCreateRequest struct {
	UserID            string              `json:"user_id"`
	ManageConnections *manageConnections  `json:"manage_connections,omitempty"`
	ConnectedAccounts map[string][]string `json:"connected_accounts,omitempty"`
	MCP               *bool               `json:"mcp,omitempty"`
}

type manageConnections struct {
	Enable bool `json:"enable"`
}

type sessionResponse struct {
	SessionID string `json:"session_id"`
	MCP       *struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"mcp"`
}

func parseSession(resp sessionResponse) *Session {
	s := &Session{SessionID: resp.SessionID}
	if resp.MCP != nil {
		s.MCP = &SessionMCP{
			URL:     resp.MCP.URL,
			Headers: resp.MCP.Headers,
		}
	}
	return s
}

// CreateSession starts a new tool-router session for userID.
func (c *Client) CreateSession(ctx context.Context, userID string, opts SessionCreateOpts) (*Session, error) {
	req := sessionCreateRequest{UserID: userID}
	if opts.ManageConnections.Enable {
		req.ManageConnections = &manageConnections{Enable: true}
	}
	if len(opts.ConnectedAccounts) > 0 {
		req.ConnectedAccounts = opts.ConnectedAccounts
	}
	if opts.MCP {
		mcp := true
		req.MCP = &mcp
	}

	var resp sessionResponse
	if err := c.doV31(ctx, http.MethodPost, "/tool_router/session", req, &resp); err != nil {
		return nil, err
	}
	return parseSession(resp), nil
}

// AttachSession re-attaches to an existing tool-router session.
func (c *Client) AttachSession(ctx context.Context, sessionID string) (*Session, error) {
	path := fmt.Sprintf("/tool_router/session/%s/attach", url.PathEscape(sessionID))
	var resp sessionResponse
	if err := c.doV31(ctx, http.MethodPost, path, map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return parseSession(resp), nil
}

type sessionExecuteRequest struct {
	ToolSlug  string         `json:"tool_slug,omitempty"`
	Slug      string         `json:"slug,omitempty"`
	Arguments map[string]any `json:"arguments"`
}

// ExecuteTool runs an app or meta tool within a session.
func (c *Client) ExecuteTool(ctx context.Context, sessionID, toolSlug string, args map[string]any) (*SessionExecuteResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	path := fmt.Sprintf("/tool_router/session/%s/execute", url.PathEscape(sessionID))
	req := sessionExecuteRequest{ToolSlug: toolSlug, Arguments: args}
	return c.executeSession(ctx, path, req)
}

// ExecuteMeta runs a meta tool within a session.
func (c *Client) ExecuteMeta(ctx context.Context, sessionID, metaSlug string, args map[string]any) (*SessionExecuteResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	path := fmt.Sprintf("/tool_router/session/%s/execute_meta", url.PathEscape(sessionID))
	req := sessionExecuteRequest{Slug: metaSlug, Arguments: args}
	return c.executeSession(ctx, path, req)
}

func (c *Client) executeSession(ctx context.Context, path string, req sessionExecuteRequest) (*SessionExecuteResult, error) {
	var result SessionExecuteResult
	if err := c.doV31(ctx, http.MethodPost, path, req, &result); err != nil {
		return nil, err
	}
	if result.Error != "" {
		return &result, fmt.Errorf("composio: session execute: %s", result.Error)
	}
	return &result, nil
}

type searchToolsRequest struct {
	Queries []searchQuery `json:"queries"`
}

type searchQuery struct {
	UseCase string `json:"use_case"`
}

// SearchTools searches for tools matching query within a session.
func (c *Client) SearchTools(ctx context.Context, sessionID, query string) (*SearchToolsResult, error) {
	path := fmt.Sprintf("/tool_router/session/%s/search", url.PathEscape(sessionID))
	req := searchToolsRequest{
		Queries: []searchQuery{{UseCase: query}},
	}

	var raw json.RawMessage
	if err := c.doV31(ctx, http.MethodPost, path, req, &raw); err != nil {
		return nil, err
	}
	var result SearchToolsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("composio: decode search response: %w", err)
	}
	result.Raw = raw
	return &result, nil
}
