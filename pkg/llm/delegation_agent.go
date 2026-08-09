package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	plugindomain "github.com/cymonevo/go_template/internal/domain/plugin"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-4o-mini"
)

// PluginDelegationAgent semantically routes user input to installed plugins.
type PluginDelegationAgent struct {
	Provider   string
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type openAIChatRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIChatMessage  `json:"messages"`
	Temperature    float64              `json:"temperature"`
	ResponseFormat openAIResponseFormat `json:"response_format"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type delegationClassifyResponse struct {
	Matched    bool           `json:"matched"`
	PluginSlug string         `json:"plugin_slug"`
	Arguments  map[string]any `json:"arguments"`
}

// Classify routes the request to the configured provider.
func (a *PluginDelegationAgent) Classify(ctx context.Context, req ClassifyRequest) (*ClassifyResult, error) {
	switch a.Provider {
	case "openai":
		return a.classifyOpenAI(ctx, req)
	case "mock":
		return mockSemanticClassify(req), nil
	default:
		return mockSemanticClassify(req), nil
	}
}

func (a *PluginDelegationAgent) classifyOpenAI(ctx context.Context, req ClassifyRequest) (*ClassifyResult, error) {
	if a.APIKey == "" {
		return nil, fmt.Errorf("llm: openai api key not configured")
	}

	model := a.Model
	if model == "" {
		model = defaultOpenAIModel
	}

	messages := []openAIChatMessage{
		{Role: "system", Content: buildDelegationSystemPrompt(req.Plugins)},
	}
	for _, turn := range req.RecentMessages {
		role := strings.TrimSpace(turn.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		messages = append(messages, openAIChatMessage{Role: role, Content: content})
	}
	messages = append(messages, openAIChatMessage{Role: "user", Content: req.Text})

	payload := openAIChatRequest{
		Model:          model,
		Messages:       messages,
		Temperature:    0,
		ResponseFormat: openAIResponseFormat{Type: "json_object"},
	}

	body, err := json.Marshal(payload)
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
		log.Printf("llm: openai classification failed: %v", err)
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
	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return nil, fmt.Errorf("openai api error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai response missing choices")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	var parsed delegationClassifyResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("decode classifier json: %w", err)
	}
	if !parsed.Matched {
		return &ClassifyResult{Matched: false}, nil
	}

	return validateDelegationResult(req.Plugins, parsed)
}

func buildDelegationSystemPrompt(plugins []PluginCandidate) string {
	var b strings.Builder
	b.WriteString("You are a plugin delegation agent. Read the user message")
	b.WriteString(" (and any recent conversation turns) and decide whether exactly one installed plugin can fulfill the request.\n")
	b.WriteString("Compare the request against each candidate plugin's name, description, and argument definitions.\n")
	b.WriteString("Do not require exact trigger phrase matches; paraphrases and synonyms are valid.\n")
	b.WriteString("When confident, extract argument values from the message.\n")
	b.WriteString("Reply with strict JSON only:\n")
	b.WriteString(`{"matched":true,"plugin_slug":"<slug>","arguments":{...}} or {"matched":false}` + "\n")
	b.WriteString("plugin_slug must be one of the listed slugs. ")
	b.WriteString("Only include argument keys defined for the matched plugin. ")
	b.WriteString("Use RFC3339/ISO-8601 for datetime values.\n\n")
	b.WriteString("Plugins:\n")
	for _, p := range plugins {
		b.WriteString("- slug: ")
		b.WriteString(p.Slug)
		b.WriteString(", name: ")
		b.WriteString(p.Name)
		if desc := strings.TrimSpace(p.Description); desc != "" {
			b.WriteString(", description: ")
			b.WriteString(desc)
		}
		if len(p.Triggers) > 0 {
			b.WriteString(", example phrases (hints only, not required): ")
			writeJSONValue(&b, p.Triggers)
		}
		b.WriteString(", arguments: ")
		writeJSONValue(&b, p.Arguments)
		b.WriteByte('\n')
	}
	return b.String()
}

func writeJSONValue(b *strings.Builder, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		b.WriteString("[]")
		return
	}
	b.Write(data)
}

func validateDelegationResult(plugins []PluginCandidate, parsed delegationClassifyResponse) (*ClassifyResult, error) {
	slug := strings.TrimSpace(parsed.PluginSlug)
	if slug == "" {
		return &ClassifyResult{Matched: false}, nil
	}

	var matched *PluginCandidate
	for i := range plugins {
		if plugins[i].Slug == slug {
			matched = &plugins[i]
			break
		}
	}
	if matched == nil {
		return &ClassifyResult{Matched: false}, nil
	}

	args := sanitizeArguments(*matched, parsed.Arguments)
	return &ClassifyResult{
		Matched:    true,
		PluginSlug: slug,
		Arguments:  args,
	}, nil
}

func sanitizeArguments(candidate PluginCandidate, raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	allowed := make(map[string]plugindomain.ManifestArgument, len(candidate.Arguments))
	for _, arg := range candidate.Arguments {
		allowed[arg.Name] = arg
	}

	out := make(map[string]any)
	for key, value := range raw {
		schema, ok := allowed[key]
		if !ok {
			continue
		}
		coerced, ok := coerceArgumentValue(schema.Type, value)
		if ok {
			out[key] = coerced
		}
	}
	return out
}

func coerceArgumentValue(argType string, raw any) (any, bool) {
	switch strings.ToLower(strings.TrimSpace(argType)) {
	case "email":
		s, ok := coerceString(raw)
		if !ok || !isEmail(s) {
			return nil, false
		}
		return s, true
	case "datetime":
		s, ok := coerceString(raw)
		if !ok {
			return nil, false
		}
		return coerceDateTime(s)
	default:
		s, ok := coerceString(raw)
		if !ok {
			return nil, false
		}
		return s, true
	}
}

func coerceString(raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", false
		}
		return s, true
	case fmt.Stringer:
		s := strings.TrimSpace(v.String())
		if s == "" {
			return "", false
		}
		return s, true
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return "", false
		}
		return s, true
	}
}

func coerceDateTime(value string) (string, bool) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Format(time.RFC3339), true
		}
	}
	return "", false
}

func isEmail(value string) bool {
	_, err := mail.ParseAddress(value)
	return err == nil
}

func truncateForLog(s string) string {
	const max = 256
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// NewClassifier builds a Classifier from provider configuration.
func NewClassifier(provider, apiKey, model string) Classifier {
	switch provider {
	case "openai":
		return &PluginDelegationAgent{
			Provider: "openai",
			APIKey:   apiKey,
			Model:    model,
		}
	case "mock":
		return &PluginDelegationAgent{Provider: "mock"}
	default:
		return &PluginDelegationAgent{Provider: "mock"}
	}
}
