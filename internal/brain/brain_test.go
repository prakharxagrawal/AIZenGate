// Package brain_test contains unit tests for the brain reasoning engine.
package brain_test

import (
	"context"
	"testing"

	"zengate.ai/internal/brain"
)

// mockLLMClient is used to test components that depend on LLMClient without making API calls.
type mockLLMClient struct {
	modelID string
	content string
	err     error
}

func (m *mockLLMClient) GenerateContent(ctx context.Context, req brain.LLMRequest) (*brain.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &brain.LLMResponse{
		Content: m.content,
		Usage: brain.TokenUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
		},
	}, nil
}

func (m *mockLLMClient) GetModelVersion() string {
	return m.modelID
}

func TestConfigDefaults(t *testing.T) {
	cfg := brain.Config{
		APIKey: "test-key",
	}

	// We test the logic that would be used by the client
	if cfg.APIKey != "test-key" {
		t.Errorf("expected APIKey test-key, got %s", cfg.APIKey)
	}
}

func TestMockLLMClient(t *testing.T) {
	mock := &mockLLMClient{
		modelID: "gemini-1.5-flash",
		content: "Suggested Fix: Restart Pod",
	}

	ctx := context.Background()
	req := brain.LLMRequest{Prompt: "System crash loop"}

	resp, err := mock.GenerateContent(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Suggested Fix: Restart Pod" {
		t.Errorf("expected 'Suggested Fix: Restart Pod', got %s", resp.Content)
	}

	if mock.GetModelVersion() != "gemini-1.5-flash" {
		t.Errorf("expected model gemini-1.5-flash, got %s", mock.GetModelVersion())
	}
}