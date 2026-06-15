// Package brain provides the reasoning engine for the self-healer.
package brain

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// LoadConfig initializes the brain configuration from environment variables.
func LoadConfig() (*Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to process brain configuration: %w", err)
	}

	return &cfg, nil
}