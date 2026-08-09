package assistant

import (
	"time"
)

const (
	TableNameSessions = "assistant_sessions"
	TableNameMessages = "assistant_messages"
)

// SessionStatus tracks whether a conversation is ongoing or finished.
type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
)

// MessageRole identifies who produced a transcript line.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
)

// PendingAction stores in-flight plugin invocation state between turns.
type PendingAction struct {
	PluginSlug           string         `json:"plugin_slug"`
	PluginID             string         `json:"plugin_id"`
	InstallID            string         `json:"install_id"`
	Arguments            map[string]any `json:"arguments"`
	MissingArgument      string         `json:"missing_argument,omitempty"`
	AwaitingConfirmation bool           `json:"awaiting_confirmation,omitempty"`
	ComposioSessionID    string         `json:"composio_session_id,omitempty"`
	ComposioPendingKind  string         `json:"composio_pending_kind,omitempty"`
	ComposioPrompt       string         `json:"composio_prompt,omitempty"`
}

// Session is a multi-turn assistant conversation owned by a user.
type Session struct {
	ID            string         `json:"id" db:"id" bson:"_id"`
	UserID        string         `json:"-" db:"user_id" bson:"user_id"`
	Status        SessionStatus  `json:"status" db:"status" bson:"status"`
	PendingAction *PendingAction `json:"pending_action,omitempty" db:"pending_action" bson:"pending_action,omitempty"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at" bson:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at" bson:"updated_at"`
}

// Message is a single line in a session transcript.
type Message struct {
	ID        string         `json:"id" db:"id" bson:"_id"`
	SessionID string         `json:"-" db:"session_id" bson:"session_id"`
	Role      MessageRole    `json:"role" db:"role" bson:"role"`
	Content   string         `json:"content" db:"content" bson:"content"`
	Metadata  map[string]any `json:"metadata,omitempty" db:"metadata" bson:"metadata"`
	CreatedAt time.Time      `json:"created_at" db:"created_at" bson:"created_at"`
}
