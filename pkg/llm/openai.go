package llm

import (
	"context"
	"fmt"
)

// OpenAIClassifier is a stub OpenAI-backed classifier. It returns no match until
// a real implementation is wired; tests use MockClassifier instead.
type OpenAIClassifier struct {
	APIKey string
	Model  string
}

// Classify currently returns no plugin match (stub for production wiring).
func (c *OpenAIClassifier) Classify(_ context.Context, _ ClassifyRequest) (*ClassifyResult, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("llm: openai api key not configured")
	}
	return &ClassifyResult{Matched: false}, nil
}

// NewClassifier builds a Classifier from provider configuration.
func NewClassifier(provider, apiKey, model string) Classifier {
	switch provider {
	case "mock":
		return NewMockClassifier()
	case "openai":
		return &OpenAIClassifier{APIKey: apiKey, Model: model}
	default:
		return NewMockClassifier()
	}
}
