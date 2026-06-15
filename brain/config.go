// Package brain handles configuration and initialization logic.
package brain

import (
	"errors"
	"fmt"
)

// Config holds the application configuration.
type Config struct {
	Model string
	Port  int
}

// Validate ensures the configuration is valid for production use.
func (c *Config) Validate() error {
	if c.Model == "" {
		return errors.New("configuration error: Model must be specified")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("configuration error: invalid port %d", c.Port)
	}
	return nil
}