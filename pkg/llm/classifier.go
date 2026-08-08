package llm

import (
	"context"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

// PluginCandidate is the plugin context passed to the classifier.
type PluginCandidate struct {
	Slug      string
	Name      string
	Triggers  []string
	Arguments []plugin.ManifestArgument
}

// ClassifyRequest is the input for intent classification.
type ClassifyRequest struct {
	Text    string
	Plugins []PluginCandidate
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
