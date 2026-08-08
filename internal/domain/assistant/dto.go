package assistant

import "time"

// MessageSource identifies how the user submitted input.
type MessageSource string

const (
	MessageSourceWakeWord MessageSource = "wake_word"
	MessageSourceButton   MessageSource = "button"
)

// ReplyType categorises assistant responses.
type ReplyType string

const (
	ReplyTypeText         ReplyType = "text"
	ReplyTypeFollowUp     ReplyType = "follow_up"
	ReplyTypeConfirmation ReplyType = "confirmation"
	ReplyTypeActionResult ReplyType = "action_result"
)

// ActionStatus tracks plugin execution outcome.
type ActionStatus string

const (
	ActionStatusPending ActionStatus = "pending"
	ActionStatusSuccess ActionStatus = "success"
	ActionStatusFailed  ActionStatus = "failed"
)

// CreateSessionResponse is returned when a new session is created.
type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
}

// ProcessMessageInput is the body for POST .../messages.
type ProcessMessageInput struct {
	Text   string        `json:"text" validate:"required"`
	Source MessageSource `json:"source" validate:"required,oneof=wake_word button"`
}

// ActionInfo describes plugin execution in a reply.
type ActionInfo struct {
	PluginSlug string       `json:"plugin_slug,omitempty"`
	Status     ActionStatus `json:"status,omitempty"`
}

// Reply is the assistant's response to a user message.
type Reply struct {
	Type   ReplyType   `json:"type"`
	Text   string      `json:"text"`
	Action *ActionInfo `json:"action,omitempty"`
}

// ProcessMessageResponse is returned after processing a user message.
type ProcessMessageResponse struct {
	SessionID     string        `json:"session_id"`
	Reply         Reply         `json:"reply"`
	SessionStatus SessionStatus `json:"session_status"`
}

// MessageHistoryItem is a transcript line for debugging/UI.
type MessageHistoryItem struct {
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	CreatedAt time.Time   `json:"created_at"`
}

// MessageHistoryResponse wraps session transcript lines.
type MessageHistoryResponse struct {
	Messages []MessageHistoryItem `json:"messages"`
}

// ToMessageHistory converts persisted messages to API items.
func ToMessageHistory(msgs []Message) []MessageHistoryItem {
	out := make([]MessageHistoryItem, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == MessageRoleSystem {
			continue
		}
		out = append(out, MessageHistoryItem{
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}
	return out
}
