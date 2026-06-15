// Package errors provides custom error types for the ZenGate AI system.
package errors

import "fmt"

// ErrInvalidInput represents a validation error.
type ErrInvalidInput struct {
	Message string
}

func (e *ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid input: %s", e.Message)
}

func NewErrInvalidInput(msg string) *ErrInvalidInput {
	return &ErrInvalidInput{Message: msg}
}