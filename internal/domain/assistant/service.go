package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/assistant/builtin"
	assistantsettings "github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	userplugin "github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/llm"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

const noActionText = "I heard you, but no action is configured."

// eligiblePlugin pairs an install row with its catalog entry.
type eligiblePlugin struct {
	install *userplugin.UserPlugin
	catalog *plugin.Plugin
}

// Service orchestrates assistant sessions and message processing.
type Service struct {
	sessions    SessionRepository
	messages    MessageRepository
	settings    *assistantsettings.Service
	userPlugins userplugin.Repository
	pluginRepo  plugin.Repository
	classifier  llm.Classifier
	executor    PluginExecutor
	log         logger.Logger
}

// NewService constructs an assistant Service.
func NewService(
	sessions SessionRepository,
	messages MessageRepository,
	settings *assistantsettings.Service,
	userPlugins userplugin.Repository,
	pluginRepo plugin.Repository,
	classifier llm.Classifier,
	executor PluginExecutor,
	log logger.Logger,
) *Service {
	return &Service{
		sessions:    sessions,
		messages:    messages,
		settings:    settings,
		userPlugins: userPlugins,
		pluginRepo:  pluginRepo,
		classifier:  classifier,
		executor:    executor,
		log:         log,
	}
}

// CreateSession starts a new active conversation for the caller.
func (s *Service) CreateSession(ctx context.Context, userID string) (*CreateSessionResponse, error) {
	now := time.Now().UTC()
	session := &Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		Status:    SessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, response.NewInternal("failed to create session").Wrap(err)
	}
	return &CreateSessionResponse{SessionID: session.ID}, nil
}

// ProcessMessage handles a user utterance within a session.
func (s *Service) ProcessMessage(ctx context.Context, userID, sessionID string, in ProcessMessageInput) (*ProcessMessageResponse, error) {
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != SessionStatusActive {
		return nil, response.NewBadRequest("session is not active")
	}

	settings, err := s.settings.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}

	text := stripWakeWord(in.Text, settings.WakeWord, in.Source)
	if strings.TrimSpace(text) == "" {
		return nil, response.NewValidation(map[string]string{
			"text": "must not be empty after wake word removal",
		})
	}

	if err := s.persistMessage(ctx, sessionID, MessageRoleUser, text, nil); err != nil {
		return nil, err
	}

	reply, pending, sessionStatus, err := s.orchestrate(ctx, userID, session, text)
	if err != nil {
		return nil, err
	}

	session.PendingAction = pending
	session.Status = sessionStatus
	session.UpdatedAt = time.Now().UTC()
	if err := s.sessions.Update(ctx, sessionID, session); err != nil {
		return nil, response.NewInternal("failed to update session").Wrap(err)
	}

	meta := map[string]any{"reply_type": reply.Type}
	if reply.Action != nil {
		meta["action"] = reply.Action
	}
	if err := s.persistMessage(ctx, sessionID, MessageRoleAssistant, reply.Text, meta); err != nil {
		return nil, err
	}

	return &ProcessMessageResponse{
		SessionID:     sessionID,
		Reply:         reply,
		SessionStatus: sessionStatus,
	}, nil
}

// ListMessages returns transcript history for an owned session.
func (s *Service) ListMessages(ctx context.Context, userID, sessionID string) (*MessageHistoryResponse, error) {
	if _, err := s.loadOwnedSession(ctx, userID, sessionID); err != nil {
		return nil, err
	}
	msgs, err := s.messages.FindBySessionID(ctx, sessionID)
	if err != nil {
		return nil, response.NewInternal("failed to load messages").Wrap(err)
	}
	return &MessageHistoryResponse{Messages: ToMessageHistory(msgs)}, nil
}

func (s *Service) loadOwnedSession(ctx context.Context, userID, sessionID string) (*Session, error) {
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("session not found")
		}
		return nil, response.NewInternal("failed to load session").Wrap(err)
	}
	if session.UserID != userID {
		return nil, response.NewForbidden("cannot access another user's session")
	}
	return session, nil
}

func (s *Service) persistMessage(ctx context.Context, sessionID string, role MessageRole, content string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	msg := &Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.messages.Create(ctx, msg); err != nil {
		return response.NewInternal("failed to persist message").Wrap(err)
	}
	return nil
}

func (s *Service) orchestrate(ctx context.Context, userID string, session *Session, text string) (Reply, *PendingAction, SessionStatus, error) {
	if session.PendingAction != nil {
		return s.handlePendingAction(ctx, userID, session, text)
	}
	return s.classifyAndExecute(ctx, userID, text)
}

