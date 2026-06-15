// Package store provides the persistence layer interfaces for ZenGate AI.
package store

import (
	"context"
)

// StateStore defines the persistence layer for agent memory/context.
// Implementations must ensure thread-safety and context awareness.
type StateStore interface {
	SaveState(ctx context.Context, agentID string, state []byte) error
	GetState(ctx context.Context, agentID string) ([]byte, error)
}