package llm

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

// ComposioAgentRequest is input for planning the next Composio MCP agent step.
type ComposioAgentRequest struct {
	Task          string
	UserMessage   string
	SearchResults json.RawMessage
	LastResult    json.RawMessage
	Confirmed     bool
	Iteration     int
}

// ComposioAgentStep is the next action the executor should take.
type ComposioAgentStep struct {
	Action    string         `json:"action"`
	MetaSlug  string         `json:"meta_slug,omitempty"`
	ToolSlug  string         `json:"tool_slug,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	ReplyText string         `json:"reply_text,omitempty"`
}

// ComposioAgent plans Composio tool-router steps for a task.
type ComposioAgent interface {
	PlanNextStep(ctx context.Context, req ComposioAgentRequest) (*ComposioAgentStep, error)
}

// ComposioAgentLLM uses an LLM provider to plan Composio MCP steps.
type ComposioAgentLLM struct {
	Provider   string
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type composioAgentLLMResponse struct {
	Action    string         `json:"action"`
	MetaSlug  string         `json:"meta_slug,omitempty"`
	ToolSlug  string         `json:"tool_slug,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	ReplyText string         `json:"reply_text,omitempty"`
}

// PlanNextStep routes to the configured provider.
func (a *ComposioAgentLLM) PlanNextStep(ctx context.Context, req ComposioAgentRequest) (*ComposioAgentStep, error) {
	switch a.Provider {
	case "openai":
		return a.planOpenAI(ctx, req)
	case "mock":
		return mockComposioAgentPlan(req), nil
	default:
		return mockComposioAgentPlan(req), nil
	}
}

func (a *ComposioAgentLLM) planOpenAI(ctx context.Context, req ComposioAgentRequest) (*ComposioAgentStep, error) {
	if a.APIKey == "" {
		return nil, fmt.Errorf("llm: openai api key not configured")
	}

	model := a.Model
	if model == "" {
		model = defaultOpenAIModel
	}

	system := strings.TrimSpace(`You plan the next step for a Composio tool-router agent.
Reply with strict JSON only using one of:
{"action":"execute_meta","meta_slug":"COMPOSIO_RUN_TASK","arguments":{"task":"..."}}
{"action":"execute_meta","meta_slug":"COMPOSIO_HANDLE_INPUT","arguments":{"input":"..."}}
{"action":"execute_meta","meta_slug":"COMPOSIO_CONFIRM","arguments":{"confirmed":true}}
{"action":"execute_tool","tool_slug":"TOOL_SLUG","arguments":{...}}
{"action":"done","reply_text":"..."}
Use COMPOSIO_RUN_TASK on the first turn for a new task, COMPOSIO_HANDLE_INPUT when resuming with user input, and COMPOSIO_CONFIRM after the user confirmed an action.`)

	payload := map[string]any{
		"task":           req.Task,
		"user_message":   req.UserMessage,
		"confirmed":      req.Confirmed,
		"iteration":      req.Iteration,
		"search_results": json.RawMessage(req.SearchResults),
		"last_result":    json.RawMessage(req.LastResult),
	}
	userBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal composio agent context: %w", err)
	}

	messages := []openAIChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(userBody)},
	}
	body, err := json.Marshal(openAIChatRequest{
		Model:          model,
		Messages:       messages,
		Temperature:    0,
		ResponseFormat: openAIResponseFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	baseURL := strings.TrimRight(a.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai http %d: %s", resp.StatusCode, truncateForLog(string(respBody)))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai response missing choices")
	}

	var parsed composioAgentLLMResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(chatResp.Choices[0].Message.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("decode composio agent json: %w", err)
	}
	return &ComposioAgentStep{
		Action:    parsed.Action,
		MetaSlug:  parsed.MetaSlug,
		ToolSlug:  parsed.ToolSlug,
		Arguments: parsed.Arguments,
		ReplyText: parsed.ReplyText,
	}, nil
}

func mockComposioAgentPlan(req ComposioAgentRequest) *ComposioAgentStep {
	if req.Confirmed {
		return &ComposioAgentStep{
			Action:    "execute_meta",
			MetaSlug:  "COMPOSIO_CONFIRM",
			Arguments: map[string]any{"confirmed": true},
		}
	}
	if strings.TrimSpace(req.UserMessage) != "" {
		return &ComposioAgentStep{
			Action:    "execute_meta",
			MetaSlug:  "COMPOSIO_HANDLE_INPUT",
			Arguments: map[string]any{"input": req.UserMessage},
		}
	}
	if req.Iteration == 0 {
		return &ComposioAgentStep{
			Action:    "execute_meta",
			MetaSlug:  "COMPOSIO_RUN_TASK",
			Arguments: map[string]any{"task": req.Task},
		}
	}
	return &ComposioAgentStep{Action: "done", ReplyText: "Done."}
}

// NewComposioAgent builds a ComposioAgent from provider configuration.
func NewComposioAgent(provider, apiKey, model string) ComposioAgent {
	return &ComposioAgentLLM{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
	}
}