func (s *Service) handlePendingAction(ctx context.Context, userID string, session *Session, text string) (Reply, *PendingAction, SessionStatus, error) {
	pending := session.PendingAction
	eligible, err := s.loadEligiblePlugin(ctx, userID, pending.PluginID, pending.InstallID)
	if err != nil {
		return Reply{}, nil, SessionStatusActive, err
	}

	if pending.AwaitingConfirmation {
		if isConfirmationYes(text) {
			return s.executePlugin(ctx, userID, eligible, pending.Arguments)
		}
		if isConfirmationNo(text) {
			return Reply{
				Type: ReplyTypeText,
				Text: "Okay, I won't do that.",
			}, nil, SessionStatusActive, nil
		}
		return Reply{
			Type: ReplyTypeConfirmation,
			Text: confirmationPrompt(eligible.catalog.Name, pending.Arguments),
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusPending,
			},
		}, pending, SessionStatusActive, nil
	}

	if pending.MissingArgument != "" {
		if pending.Arguments == nil {
			pending.Arguments = map[string]any{}
		}
		pending.Arguments[pending.MissingArgument] = strings.TrimSpace(text)
		pending.MissingArgument = ""
	}

	return s.advancePlugin(ctx, userID, eligible, pending.Arguments, text)
}

func (s *Service) classifyAndExecute(ctx context.Context, userID, text string) (Reply, *PendingAction, SessionStatus, error) {
	eligible, err := s.listEligiblePlugins(ctx, userID)
	if err != nil {
		return Reply{}, nil, SessionStatusActive, err
	}
	if len(eligible) == 0 {
		return Reply{Type: ReplyTypeText, Text: noActionText}, nil, SessionStatusActive, nil
	}

	candidates := make([]llm.PluginCandidate, 0, len(eligible))
	for _, e := range eligible {
		candidates = append(candidates, llm.PluginCandidate{
			Slug:      e.catalog.Slug,
			Name:      e.catalog.Name,
			Triggers:  e.catalog.Manifest.Triggers,
			Arguments: e.catalog.Manifest.Arguments,
		})
	}

	result, err := s.classifier.Classify(ctx, llm.ClassifyRequest{
		Text:    text,
		Plugins: candidates,
	})
	if err != nil {
		s.log.Error("intent classification failed", logger.Err(err))
		return Reply{Type: ReplyTypeText, Text: noActionText}, nil, SessionStatusActive, nil
	}
	if result == nil || !result.Matched || result.PluginSlug == "" {
		return Reply{Type: ReplyTypeText, Text: noActionText}, nil, SessionStatusActive, nil
	}

	var matched *eligiblePlugin
	for i := range eligible {
		if eligible[i].catalog.Slug == result.PluginSlug {
			matched = &eligible[i]
			break
		}
	}
	if matched == nil {
		return Reply{Type: ReplyTypeText, Text: noActionText}, nil, SessionStatusActive, nil
	}

	args := result.Arguments
	if args == nil {
		args = map[string]any{}
	}
	return s.advancePlugin(ctx, userID, *matched, args, text)
}

func (s *Service) advancePlugin(ctx context.Context, userID string, eligible eligiblePlugin, args map[string]any, text string) (Reply, *PendingAction, SessionStatus, error) {
	if args == nil {
		args = map[string]any{}
	}

	if isReminderPlugin(eligible.catalog) {
		operation := inferReminderOperation(text, args)
		args["operation"] = operation
		if operation == "list" {
			inferReminderFilter(text, args)
		}

		missingName, prompt := firstMissingReminderArgument(operation, args)
		if missingName != "" {
			pending := &PendingAction{
				PluginSlug:      eligible.catalog.Slug,
				PluginID:        eligible.catalog.ID,
				InstallID:       eligible.install.ID,
				Arguments:       args,
				MissingArgument: missingName,
			}
			return Reply{
				Type: ReplyTypeFollowUp,
				Text: prompt,
				Action: &ActionInfo{
					PluginSlug: eligible.catalog.Slug,
					Status:     ActionStatusPending,
				},
			}, pending, SessionStatusActive, nil
		}

		if reminderNeedsConfirmation(operation) {
			pending := &PendingAction{
				PluginSlug:           eligible.catalog.Slug,
				PluginID:             eligible.catalog.ID,
				InstallID:            eligible.install.ID,
				Arguments:            args,
				AwaitingConfirmation: true,
			}
			return Reply{
				Type: ReplyTypeConfirmation,
				Text: confirmationPromptReminder(operation, args),
				Action: &ActionInfo{
					PluginSlug: eligible.catalog.Slug,
					Status:     ActionStatusPending,
				},
			}, pending, SessionStatusActive, nil
		}

		return s.executePlugin(ctx, userID, eligible, args)
	}

	missingName, prompt := firstMissingArgument(eligible.catalog.Manifest, args)
	if missingName != "" {
		pending := &PendingAction{
			PluginSlug:      eligible.catalog.Slug,
			PluginID:        eligible.catalog.ID,
			InstallID:       eligible.install.ID,
			Arguments:       args,
			MissingArgument: missingName,
		}
		return Reply{
			Type: ReplyTypeFollowUp,
			Text: prompt,
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusPending,
			},
		}, pending, SessionStatusActive, nil
	}

	if eligible.catalog.Manifest.ConfirmationRequired {
		pending := &PendingAction{
			PluginSlug:           eligible.catalog.Slug,
			PluginID:             eligible.catalog.ID,
			InstallID:            eligible.install.ID,
			Arguments:            args,
			AwaitingConfirmation: true,
		}
		return Reply{
			Type: ReplyTypeConfirmation,
			Text: confirmationPrompt(eligible.catalog.Name, args),
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusPending,
			},
		}, pending, SessionStatusActive, nil
	}

	return s.executePlugin(ctx, userID, eligible, args)
}

