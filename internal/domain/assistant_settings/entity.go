package assistantsettings

import "time"

const (
	// TableName is the SQL table / Mongo collection name for assistant settings.
	TableName = "assistant_settings"

	// DefaultWakeWord is applied when settings are first created for a user.
	DefaultWakeWord = "Jarvis"
)

// Settings holds per-user assistant preferences.
type Settings struct {
	UserID                 string    `json:"-" db:"user_id" bson:"_id"`
	WakeWord               string    `json:"wake_word" db:"wake_word" bson:"wake_word"`
	ActiveListeningEnabled bool      `json:"active_listening_enabled" db:"active_listening_enabled" bson:"active_listening_enabled"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at" bson:"updated_at"`
}
