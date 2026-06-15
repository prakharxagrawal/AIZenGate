// Package orchestrator defines the core contracts for the ZenGate AI system.
package orchestrator

import "context"

// Result represents the successful output of a task execution.
type Result struct {
	Output []byte
}

// TaskPayload represents the input data for an AI agent task.
type TaskPayload struct {
	AgentID string
	Data    []byte
}

// TaskProcessor defines the contract for AI agent execution units.
type TaskProcessor interface {
	Execute(ctx context.Context, task TaskPayload) (Result, error)
}

// StateStore defines the persistence layer for agent memory/context.
type StateStore interface {
	SaveState(ctx context.Context, agentID string, state []byte) error
	GetState(ctx context.Context, agentID string) ([]byte, error)
}