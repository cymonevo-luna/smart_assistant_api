package assistant

import (
	"context"
	"fmt"

	"github.com/cymonevo/go_template/internal/domain/assistant/builtin"
	assistantsettings "github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/calendar/availability"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/pkg/composio"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/places"
)

// PluginExecutor runs a matched plugin action server-side.
type PluginExecutor interface {
	Execute(ctx context.Context, userID string, p *plugin.Plugin, args map[string]any) (map[string]any, error)
}

// ComposioExecutor executes composio-type plugin manifests.
type ComposioExecutor struct {
	client *composio.Client
	log    logger.Logger
}

// NewComposioExecutor constructs a ComposioExecutor.
func NewComposioExecutor(client *composio.Client, log logger.Logger) *ComposioExecutor {
	return &ComposioExecutor{client: client, log: log}
}

// Execute runs the composio tool defined in the manifest config.
func (e *ComposioExecutor) Execute(ctx context.Context, userID string, p *plugin.Plugin, args map[string]any) (map[string]any, error) {
	if p.Manifest.Executor.Type != plugin.ExecutorTypeComposio {
		return nil, fmt.Errorf("executor type %q is not composio", p.Manifest.Executor.Type)
	}
	cfg := p.Manifest.Executor.Config
	toolSlug, _ := cfg["tool_slug"].(string)
	if toolSlug == "" {
		toolSlug, _ = cfg["slug"].(string)
	}
	if toolSlug == "" {
		return nil, fmt.Errorf("composio tool slug not configured for plugin %q", p.Slug)
	}

	client := e.client.ForEntity(userID)
	if ids, ok := cfg["connected_account_ids"].([]any); ok {
		strIDs := make([]string, 0, len(ids))
		for _, id := range ids {
			if s, ok := id.(string); ok {
				strIDs = append(strIDs, s)
			}
		}
		client = client.ForEntity(userID, strIDs...)
	}

	execArgs := args
	if mapped, err := mapBuiltinAdapterArgs(p, args); err != nil {
		return nil, err
	} else if mapped != nil {
		execArgs = mapped
	}

	result, err := client.Execute(ctx, toolSlug, execArgs)
	if err != nil {
		e.log.Error("composio execution failed",
			logger.String("plugin", p.Slug),
			logger.String("tool", toolSlug),
			logger.Err(err))
		return nil, err
	}
	out := map[string]any{"successful": result.Successful}
	if len(result.Data) > 0 {
		out["data"] = string(result.Data)
	}
	return out, nil
}

// BuiltinExecutor executes builtin-type plugin manifests.
type BuiltinExecutor struct {
	reminders *reminder.Service
	settings  *assistantsettings.Service
	places    places.Provider
	log       logger.Logger
}

// NewBuiltinExecutor constructs a BuiltinExecutor.
func NewBuiltinExecutor(
	reminders *reminder.Service,
	settings *assistantsettings.Service,
	placesProvider places.Provider,
	log logger.Logger,
) *BuiltinExecutor {
	return &BuiltinExecutor{
		reminders: reminders,
		settings:  settings,
		places:    placesProvider,
		log:       log,
	}
}

// Execute dispatches to the configured builtin adapter.
func (e *BuiltinExecutor) Execute(ctx context.Context, userID string, p *plugin.Plugin, args map[string]any) (map[string]any, error) {
	if p.Manifest.Executor.Type != plugin.ExecutorTypeBuiltin {
		return nil, fmt.Errorf("executor type %q is not builtin", p.Manifest.Executor.Type)
	}

	adapter, _ := p.Manifest.Executor.Config["builtin_adapter"].(string)
	if adapter == "" {
		switch p.Slug {
		case builtin.ReminderSlug:
			adapter = builtin.AdapterReminder
		case builtin.LocationReminderSlug:
			adapter = builtin.AdapterLocationReminder
		default:
			return nil, fmt.Errorf("builtin adapter not configured for plugin %q", p.Slug)
		}
	}

	switch adapter {
	case builtin.AdapterReminder:
		installID, _ := args["install_id"].(string)
		return builtin.ExecuteReminder(ctx, e.reminders, userID, installID, args)
	case builtin.AdapterLocationReminder:
		return builtin.ExecuteLocationReminder(ctx, e.reminders, e.settings, e.places, userID, args)
	default:
		return nil, fmt.Errorf("unsupported builtin adapter %q for plugin %q", adapter, p.Slug)
	}
}

// StubExecutor simulates successful plugin execution when Composio is not configured.
type StubExecutor struct {
	log logger.Logger
}

// NewStubExecutor returns an executor that logs and succeeds (for tests/dev).
func NewStubExecutor(log logger.Logger) *StubExecutor {
	return &StubExecutor{log: log}
}

// Execute logs the invocation and returns a synthetic success payload.
func (e *StubExecutor) Execute(_ context.Context, userID string, p *plugin.Plugin, args map[string]any) (map[string]any, error) {
	e.log.Info("stub plugin execution",
		logger.String("user_id", userID),
		logger.String("plugin", p.Slug),
		logger.Any("arguments", args))
	return map[string]any{"successful": true, "stub": true}, nil
}

func mapBuiltinAdapterArgs(p *plugin.Plugin, args map[string]any) (map[string]any, error) {
	adapter, _ := p.Manifest.Executor.Config["builtin_adapter"].(string)
	if adapter == "" {
		if p.Slug == builtin.GoogleCalendarMeetSlug {
			adapter = builtin.AdapterGoogleCalendarMeet
		} else {
			return nil, nil
		}
	}
	switch adapter {
	case builtin.AdapterGoogleCalendarMeet:
		return builtin.MapGoogleCalendarMeetArgs(stripInternalArgs(args), availability.MeetingTimezone)
	default:
		return nil, fmt.Errorf("unsupported builtin adapter %q for plugin %q", adapter, p.Slug)
	}
}

// RoutingExecutor dispatches to the correct backend by manifest executor type.
type RoutingExecutor struct {
	composio    *ComposioExecutor
	composioMCP *ComposioMCPExecutor
	builtin     *BuiltinExecutor
	stub        *StubExecutor
}

// NewRoutingExecutor builds an executor that prefers composio when available.
func NewRoutingExecutor(composio *ComposioExecutor, composioMCP *ComposioMCPExecutor, builtinExec *BuiltinExecutor, stub *StubExecutor) *RoutingExecutor {
	return &RoutingExecutor{composio: composio, composioMCP: composioMCP, builtin: builtinExec, stub: stub}
}

// Execute routes execution based on manifest executor type.
func (r *RoutingExecutor) Execute(ctx context.Context, userID string, p *plugin.Plugin, args map[string]any) (map[string]any, error) {
	switch p.Manifest.Executor.Type {
	case plugin.ExecutorTypeComposio:
		if r.composio != nil {
			return r.composio.Execute(ctx, userID, p, args)
		}
		return r.stub.Execute(ctx, userID, p, args)
	case plugin.ExecutorTypeComposioMCP:
		if r.composioMCP != nil {
			return r.composioMCP.Execute(ctx, userID, p, args)
		}
		return r.stub.Execute(ctx, userID, p, args)
	case plugin.ExecutorTypeBuiltin:
		if r.builtin != nil {
			return r.builtin.Execute(ctx, userID, p, args)
		}
		return r.stub.Execute(ctx, userID, p, args)
	default:
		return nil, fmt.Errorf("unsupported executor type %q", p.Manifest.Executor.Type)
	}
}
