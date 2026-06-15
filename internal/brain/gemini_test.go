package brain

import (
	"context"
	"os"
	"testing"
)

func TestNewGeminiClient(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	client, err := NewGeminiClient(ctx, apiKey, DefaultModel)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.modelName != DefaultModel {
		t.Errorf("expected model %s, got %s", DefaultModel, client.modelName)
	}
}