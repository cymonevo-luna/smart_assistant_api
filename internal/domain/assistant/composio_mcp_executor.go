package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	plugincred "github.com/cymonevo/go_template/internal/domain/plugin_credential"
	"github.com/cymonevo/go_template/pkg/composio"
	"github.com/cymonevo/go_template/pkg/llm"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
)

const (
	composioMCPMaxIterations = 10
	composioAISlug           = "composio-ai"
)

// ErrComposioSetupIncomplete indicates the install has no usable Composio credentials.
var ErrComposioSetupIncomplete = errors.New("setup incomplete")

// ComposioSessionClient is the subset of Composio v3.1 session APIs used by the executor.
type ComposioSessionClient interface {
	CreateSession(ctx context.Context, userID string, opts composio.SessionCreateOpts) (*composio.Session, error)
	AttachSession(ctx context.Context, sessionID string) (*composio.Session, error)
	SearchTools(ctx context.Context, sessionID, query string) (*composio.SearchToolsResult, error)
	ExecuteTool(ctx context.Context, sessionID, toolSlug string, args map[string]any) (*composio.SessionExecuteResult, error)
	ExecuteMeta(ctx context.Context, sessionID, metaSlug string, args map[string]any) (*composio.SessionExecuteResult, error)
}

// ComposioCredentialService loads per-install Composio credentials.
type ComposioCredentialService interface {
	GetComposio(ctx context.Context, userPluginID string) (plugincred.ComposioPayload, error)
}

// ComposioMCPExecutor runs composio_mcp plugin manifests via Composio tool-router sessions.
type ComposioMCPExecutor struct {
	creds  ComposioCredentialService
	agent  llm.ComposioAgent
	client ComposioSessionClient
	log    logger.Logger
}

// NewComposioMCPExecutor constructs a ComposioMCPExecutor.
func NewComposioMCPExecutor(
	creds ComposioCredentialService,
	agent llm.ComposioAgent,
	client ComposioSessionClient,
	log logger.Logger,
) *ComposioMCPExecutor {
	return &ComposioMCPExecutor{
		creds:  creds,
		agent:  agent,
		client: client,
		log:    log,
	}
}

// Execute runs a Composio MCP task for the given install.
func (e *ComposioMCPExecutor) Execute(ctx context.Context, userID string, p *plugin.Plugin, args map[string]any) (map[string]any, error) {
	if p.Manifest.Executor.Type != plugin.ExecutorTypeComposioMCP {
		return nil, fmt.Errorf("executor type %q is not composio_mcp", p.Manifest.Executor.Type)
	}

	installID, _ := args["install_id"].(string)
	if installID == "" {
		return nil, fmt.Errorf("install_id is required")
	}

	payload, err := e.creds.GetComposio(ctx, installID)
	if err != nil {
		if respErr, ok := err.(*response.AppError); ok && respErr.Code == "not_found" {
			return nil, ErrComposioSetupIncomplete
		}
		return nil, err
	}
	if strings.TrimSpace(payload.APIKey) == "" {
		return nil, ErrComposioSetupIncomplete
	}

	client := e.sessionClient(payload)
	task := extractComposioTask(args)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}

	sessionID, _ := args["composio_session_id"].(string)
	userMessage, _ := args["user_message"].(string)
	confirmed, _ := args["confirmed"].(bool)
	resume := strings.TrimSpace(sessionID) != ""

	if !resume {
		opts := composio.SessionCreateOpts{}
		opts.ManageConnections.Enable = true
		if accounts := connectedAccountsSnapshot(payload.ConnectedAccounts); len(accounts) > 0 {
			opts.ConnectedAccounts = accounts
		}
		sess, err := client.CreateSession(ctx, userID, opts)
		if err != nil {
			return nil, e.wrapComposioErr(err)
		}
		sessionID = sess.SessionID
		userMessage = ""
	} else if _, err := client.AttachSession(ctx, sessionID); err != nil {
		return nil, e.wrapComposioErr(err)
	}

	return e.runAgentLoop(ctx, client, sessionID, task, userMessage, confirmed, resume)
}

