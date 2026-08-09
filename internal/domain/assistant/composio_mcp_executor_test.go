package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	plugincred "github.com/cymonevo/go_template/internal/domain/plugin_credential"
	"github.com/cymonevo/go_template/pkg/composio"
	"github.com/cymonevo/go_template/pkg/llm"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
)

type stubComposioCreds struct {
	payload plugincred.ComposioPayload
	err     error
}

func (s stubComposioCreds) GetComposio(context.Context, string) (plugincred.ComposioPayload, error) {
	if s.err != nil {
		return plugincred.ComposioPayload{}, s.err
	}
	return s.payload, nil
}

type recordingComposioSession struct {
	createOpts composio.SessionCreateOpts
	sessionID  string
	searchQ    string
	metaCalls  []string
	toolCalls  []string
	scenario   string
}

func (r *recordingComposioSession) CreateSession(_ context.Context, userID string, opts composio.SessionCreateOpts) (*composio.Session, error) {
	r.createOpts = opts
	if r.sessionID == "" {
		r.sessionID = "trs_test"
	}
	if userID == "" {
		return nil, errors.New("user_id required")
	}
	return &composio.Session{SessionID: r.sessionID}, nil
}

func (r *recordingComposioSession) AttachSession(_ context.Context, sessionID string) (*composio.Session, error) {
	return &composio.Session{SessionID: sessionID}, nil
}

func (r *recordingComposioSession) SearchTools(_ context.Context, _ string, query string) (*composio.SearchToolsResult, error) {
	r.searchQ = query
	return &composio.SearchToolsResult{Success: true, Raw: json.RawMessage(`{"success":true}`)}, nil
}

func (r *recordingComposioSession) ExecuteMeta(_ context.Context, _ string, metaSlug string, _ map[string]any) (*composio.SessionExecuteResult, error) {
	r.metaCalls = append(r.metaCalls, metaSlug)
	switch r.scenario {
	case "needs_input":
		data, _ := json.Marshal(map[string]any{
			"status": "needs_input",
			"prompt": "Which repository should I use?",
		})
		return &composio.SessionExecuteResult{Data: data}, nil
	case "needs_confirmation":
		data, _ := json.Marshal(map[string]any{
			"status": "needs_confirmation",
			"prompt": "Create issue titled 'Bug' in repo org/app?",
		})
		return &composio.SessionExecuteResult{Data: data}, nil
	default:
		data, _ := json.Marshal(map[string]any{
			"status":     "completed",
			"reply_text": "GitHub issue created successfully.",
		})
		return &composio.SessionExecuteResult{Data: data}, nil
	}
}

func (r *recordingComposioSession) ExecuteTool(_ context.Context, _ string, toolSlug string, _ map[string]any) (*composio.SessionExecuteResult, error) {
	r.toolCalls = append(r.toolCalls, toolSlug)
	return &composio.SessionExecuteResult{Data: json.RawMessage(`{"reply_text":"done"}`)}, nil
}

func composioAIPlugin() *plugin.Plugin {
	return &plugin.Plugin{
		Slug: composioAISlug,
		Manifest: plugin.PluginManifest{
			Executor: plugin.Executor{Type: plugin.ExecutorTypeComposioMCP},
		},
	}
}

func TestComposioMCPExecutorSuccess(t *testing.T) {
	log, _ := logger.New("debug", false)
	sess := &recordingComposioSession{sessionID: "trs_ok", scenario: "success"}
	exec := NewComposioMCPExecutor(
		stubComposioCreds{payload: plugincred.ComposioPayload{APIKey: "user-key"}},
		llm.NewComposioAgent("mock", "", ""),
		sess,
		log,
	)

	out, err := exec.Execute(context.Background(), "user-1", composioAIPlugin(), map[string]any{
		"install_id":   "install-1",
		"task":         "create a github issue",
		"user_message": "create a github issue",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out["successful"] != true {
		t.Fatalf("expected success, got %+v", out)
	}
	if !strings.Contains(out["reply_text"].(string), "success") {
		t.Fatalf("unexpected reply: %+v", out)
	}
	if !sess.createOpts.ManageConnections.Enable {
		t.Fatal("expected manage_connections enabled")
	}
	if sess.searchQ != "create a github issue" {
		t.Fatalf("search query = %q", sess.searchQ)
	}
}

func TestComposioMCPExecutorNeedsInput(t *testing.T) {
	log, _ := logger.New("debug", false)
	sess := &recordingComposioSession{sessionID: "trs_input", scenario: "needs_input"}
	exec := NewComposioMCPExecutor(
		stubComposioCreds{payload: plugincred.ComposioPayload{APIKey: "user-key"}},
		llm.NewComposioAgent("mock", "", ""),
		sess,
		log,
	)

	out, err := exec.Execute(context.Background(), "user-1", composioAIPlugin(), map[string]any{
		"install_id":   "install-1",
		"task":         "create a github issue",
		"user_message": "create a github issue",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out["status"] != "needs_input" {
		t.Fatalf("expected needs_input, got %+v", out)
	}
	if out["composio_session_id"] != "trs_input" {
		t.Fatalf("session id = %v", out["composio_session_id"])
	}
}

func TestComposioMCPExecutorNeedsConfirmation(t *testing.T) {
	log, _ := logger.New("debug", false)
	sess := &recordingComposioSession{sessionID: "trs_confirm", scenario: "needs_confirmation"}
	exec := NewComposioMCPExecutor(
		stubComposioCreds{payload: plugincred.ComposioPayload{APIKey: "user-key"}},
		llm.NewComposioAgent("mock", "", ""),
		sess,
		log,
	)

	out, err := exec.Execute(context.Background(), "user-1", composioAIPlugin(), map[string]any{
		"install_id":   "install-1",
		"task":         "create a github issue",
		"user_message": "create a github issue",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out["status"] != "needs_confirmation" {
		t.Fatalf("expected needs_confirmation, got %+v", out)
	}
}

func TestComposioMCPExecutorInvalidCredentials(t *testing.T) {
	log, _ := logger.New("debug", false)
	exec := NewComposioMCPExecutor(
		stubComposioCreds{err: response.NewNotFound("composio credentials not found")},
		llm.NewComposioAgent("mock", "", ""),
		&recordingComposioSession{},
		log,
	)

	_, err := exec.Execute(context.Background(), "user-1", composioAIPlugin(), map[string]any{
		"install_id":   "install-1",
		"task":         "create a github issue",
		"user_message": "create a github issue",
	})
	if !errors.Is(err, ErrComposioSetupIncomplete) {
		t.Fatalf("expected setup incomplete, got %v", err)
	}
}

func TestComposioMCPExecutorResumeSession(t *testing.T) {
	log, _ := logger.New("debug", false)
	sess := &recordingComposioSession{sessionID: "trs_resume", scenario: "success"}
	exec := NewComposioMCPExecutor(
		stubComposioCreds{payload: plugincred.ComposioPayload{APIKey: "user-key"}},
		llm.NewComposioAgent("mock", "", ""),
		sess,
		log,
	)

	out, err := exec.Execute(context.Background(), "user-1", composioAIPlugin(), map[string]any{
		"install_id":          "install-1",
		"task":                "create a github issue",
		"composio_session_id": "trs_resume",
		"user_message":        "org/app",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out["successful"] != true {
		t.Fatalf("expected success, got %+v", out)
	}
	if len(sess.metaCalls) == 0 || sess.metaCalls[0] != "COMPOSIO_HANDLE_INPUT" {
		t.Fatalf("expected handle input meta call, got %+v", sess.metaCalls)
	}
}
