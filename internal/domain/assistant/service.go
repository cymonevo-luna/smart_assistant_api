package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/assistant/builtin"
	assistantsettings "github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/calendar/availability"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	userplugin "github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/llm"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

const (
	noActionText                = "I heard you, but no action is configured."
	noMatchText                 = "I don't know how to do that yet."
	classifierUnavailableText   = "I'm having trouble understanding that right now. Please try again in a moment."
	actionReasonSetupIncomplete = "setup_incomplete"
	actionReasonPluginDisabled  = "plugin_disabled"
)

// installedPluginState categorises why an install may or may not be usable.
type installedPluginState string

const (
	pluginStateEligible        installedPluginState = "eligible"
	pluginStateSetupIncomplete installedPluginState = "setup_incomplete"
	pluginStateDisabled        installedPluginState = "disabled"
	pluginStateCatalogMissing  installedPluginState = "catalog_missing"
)

// eligiblePlugin pairs an install row with its catalog entry.
type eligiblePlugin struct {
	install *userplugin.UserPlugin
	catalog *plugin.Plugin
}

// pluginInstallState pairs an install with catalog metadata and eligibility state.
type pluginInstallState struct {
	install *userplugin.UserPlugin
	catalog *plugin.Plugin
	state   installedPluginState
}

// Service orchestrates assistant sessions and message processing.
type Service struct {
	sessions     SessionRepository
	messages     MessageRepository
	settings     *assistantsettings.Service
	userPlugins  userplugin.Repository
	pluginRepo   plugin.Repository
	classifier   llm.Classifier
	executor     PluginExecutor
	availability availability.AvailabilityService
	log          logger.Logger
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
	availabilitySvc availability.AvailabilityService,
	log logger.Logger,
) *Service {
	return &Service{
		sessions:     sessions,
		messages:     messages,
		settings:     settings,
		userPlugins:  userPlugins,
		pluginRepo:   pluginRepo,
		classifier:   classifier,
		executor:     executor,
		availability: availabilitySvc,
		log:          log,
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
		if isGoogleCalendarMeetPlugin(eligible.catalog) {
			return s.handleGoogleCalendarMeetPending(ctx, userID, eligible, pending, text)
		}
		if isLocationReminderSlug(pending.PluginSlug) {
			if applyLocationReminderFollowUp(pending.MissingArgument, text, pending.Arguments) {
				pending.MissingArgument = ""
			} else {
				pending.Arguments[pending.MissingArgument] = strings.TrimSpace(text)
				pending.MissingArgument = ""
			}
		} else if isReminderPlugin(eligible.catalog) {
			ok, prompt := applyReminderFollowUp(pending.MissingArgument, text, pending.Arguments)
			if !ok {
				return Reply{
						Type: ReplyTypeFollowUp,
						Text: prompt,
						Action: &ActionInfo{
							PluginSlug: eligible.catalog.Slug,
							Status:     ActionStatusPending,
						},
					}, &PendingAction{
						PluginSlug:      pending.PluginSlug,
						PluginID:        pending.PluginID,
						InstallID:       pending.InstallID,
						Arguments:       pending.Arguments,
						MissingArgument: "remind_at",
					}, SessionStatusActive, nil
			}
			pending.MissingArgument = ""
		} else {
			pending.Arguments[pending.MissingArgument] = strings.TrimSpace(text)
			pending.MissingArgument = ""
		}
	}

	return s.advancePlugin(ctx, userID, eligible, pending.Arguments, text)
}

