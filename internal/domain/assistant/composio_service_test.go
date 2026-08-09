package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	assistantsettings "github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	userplugin "github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/logger"
)

type stubComposioExecutor struct {
	results []map[string]any
	calls   int
}

func (s *stubComposioExecutor) Execute(_ context.Context, _ string, _ *plugin.Plugin, _ map[string]any) (map[string]any, error) {
	idx := s.calls
	s.calls++
	if idx >= len(s.results) {
		return map[string]any{"successful": true, "reply_text": "done"}, nil
	}
	return s.results[idx], nil
}

func composioServiceHarness(t *testing.T, executor PluginExecutor) (*Service, *fakeSessionRepo) {
	t.Helper()
	sessions := newFakeSessionRepo()
	messages := &fakeMessageRepo{}
	settingsRepo := &fakeSettingsRepo{
		settings: &assistantsettings.Settings{
			UserID:    "user-1",
			WakeWord:  "Jarvis",
			UpdatedAt: time.Now().UTC(),
		},
	}
	settingsSvc := assistantsettings.NewService(settingsRepo)
	log, _ := logger.New("debug", false)

	catalog := plugin.Plugin{
		ID:   "plugin-composio",
		Slug: composioAISlug,
		Name: "Composio AI",
		Manifest: plugin.PluginManifest{
			RequiredSetup: true,
			Executor:      plugin.Executor{Type: plugin.ExecutorTypeComposioMCP},
			Arguments: []plugin.ManifestArgument{
				{Name: "task", Type: "string", Required: true, Prompt: "What would you like me to do?"},
			},
		},
	}
	userPlugins := fakeUserPluginRepo{installs: []userplugin.UserPlugin{{
		ID: "install-1", UserID: "user-1", PluginID: "plugin-composio", Enabled: true,
		SetupStatus: userplugin.SetupStatusCompleted,
	}}}

	svc := NewService(sessions, messages, settingsSvc, userPlugins, fakePluginRepo{plugins: []plugin.Plugin{catalog}}, nil, executor, nil, log)
	return svc, sessions
}

func TestComposioPendingActionResumeFollowUp(t *testing.T) {
	exec := &stubComposioExecutor{
		results: []map[string]any{
			{
				"successful": true,
				"reply_text": "GitHub issue created successfully.",
			},
		},
	}
	svc, sessions := composioServiceHarness(t, exec)
	session := &Session{
		ID: "sess-1", UserID: "user-1", Status: SessionStatusActive,
		PendingAction: &PendingAction{
			PluginSlug:          composioAISlug,
			PluginID:            "plugin-composio",
			InstallID:           "install-1",
			Arguments:           map[string]any{"task": "create a github issue"},
			ComposioSessionID:   "trs_1",
			ComposioPendingKind: "input",
			ComposioPrompt:      "Which repository?",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = sessions.Create(context.Background(), session)

	out, err := svc.ProcessMessage(context.Background(), "user-1", "sess-1", ProcessMessageInput{
		Text:   "org/app",
		Source: MessageSourceButton,
	})
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if out.Reply.Type != ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q", out.Reply.Type)
	}
	if out.Reply.Action == nil || out.Reply.Action.Status != ActionStatusSuccess {
		t.Fatalf("expected success action, got %+v", out.Reply.Action)
	}
	if !strings.Contains(out.Reply.Text, "success") {
		t.Fatalf("unexpected reply text: %q", out.Reply.Text)
	}
}

func TestComposioSetupIncompleteBlocked(t *testing.T) {
	sessions := newFakeSessionRepo()
	messages := &fakeMessageRepo{}
	settingsRepo := &fakeSettingsRepo{
		settings: &assistantsettings.Settings{
			UserID:    "user-1",
			WakeWord:  "Jarvis",
			UpdatedAt: time.Now().UTC(),
		},
	}
	settingsSvc := assistantsettings.NewService(settingsRepo)
	log, _ := logger.New("debug", false)

	catalog := plugin.Plugin{
		ID:   "plugin-composio",
		Slug: composioAISlug,
		Name: "Composio AI",
		Manifest: plugin.PluginManifest{
			RequiredSetup: true,
			Executor:      plugin.Executor{Type: plugin.ExecutorTypeComposioMCP},
		},
	}
	userPlugins := fakeUserPluginRepo{installs: []userplugin.UserPlugin{{
		ID: "install-1", UserID: "user-1", PluginID: "plugin-composio", Enabled: true,
		SetupStatus: userplugin.SetupStatusNotStarted,
	}}}

	svc := NewService(sessions, messages, settingsSvc, userPlugins, fakePluginRepo{plugins: []plugin.Plugin{catalog}}, nil, NewStubExecutor(log), nil, log)
	session := &Session{
		ID: "sess-1", UserID: "user-1", Status: SessionStatusActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	_ = sessions.Create(context.Background(), session)

	out, err := svc.ProcessMessage(context.Background(), "user-1", "sess-1", ProcessMessageInput{
		Text:   "create a github issue",
		Source: MessageSourceButton,
	})
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.Reply.Text), "setup") {
		t.Fatalf("expected setup guidance, got %q", out.Reply.Text)
	}
	if out.Reply.Action == nil || out.Reply.Action.Payload["reason"] != actionReasonSetupIncomplete {
		t.Fatalf("expected setup_incomplete payload, got %+v", out.Reply.Action)
	}
}

func TestComposioConfirmationYesCompletes(t *testing.T) {
	exec := &stubComposioExecutor{
		results: []map[string]any{
			{"successful": true, "reply_text": "Issue created."},
		},
	}
	svc, sessions := composioServiceHarness(t, exec)
	session := &Session{
		ID: "sess-1", UserID: "user-1", Status: SessionStatusActive,
		PendingAction: &PendingAction{
			PluginSlug:          composioAISlug,
			PluginID:            "plugin-composio",
			InstallID:           "install-1",
			Arguments:           map[string]any{"task": "create a github issue"},
			ComposioSessionID:   "trs_confirm",
			ComposioPendingKind: "confirmation",
			ComposioPrompt:      "Create issue?",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = sessions.Create(context.Background(), session)

	out, err := svc.ProcessMessage(context.Background(), "user-1", "sess-1", ProcessMessageInput{
		Text:   "yes",
		Source: MessageSourceButton,
	})
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if out.Reply.Type != ReplyTypeActionResult {
		t.Fatalf("expected action_result, got %q", out.Reply.Type)
	}
	if out.Reply.Action == nil || out.Reply.Action.Status != ActionStatusSuccess {
		t.Fatalf("expected success, got %+v", out.Reply.Action)
	}
}
