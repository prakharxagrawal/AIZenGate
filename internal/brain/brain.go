// Package brain handles the orchestration of AI-driven self-healing logic.
package brain

import (
	"context"
	"fmt"
)

// SelfHealer orchestrates the diagnostic and repair process.
type SelfHealer struct {
	llm LLMClient
}

// NewSelfHealer creates a new instance of the self-healer with the provided LLM client.
func NewSelfHealer(llm LLMClient) *SelfHealer {
	return &SelfHealer{llm: llm}
}

// DiagnoseAndRepair processes logs and returns a repair plan.
func (s *SelfHealer) DiagnoseAndRepair(ctx context.Context, logs string) (string, error) {
	prompt := fmt.Sprintf("Analyze these logs and provide a repair plan: %s", logs)
	plan, err := s.llm.GenerateContent(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("self-healer failed to generate plan: %w", err)
	}
	return plan, nil
}