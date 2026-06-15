// Package brain_test verifies the functionality of the AI brain components.
package brain_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/zengate-ai/zengate/brain"
)

func TestNewGeminiClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiKey := "test-key"

	client := brain.NewGeminiClient(logger, apiKey)
	if client == nil {
		t.Fatal("expected client to be initialized, got nil")
	}
}