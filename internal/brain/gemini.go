// Package brain provides the interface and implementation for LLM-based decision making
// within the ZenGate AI self-healing infrastructure.
package brain

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const DefaultModel = "gemini-1.5-flash"

// ModelProvider defines the contract for interacting with LLM backends.
type ModelProvider interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// GeminiClient implements ModelProvider for Google's Gemini API.
type GeminiClient struct {
	client    *genai.Client
	modelName string
}

// NewGeminiClient initializes a new Gemini client with the specified model.
func NewGeminiClient(ctx context.Context, apiKey string) (*GeminiClient, error) {
	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = DefaultModel
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	return &GeminiClient{
		client:    client,
		modelName: modelName,
	}, nil
}

// GenerateContent sends a prompt to the configured Gemini model and returns the response.
func (g *GeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	model := g.client.GenerativeModel(g.modelName)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		slog.Error("failed to generate content from gemini", "model", g.modelName, "error", err)
		return "", fmt.Errorf("gemini generation failed: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("received empty response from gemini")
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return "", fmt.Errorf("unexpected response format from gemini")
	}

	return string(text), nil
}