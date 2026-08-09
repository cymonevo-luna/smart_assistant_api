package assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant/builtin"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/store"
)

func TestRoutingExecutorBuiltinReminderAdapter(t *testing.T) {
	log, _ := logger.New("debug", false)
	reminderSvc := reminder.NewService(reminder.NewRepository(store.NewMemoryStore[reminder.Reminder]()))
	builtinExec := NewBuiltinExecutor(reminderSvc, nil, nil, log)

	p := &plugin.Plugin{
		Slug: builtin.ReminderSlug,
		Manifest: plugin.PluginManifest{
			Executor: plugin.Executor{
				Type: plugin.ExecutorTypeBuiltin,
				Config: map[string]any{
					"builtin_adapter": builtin.AdapterReminder,
				},
			},
		},
	}

	router := NewRoutingExecutor(nil, nil, builtinExec, NewStubExecutor(log))
	result, err := router.Execute(context.Background(), "user-1", p, map[string]any{
		"operation":  "create",
		"message":    "call mom",
		"remind_at":  "2099-01-01T14:00:00Z",
		"install_id": "install-1",
	})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported executor type") {
			t.Fatalf("builtin executor should be supported, got: %v", err)
		}
		t.Fatalf("Execute: %v", err)
	}
	replyText, _ := result["reply_text"].(string)
	if !strings.Contains(replyText, "call mom") {
		t.Fatalf("reply_text = %q", replyText)
	}
}