func (s *Service) classifyAndExecute(ctx context.Context, userID, text string) (Reply, *PendingAction, SessionStatus, error) {
	states, err := s.listInstalledPluginStates(ctx, userID)
	if err != nil {
		return Reply{}, nil, SessionStatusActive, err
	}
	if len(states) == 0 {
		return Reply{Type: ReplyTypeText, Text: noActionText}, nil, SessionStatusActive, nil
	}

	eligible := make([]eligiblePlugin, 0, len(states))
	for _, st := range states {
		if st.state == pluginStateEligible {
			eligible = append(eligible, eligiblePlugin{install: st.install, catalog: st.catalog})
		}
	}

	if len(eligible) == 0 {
		for _, st := range states {
			if st.state == pluginStateSetupIncomplete {
				return blockedPluginReply(st, actionReasonSetupIncomplete), nil, SessionStatusActive, nil
			}
		}
		for _, st := range states {
			if st.state == pluginStateDisabled {
				return blockedPluginReply(st, actionReasonPluginDisabled), nil, SessionStatusActive, nil
			}
		}
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
		return Reply{Type: ReplyTypeText, Text: classifierUnavailableText}, nil, SessionStatusActive, nil
	}
	if result == nil || !result.Matched || result.PluginSlug == "" {
		return Reply{Type: ReplyTypeText, Text: noMatchText}, nil, SessionStatusActive, nil
	}

	var matched *eligiblePlugin
	for i := range eligible {
		if eligible[i].catalog.Slug == result.PluginSlug {
			matched = &eligible[i]
			break
		}
	}
	if matched == nil {
		return Reply{Type: ReplyTypeText, Text: noMatchText}, nil, SessionStatusActive, nil
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

		if operation == "create" {
			if msg := stringArgValue(args, "message"); msg != "" && stringArgValue(args, "remind_at") == "" {
				splitMsg, clause := splitMessageAndRemindAt(msg)
				if clause != "" {
					args["message"] = splitMsg
					args["remind_at"] = clause
				}
			}
			if prompt, ok := normalizeReminderArgs(args); !ok {
				pending := &PendingAction{
					PluginSlug:      eligible.catalog.Slug,
					PluginID:        eligible.catalog.ID,
					InstallID:       eligible.install.ID,
					Arguments:       args,
					MissingArgument: "remind_at",
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

	if isGoogleCalendarMeetPlugin(eligible.catalog) {
		return s.advanceGoogleCalendarMeet(ctx, userID, eligible, args, text)
	}

	if isLocationReminderPlugin(eligible.catalog) {
		inferLocationReminderFromText(text, args)

		missingName, prompt := firstMissingLocationReminderArgument(args)
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

		settings, err := s.settings.GetOrCreate(ctx, userID)
		if err != nil {
			return Reply{}, nil, SessionStatusActive, err
		}
		args["radius_meters"] = settings.LocationReminderThresholdMeters

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
	execArgs := stripInternalArgs(cloneArgs(args))
	execArgs["install_id"] = eligible.install.ID

	result, err := s.executor.Execute(ctx, userID, eligible.catalog, execArgs)
	if err != nil {
		errText := builtin.ExecutorErrorText(err)
		if isLocationReminderPlugin(eligible.catalog) {
			errText = builtin.LocationReminderExecutorErrorText(err)
		}
		return Reply{
			Type: ReplyTypeActionResult,
			Text: errText,
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusFailed,
			},
		}, nil, SessionStatusActive, nil
	}

	replyText := fmt.Sprintf("Done. %s completed successfully.", eligible.catalog.Name)
	var clientPayload map[string]any
	if result != nil {
		if custom, ok := result["reply_text"].(string); ok && strings.TrimSpace(custom) != "" {
			replyText = custom
		}
		if cp, ok := result["client_payload"].(map[string]any); ok {
			clientPayload = cp
		}
	}

	action := &ActionInfo{
		PluginSlug: eligible.catalog.Slug,
		Status:     ActionStatusSuccess,
		Payload:    clientPayload,
	}

	return Reply{
		Type:   ReplyTypeActionResult,
		Text:   replyText,
		Action: action,
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

func isLocationReminderPlugin(p *plugin.Plugin) bool {
	if p.Slug == builtin.LocationReminderSlug {
		return true
	}
	adapter, _ := p.Manifest.Executor.Config["builtin_adapter"].(string)
	return adapter == builtin.AdapterLocationReminder
}

func isLocationReminderSlug(slug string) bool {
	return slug == builtin.LocationReminderSlug
}

func (s *Service) listInstalledPluginStates(ctx context.Context, userID string) ([]pluginInstallState, error) {
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

	out := make([]pluginInstallState, 0, len(installs))
	for i := range installs {
		inst := &installs[i]
		cat, ok := byID[inst.PluginID]
		if !ok {
			out = append(out, pluginInstallState{
				install: inst,
				state:   pluginStateCatalogMissing,
			})
			continue
		}
		if !inst.Enabled {
			out = append(out, pluginInstallState{
				install: inst,
				catalog: cat,
				state:   pluginStateDisabled,
			})
			continue
		}
		if cat.Manifest.RequiredSetup && inst.SetupStatus != userplugin.SetupStatusCompleted {
			out = append(out, pluginInstallState{
				install: inst,
				catalog: cat,
				state:   pluginStateSetupIncomplete,
			})
			continue
		}
		out = append(out, pluginInstallState{
			install: inst,
			catalog: cat,
			state:   pluginStateEligible,
		})
	}
	return out, nil
}

func blockedPluginReply(state pluginInstallState, reason string) Reply {
	name := "This plugin"
	slug := ""
	if state.catalog != nil {
		name = state.catalog.Name
		slug = state.catalog.Slug
	}

	var text string
	switch reason {
	case actionReasonSetupIncomplete:
		text = fmt.Sprintf("%s is installed but still needs setup before I can help with that.", name)
	case actionReasonPluginDisabled:
		text = fmt.Sprintf("%s is installed but disabled. Please enable it to use this feature.", name)
	default:
		text = noActionText
	}

	return Reply{
		Type: ReplyTypeText,
		Text: text,
		Action: &ActionInfo{
			PluginSlug: slug,
			Status:     ActionStatusPending,
			Payload: map[string]any{
				"reason":      reason,
				"install_id":  state.install.ID,
				"plugin_slug": slug,
			},
		},
	}
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