func (s *Service) executePlugin(ctx context.Context, userID string, eligible eligiblePlugin, args map[string]any) (Reply, *PendingAction, SessionStatus, error) {
	execArgs := cloneArgs(args)
	execArgs["install_id"] = eligible.install.ID

	result, err := s.executor.Execute(ctx, userID, eligible.catalog, execArgs)
	if err != nil {
		return Reply{
			Type: ReplyTypeActionResult,
			Text: builtin.ExecutorErrorText(err),
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusFailed,
			},
		}, nil, SessionStatusActive, nil
	}

	replyText := fmt.Sprintf("Done. %s completed successfully.", eligible.catalog.Name)
	if result != nil {
		if custom, ok := result["reply_text"].(string); ok && strings.TrimSpace(custom) != "" {
			replyText = custom
		}
	}

	return Reply{
		Type: ReplyTypeActionResult,
		Text: replyText,
		Action: &ActionInfo{
			PluginSlug: eligible.catalog.Slug,
			Status:     ActionStatusSuccess,
		},
	}, nil, SessionStatusActive, nil
}

func cloneArgs(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}

func isReminderPlugin(p *plugin.Plugin) bool {
	if p.Slug == builtin.ReminderSlug {
		return true
	}
	adapter, _ := p.Manifest.Executor.Config["builtin_adapter"].(string)
	return adapter == builtin.AdapterReminder
}

func (s *Service) listEligiblePlugins(ctx context.Context, userID string) ([]eligiblePlugin, error) {
	installs, err := s.userPlugins.FindByUserID(ctx, userID)
	if err != nil {
		return nil, response.NewInternal("failed to list installed plugins").Wrap(err)
	}
	if len(installs) == 0 {
		return nil, nil
	}

	pluginIDs := make([]string, 0, len(installs))
	for i := range installs {
		pluginIDs = append(pluginIDs, installs[i].PluginID)
	}
	catalogItems, err := s.pluginRepo.Find(ctx, store.NewQuery().In("id", pluginIDs))
	if err != nil {
		return nil, response.NewInternal("failed to load plugin catalog").Wrap(err)
	}
	byID := make(map[string]*plugin.Plugin, len(catalogItems))
	for i := range catalogItems {
		byID[catalogItems[i].ID] = &catalogItems[i]
	}

	out := make([]eligiblePlugin, 0, len(installs))
	for i := range installs {
		inst := &installs[i]
		if !inst.Enabled {
			continue
		}
		cat, ok := byID[inst.PluginID]
		if !ok {
			continue
		}
		if cat.Manifest.RequiredSetup && inst.SetupStatus != userplugin.SetupStatusCompleted {
			continue
		}
		out = append(out, eligiblePlugin{install: inst, catalog: cat})
	}
	return out, nil
}

func (s *Service) loadEligiblePlugin(ctx context.Context, userID, pluginID, installID string) (eligiblePlugin, error) {
	install, err := s.userPlugins.FindByID(ctx, installID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return eligiblePlugin{}, response.NewNotFound("installed plugin not found")
		}
		return eligiblePlugin{}, response.NewInternal("failed to load installed plugin").Wrap(err)
	}
	if install.UserID != userID || install.PluginID != pluginID {
		return eligiblePlugin{}, response.NewForbidden("plugin install mismatch")
	}
	if !install.Enabled {
		return eligiblePlugin{}, response.NewBadRequest("plugin is disabled")
	}

	catalog, err := s.pluginRepo.FindByID(ctx, pluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return eligiblePlugin{}, response.NewNotFound("plugin not found")
		}
		return eligiblePlugin{}, response.NewInternal("failed to load plugin").Wrap(err)
	}
	if catalog.Manifest.RequiredSetup && install.SetupStatus != userplugin.SetupStatusCompleted {
		return eligiblePlugin{}, response.NewBadRequest("plugin setup is incomplete")
	}
	return eligiblePlugin{install: install, catalog: catalog}, nil
}
