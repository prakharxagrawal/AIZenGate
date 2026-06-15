// Package brain handles the configuration and initialization of the AI brain components.
package brain

import (
	"os"
)

// GetModelName retrieves the configured model from environment variables,
// defaulting to gemini-1.5-flash if not set.
func GetModelName() string {
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		return DefaultModel
	}
	return model
}