package userplugin

import "time"

const (
	// TableName is the SQL table / Mongo collection name for user plugin installs.
	TableName = "user_plugins"
)

// SetupStatus tracks plugin setup progress for a user install.
type SetupStatus string

const (
	SetupStatusNotStarted SetupStatus = "not_started"
	SetupStatusInProgress SetupStatus = "in_progress"
	SetupStatusCompleted  SetupStatus = "completed"
	SetupStatusFailed     SetupStatus = "failed"
)

// InstallConfig holds non-secret per-install settings stored in UserPlugin.Config.
type InstallConfig struct {
	ConnectedToolkits      []string `json:"connected_toolkits,omitempty"`
	ConnectedAccountsCount int      `json:"connected_accounts_count,omitempty"`
}

// UserPlugin links a user to an installed catalog plugin.
type UserPlugin struct {
	ID          string         `json:"id" db:"id" bson:"_id"`
	UserID      string         `json:"-" db:"user_id" bson:"user_id"`
	PluginID    string         `json:"-" db:"plugin_id" bson:"plugin_id"`
	Enabled     bool           `json:"enabled" db:"enabled" bson:"enabled"`
	SetupStatus SetupStatus    `json:"setup_status" db:"setup_status" bson:"setup_status"`
	SetupError  *string        `json:"setup_error,omitempty" db:"setup_error" bson:"setup_error,omitempty"`
	Config      map[string]any `json:"config" db:"config" bson:"config"`
	InstalledAt time.Time      `json:"installed_at" db:"installed_at" bson:"installed_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at" bson:"updated_at"`
}