func (e *ComposioMCPExecutor) sessionClient(payload plugincred.ComposioPayload) ComposioSessionClient {
	if e.client != nil {
		return e.client
	}
	cfg := composio.Config{
		APIKey:  payload.APIKey,
		BaseURL: payload.BaseURL,
	}
	return composio.New(cfg)
}

func (e *ComposioMCPExecutor) runAgentLoop(
	ctx context.Context,
	client ComposioSessionClient,
	sessionID, task, userMessage string,
	confirmed bool,
	resume bool,
) (map[string]any, error) {
	var (
		searchRaw  json.RawMessage
		lastResult json.RawMessage
	)

	for i := 0; i < composioMCPMaxIterations; i++ {
		if i == 0 && !resume && !confirmed {
			search, err := client.SearchTools(ctx, sessionID, task)
			if err != nil {
				if outcome := rateLimitOutcome(err); outcome != nil {
					return outcome, nil
				}
				return nil, e.wrapComposioErr(err)
			}
			searchRaw = search.Raw
		}

		step, err := e.agent.PlanNextStep(ctx, llm.ComposioAgentRequest{
			Task:          task,
			UserMessage:   userMessage,
			SearchResults: searchRaw,
			LastResult:    lastResult,
			Confirmed:     confirmed && i == 0,
			Iteration:     i,
		})
		if err != nil {
			return nil, err
		}
		if step == nil {
			return nil, fmt.Errorf("composio agent returned no step")
		}

		switch step.Action {
		case "done":
			reply := strings.TrimSpace(step.ReplyText)
			if reply == "" {
				reply = "Done."
			}
			return map[string]any{
				"successful":          true,
				"reply_text":          reply,
				"composio_session_id": sessionID,
			}, nil
		case "execute_meta":
			result, execErr := client.ExecuteMeta(ctx, sessionID, step.MetaSlug, step.Arguments)
			if outcome := interpretComposioOutcome(sessionID, result, execErr); outcome != nil {
				return outcome, nil
			}
			if result != nil {
				lastResult = result.Data
			}
		case "execute_tool":
			result, execErr := client.ExecuteTool(ctx, sessionID, step.ToolSlug, step.Arguments)
			if outcome := interpretComposioOutcome(sessionID, result, execErr); outcome != nil {
				return outcome, nil
			}
			if result != nil {
				lastResult = result.Data
			}
		default:
			return nil, fmt.Errorf("unsupported composio agent action %q", step.Action)
		}

		userMessage = ""
		confirmed = false
	}

	return map[string]any{
		"successful": false,
		"reply_text": "I wasn't able to finish that task. Please try again.",
	}, nil
}

func extractComposioTask(args map[string]any) string {
	if task := stringArgValue(args, "task"); task != "" {
		return task
	}
	return stringArgValue(args, "user_message")
}

func connectedAccountsSnapshot(accounts []plugincred.ComposioConnectedAccount) map[string][]string {
	out := make(map[string][]string)
	for _, acct := range accounts {
		if acct.ToolkitSlug == "" || acct.ID == "" {
			continue
		}
		out[acct.ToolkitSlug] = append(out[acct.ToolkitSlug], acct.ID)
	}
	return out
}

