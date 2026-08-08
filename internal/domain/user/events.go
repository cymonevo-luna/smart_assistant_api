package user

import "time"

// Queue topics published by the user domain. Consumers (background jobs) live
// in the worker wiring.
const (
	TopicUserCreated = "user.created"
)

// UserCreatedEvent is the payload published when a user registers. It is
// intentionally small and decoupled from the entity so the event contract can
// evolve independently of storage.
type UserCreatedEvent struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
