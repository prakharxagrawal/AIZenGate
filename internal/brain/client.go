// Package brain defines client interfaces for external dependencies.
package brain

import "context"

// LLMClient defines the interface for interacting with AI models.
type LLMClient interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}