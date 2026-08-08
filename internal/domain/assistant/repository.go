package assistant

import (
	"context"

	"github.com/cymonevo/go_template/pkg/store"
)

// SessionRepository persists assistant sessions.
type SessionRepository interface {
	store.Store[Session]
}

// MessageRepository persists assistant messages.
type MessageRepository interface {
	store.Store[Message]
	FindBySessionID(ctx context.Context, sessionID string) ([]Message, error)
}

type sessionRepository struct {
	store.Store[Session]
}

type messageRepository struct {
	store.Store[Message]
}

// NewSessionRepository wraps a store as a SessionRepository.
func NewSessionRepository(s store.Store[Session]) SessionRepository {
	return &sessionRepository{Store: s}
}

// NewMessageRepository wraps a store as a MessageRepository.
func NewMessageRepository(s store.Store[Message]) MessageRepository {
	return &messageRepository{Store: s}
}

// FindBySessionID returns messages for a session ordered oldest first.
func (r *messageRepository) FindBySessionID(ctx context.Context, sessionID string) ([]Message, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("session_id", sessionID).
		OrderBy("created_at", false))
}
