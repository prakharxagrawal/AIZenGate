// Package brain provides interfaces and implementations for interacting with LLM backends.
package brain

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// DefaultModel is the stable, high-performance model for the free tier.
const DefaultModel = "gemini-1.5-flash"

// ModelProvider defines the contract for interacting with LLM backends.
type ModelProvider interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
	Close() error
}

// GeminiClient implements ModelProvider for Google's Gemini API.
// Note: The caller is responsible for calling Close() to prevent connection leaks.
type GeminiClient struct {
	client    *genai.Client
	modelName string
}

// NewGeminiClient initializes a new Gemini client.
func NewGeminiClient(ctx context.Context, apiKey, modelName string) (*GeminiClient, error) {
	if modelName == "" {
		modelName = DefaultModel
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gemini client: %w", err)
	}

	return &GeminiClient{
		client:    client,
		modelName: modelName,
	}, nil
}

// GenerateContent sends a prompt to the Gemini API and returns the response.
func (g *GeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if g.client == nil {
		return "", fmt.Errorf("gemini client is not initialized")
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled before request: %w", err)
	}

	model := g.client.GenerativeModel(g.modelName)
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content returned from model")
	}

	// Safely extract text from the first part
	part := resp.Candidates[0].Content.Parts[0]
	return fmt.Sprintf("%v", part), nil
}

// Close cleans up the underlying client connection.
func (g *GeminiClient) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}