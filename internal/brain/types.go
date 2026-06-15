// Package brain provides the reasoning engine for the self-healer, 
// interfacing with Google Gemini LLMs to analyze system failures and suggest fixes.
package brain

import "context"

// LLMRequest defines the input for the brain's reasoning engine.
type LLMRequest struct {
	Prompt      string
	Temperature float64
	MaxTokens   int
}

// LLMResponse defines the structured output from the brain.
type LLMResponse struct {
	Content string
	Usage   TokenUsage
}

// TokenUsage tracks the consumption of tokens for the request.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// LLMClient defines the contract for interacting with Gemini.
type LLMClient interface {
	// GenerateContent sends a prompt to the configured Gemini model.
	GenerateContent(ctx context.Context, req LLMRequest) (*LLMResponse, error)

	// GetModelVersion returns the current active model ID.
	GetModelVersion() string
}

// Config holds the brain's operational parameters.
type Config struct {
	APIKey    string `envconfig:"GEMINI_API_KEY" required:"true"`
	ModelID   string `envconfig:"GEMINI_MODEL_ID" default:"gemini-1.5-flash"`
	TimeoutMS int    `envconfig:"GEMINI_TIMEOUT_MS" default:"30000"`
}