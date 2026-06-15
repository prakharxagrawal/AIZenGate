// Package brain defines core types for the self-healer brain.
package brain

// LLMClient is an alias for the ModelProvider interface to maintain backward compatibility.
type LLMClient interface {
	ModelProvider
}