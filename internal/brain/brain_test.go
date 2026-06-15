// Package brain_test verifies the functionality of the brain package.
package brain_test

import (
	"context"
	"testing"

	"github.com/zengate-ai/zengate/internal/brain"
)

func TestGeminiClientInitialization(t *testing.T) {
	ctx := context.Background()
	// Test that the client can be initialized (mocking API key for structure)
	client, err := brain.NewGeminiClient(ctx, "test-key")
	if err != nil {
		t.Fatalf("expected no error during initialization, got %v", err)
	}

	if client == nil {
		t.Fatal("expected client to be initialized")
	}
}