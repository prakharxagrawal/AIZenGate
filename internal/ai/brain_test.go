package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/zengate-ai/zengate/internal/config"
)

func TestBrain_MockCompletion_RateLimit(t *testing.T) {
	// Configure without API keys to trigger mock mode
	cfg := &config.Config{
		DeepSeekAPIKey: "",
		GeminiAPIKey:   "",
	}

	brain := NewBrain(cfg)

	ctx := context.Background()

	// Query with a rate limit prompt
	resp, err := brain.GenerateCompletion(ctx, "system prompt", "translate rate limit config to 100/min")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(resp, "mock-policy-1") {
		t.Errorf("expected mock response to contain 'mock-policy-1', got %q", resp)
	}
}

func TestBrain_MockCompletion_Anomaly(t *testing.T) {
	cfg := &config.Config{
		DeepSeekAPIKey: "",
		GeminiAPIKey:   "",
	}

	brain := NewBrain(cfg)

	ctx := context.Background()

	// Query with anomaly prompt
	resp, err := brain.GenerateCompletion(ctx, "system prompt", "validate anomalous spikes in TPS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(resp, "detected traffic spike") {
		t.Errorf("expected mock response to contain 'detected traffic spike', got %q", resp)
	}
}
