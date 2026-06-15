// Package brain provides the core AI agent execution logic.
package brain

import "context"

// TaskPayload represents the input data for an AI agent task.
type TaskPayload struct {
	AgentID string
	Prompt  string
	Data    map[string]interface{}
}

// Result represents the output of an AI agent task.
type Result struct {
	Content string
	Meta    map[string]interface{}
}

// TaskProcessor defines the contract for AI agent execution units.
type TaskProcessor interface {
	Execute(ctx context.Context, task TaskPayload) (Result, error)
}