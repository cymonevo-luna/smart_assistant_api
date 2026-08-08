// Package cursoragent is a small, dependency-free client for the Cursor Cloud
// (background) Agents REST API. It launches headless coding agents against a Git
// repository, optionally opening a pull request automatically, and reports run
// status so an orchestrator can drive a task to completion.
//
// It targets the public API at https://api.cursor.com/v0/agents and authenticates
// with a Cursor API key (user key or team service-account key).
package cursoragent

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

// DefaultBaseURL is the public Cursor API base.
const DefaultBaseURL = "https://api.cursor.com"

// Status values reported by the API.
const (
	StatusCreating = "CREATING"
	StatusPending  = "PENDING"
	StatusRunning  = "RUNNING"
	StatusFinished = "FINISHED"
	StatusError    = "ERROR"
	StatusExpired  = "EXPIRED"
)

// Config configures a Client.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client talks to the Cursor Cloud Agents API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
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
	return &Client{apiKey: cfg.APIKey, baseURL: base, http: hc}
}

// MCPServer is an inline MCP server configuration passed to a launched agent so
// it can use external tools (for example Composio) during the run.
type MCPServer struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// LaunchInput describes a coding run to start.
type LaunchInput struct {
	// Prompt is the natural-language instruction for the agent.
	Prompt string
	// Repository is the Git URL the agent clones (for example
	// https://github.com/org/repo).
	Repository string
	// Ref is the base branch/ref. Empty uses the repository default.
	Ref string
	// Model is the model id. Empty lets the server pick.
	Model string
	// AutoCreatePR opens a pull request when the run finishes.
	AutoCreatePR bool
	// BranchName optionally names the working branch.
	BranchName string
	// MCPServers are inline MCP servers available to the agent during the run.
	MCPServers []MCPServer
}

// Agent is the state of a launched run.
type Agent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Source    Source `json:"source"`
	Target    Target `json:"target"`
	Summary   string `json:"summary"`
	Error     string `json:"error"`
	CreatedAt string `json:"createdAt"`
}

// Source is the repository the agent runs against.
type Source struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
}

// Target carries the run outputs: working branch, agent URL and PR URL.
type Target struct {
	BranchName string `json:"branchName"`
	URL        string `json:"url"`
	PRURL      string `json:"prUrl"`
}

// IsTerminal reports whether status is a final state.
func IsTerminal(status string) bool {
	switch status {
	case StatusFinished, StatusError, StatusExpired:
		return true
	default:
		return false
	}
}

type launchRequest struct {
	Prompt prompt      `json:"prompt"`
	Source source      `json:"source"`
	Model  string      `json:"model,omitempty"`
	Target *target     `json:"target,omitempty"`
	MCP    []MCPServer `json:"mcpServers,omitempty"`
}

type prompt struct {
	Text string `json:"text"`
}

type source struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref,omitempty"`
}

type target struct {
	AutoCreatePR bool   `json:"autoCreatePr"`
	BranchName   string `json:"branchName,omitempty"`
}

// Launch starts a new agent run and returns its initial state.
func (c *Client) Launch(ctx context.Context, in LaunchInput) (*Agent, error) {
	reqBody := launchRequest{
		Prompt: prompt{Text: in.Prompt},
		Source: source{Repository: in.Repository, Ref: in.Ref},
		Model:  in.Model,
		MCP:    in.MCPServers,
	}
	if in.AutoCreatePR || in.BranchName != "" {
		reqBody.Target = &target{AutoCreatePR: in.AutoCreatePR, BranchName: in.BranchName}
	}

	var agent Agent
	if err := c.do(ctx, http.MethodPost, "/v0/agents", reqBody, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// Get returns the current state of a run by id.
func (c *Client) Get(ctx context.Context, id string) (*Agent, error) {
	var agent Agent
	if err := c.do(ctx, http.MethodGet, "/v0/agents/"+id, nil, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// Message is a single conversation message produced during a run.
type Message struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type conversationResponse struct {
	Messages []Message `json:"messages"`
}

// Conversation returns the messages exchanged during a run.
func (c *Client) Conversation(ctx context.Context, id string) ([]Message, error) {
	var resp conversationResponse
	if err := c.do(ctx, http.MethodGet, "/v0/agents/"+id+"/conversation", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

// ResultText returns the concatenated assistant output for a run. It is the
// convenient way to read a run's final textual result (for example a JSON block
// the agent was instructed to emit).
func (c *Client) ResultText(ctx context.Context, id string) (string, error) {
	msgs, err := c.Conversation(ctx, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, m := range msgs {
		if isAssistantMessage(m.Type) {
			b.WriteString(m.Text)
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// isAssistantMessage reports whether a conversation message type carries
// assistant output. The API uses "assistant_message" (and "user_message" for
// the prompt); older/empty types are treated as assistant for safety, while
// user-authored messages are always excluded.
func isAssistantMessage(msgType string) bool {
	if strings.Contains(msgType, "user") {
		return false
	}
	return msgType == "" || strings.Contains(msgType, "assistant")
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cursoragent: marshal request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("cursoragent: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cursoragent: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cursoragent: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cursoragent: %s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("cursoragent: decode response: %w", err)
		}
	}
	return nil
}
