package llm

import (
	"context"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

// ConversationTurn is one message in recent conversation history.
type ConversationTurn struct {
	Role    string
	Content string
}

// PluginCandidate is the plugin context passed to the classifier.
type PluginCandidate struct {
	Slug        string
	Name        string
	Description string
	Triggers    []string
	Arguments   []plugin.ManifestArgument
}

// ClassifyRequest is the input for intent classification.
type ClassifyRequest struct {
	Text           string
	Plugins        []PluginCandidate
	RecentMessages []ConversationTurn
}

// ClassifyResult is the classifier output.
type ClassifyResult struct {
	Matched    bool
	PluginSlug string
	Arguments  map[string]any
}

// Classifier picks a plugin and extracts arguments from user text.
type Classifier interface {
	Classify(ctx context.Context, req ClassifyRequest) (*ClassifyResult, error)
}
