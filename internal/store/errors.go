// Package store provides persistence abstractions for ZenGate AI.
package store

import "errors"

var (
	ErrTransient = errors.New("transient error: operation may be retried")
	ErrFatal     = errors.New("fatal error: operation cannot be completed")
	ErrNotFound  = errors.New("state not found")
)