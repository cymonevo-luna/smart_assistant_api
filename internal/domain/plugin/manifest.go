package plugin

// SetupType drives the client setup UX for a plugin.
type SetupType string

const (
	SetupTypeNone         SetupType = "none"
	SetupTypeOAuthGoogle  SetupType = "oauth_google"
	SetupTypeOAuthGeneric SetupType = "oauth_generic"
	SetupTypeForm         SetupType = "form"
)

// ExecutorType identifies how the backend runs a plugin action.
type ExecutorType string

const (
	ExecutorTypeComposio ExecutorType = "composio"
	ExecutorTypeHTTP     ExecutorType = "http"
	ExecutorTypeBuiltin  ExecutorType = "builtin"
)

// ManifestArgument describes a slot the plugin may need filled via follow-up.
type ManifestArgument struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

// Executor describes how the backend runs the plugin action.
type Executor struct {
	Type   ExecutorType   `json:"type"`
	Config map[string]any `json:"config"`
}

// PluginManifest is the standardized plugin contract stored as JSONB.
type PluginManifest struct {
	Triggers             []string           `json:"triggers"`
	RequiredSetup        bool               `json:"required_setup"`
	SetupType            SetupType          `json:"setup_type"`
	Arguments            []ManifestArgument `json:"arguments"`
	ConfirmationRequired bool               `json:"confirmation_required"`
	Executor             Executor           `json:"executor"`
}
