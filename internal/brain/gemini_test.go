package brain

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestGeminiClient_Execute(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewGeminiClient(logger, "test-key")

	task := TaskPayload{
		AgentID: "test-agent",
		Prompt:  "hello",
	}

	res, err := client.Execute(context.Background(), task)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res.Content == "" {
		t.Error("expected content in result")
	}
}