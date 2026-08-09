package plugincredential

import "time"

const (
	// TableName is the SQL table / Mongo collection name for plugin credentials.
	TableName = "plugin_credentials"

	ProviderGoogle   = "google"
	ProviderComposio = "composio"
)

// ComposioConnectedAccount is a snapshot of a Composio connected account stored with credentials.
type ComposioConnectedAccount struct {
	ID          string `json:"id"`
	ToolkitSlug string `json:"toolkit_slug"`
	Status      string `json:"status"`
	Alias       string `json:"alias,omitempty"`
}

// ComposioPayload is the decrypted credential payload for Composio MCP plugins.
type ComposioPayload struct {
	APIKey            string                     `json:"api_key"`
	BaseURL           string                     `json:"base_url,omitempty"`
	ConnectedAccounts []ComposioConnectedAccount `json:"connected_accounts,omitempty"`
}

// Credential stores encrypted OAuth tokens for a user plugin install.
type Credential struct {
	ID               string     `json:"-" db:"id" bson:"_id"`
	UserPluginID     string     `json:"-" db:"user_plugin_id" bson:"user_plugin_id"`
	Provider         string     `json:"-" db:"provider" bson:"provider"`
	EncryptedPayload string     `json:"-" db:"encrypted_payload" bson:"encrypted_payload"`
	ExpiresAt        *time.Time `json:"-" db:"expires_at" bson:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"-" db:"created_at" bson:"created_at"`
	UpdatedAt        time.Time  `json:"-" db:"updated_at" bson:"updated_at"`
}