func interpretComposioOutcome(sessionID string, result *composio.SessionExecuteResult, err error) map[string]any {
	data := map[string]any{}
	if result != nil && len(result.Data) > 0 {
		_ = json.Unmarshal(result.Data, &data)
	}

	if status := strings.ToLower(stringFieldFromMap(data, "status")); status != "" {
		switch status {
		case "needs_input":
			return composioPendingOutcome(sessionID, "input", promptFromComposioData(data, err))
		case "needs_confirmation":
			return composioPendingOutcome(sessionID, "confirmation", promptFromComposioData(data, err))
		case "auth_required", "needs_auth":
			return composioAuthOutcome(sessionID, data, promptFromComposioData(data, err))
		case "cannot_handle":
			return map[string]any{
				"successful": false,
				"reply_text": "I can't do that with your connected apps.",
			}
		case "completed", "success":
			return map[string]any{
				"successful": true,
				"reply_text": replyTextFromComposioData(data),
			}
		}
	}

	if typ := strings.ToLower(stringFieldFromMap(data, "type")); typ == "elicitation" {
		return composioPendingOutcome(sessionID, "input", promptFromComposioData(data, err))
	}
	if strings.ToLower(stringFieldFromMap(data, "type")) == "confirmation" {
		return composioPendingOutcome(sessionID, "confirmation", promptFromComposioData(data, err))
	}

	if authURL := authURLFromComposioData(data); authURL != "" {
		return composioAuthOutcome(sessionID, data, promptFromComposioData(data, err))
	}

	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "needs user input") || strings.Contains(lower, "needs_input") {
			return composioPendingOutcome(sessionID, "input", promptFromComposioData(data, err))
		}
		if strings.Contains(lower, "confirmation") {
			return composioPendingOutcome(sessionID, "confirmation", promptFromComposioData(data, err))
		}
		if outcome := rateLimitOutcome(err); outcome != nil {
			return outcome
		}
		return nil
	}

	if result != nil && len(result.Data) > 0 {
		reply := replyTextFromComposioData(data)
		if reply != "" {
			return map[string]any{
				"successful":          true,
				"reply_text":          reply,
				"composio_session_id": sessionID,
			}
		}
	}
	return nil
}

func composioPendingOutcome(sessionID, kind, prompt string) map[string]any {
	if strings.TrimSpace(prompt) == "" {
		prompt = "I need a bit more information to continue."
	}
	return map[string]any{
		"status":                kindToStatus(kind),
		"prompt":                prompt,
		"composio_session_id":   sessionID,
		"composio_pending_kind": kind,
	}
}

func composioAuthOutcome(sessionID string, data map[string]any, prompt string) map[string]any {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Please connect the required app to continue."
	}
	out := composioPendingOutcome(sessionID, "auth", prompt)
	out["status"] = "needs_auth"
	if authURL := authURLFromComposioData(data); authURL != "" {
		out["auth_url"] = authURL
	}
	return out
}

func kindToStatus(kind string) string {
	switch kind {
	case "confirmation":
		return "needs_confirmation"
	case "auth":
		return "needs_auth"
	default:
		return "needs_input"
	}
}

func promptFromComposioData(data map[string]any, err error) string {
	for _, key := range []string{"prompt", "message", "question"} {
		if v := stringFieldFromMap(data, key); v != "" {
			return v
		}
	}
	if err != nil {
		return strings.TrimPrefix(err.Error(), "composio: session execute: ")
	}
	return ""
}

func replyTextFromComposioData(data map[string]any) string {
	for _, key := range []string{"reply_text", "message", "summary", "result"} {
		if v := stringFieldFromMap(data, key); v != "" {
			return v
		}
	}
	return ""
}

func authURLFromComposioData(data map[string]any) string {
	for _, key := range []string{"auth_url", "connect_url", "url"} {
		if v := stringFieldFromMap(data, key); v != "" && strings.HasPrefix(v, "http") {
			return v
		}
	}
	return ""
}

func stringFieldFromMap(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	val, ok := data[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func rateLimitOutcome(err error) map[string]any {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "status 429") || strings.Contains(strings.ToLower(err.Error()), "rate limit") {
		return map[string]any{
			"successful": false,
			"reply_text": "Composio is busy right now. Please try again in a moment.",
		}
	}
	return nil
}

func (e *ComposioMCPExecutor) wrapComposioErr(err error) error {
	if outcome := rateLimitOutcome(err); outcome != nil {
		return fmt.Errorf("%s", outcome["reply_text"])
	}
	e.log.Error("composio mcp execution failed", logger.Err(err))
	return err
}

func isComposioAIPlugin(p *plugin.Plugin) bool {
	if p == nil {
		return false
	}
	if p.Slug == composioAISlug {
		return true
	}
	return p.Manifest.Executor.Type == plugin.ExecutorTypeComposioMCP
}
