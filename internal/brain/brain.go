// Package brain manages the orchestration of AI tasks.
package brain

import (
	"context"
	"fmt"
	"log/slog"
)

// Coordinator manages task execution flow.
type Coordinator struct {
	processor TaskProcessor
	logger    *slog.Logger
}

// NewCoordinator creates a new task coordinator.
func NewCoordinator(p TaskProcessor, l *slog.Logger) *Coordinator {
	return &Coordinator{
		processor: p,
		logger:    l,
	}
}

// ProcessTask handles the orchestration of a single task.
func (c *Coordinator) ProcessTask(ctx context.Context, task TaskPayload) (Result, error) {
	c.logger.Info("processing task", "agent_id", task.AgentID)

	res, err := c.processor.Execute(ctx, task)
	if err != nil {
		c.logger.Error("failed to execute task", "agent_id", task.AgentID, "error", err)
		return Result{}, fmt.Errorf("brain: execution failed: %w", err)
	}

	return res, nil
}