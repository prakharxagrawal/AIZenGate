package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all gateway configuration.
// Currently loaded from environment variables.
// Phase 3 will add etcd-backed dynamic configuration.
type Config struct {
	// Server
	Version string
	Port    int
	Env     string // "development", "staging", "production"

	// Upstream
	UpstreamURL string

	// Timeouts
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	ProxyTimeout    time.Duration

	// CORS
	CORSAllowedOrigins []string
	CORSAllowedMethods []string
	CORSAllowedHeaders []string

	// Rate Limiting (Phase 2 — placeholders)
	RateLimitEnabled bool
	RedisURL         string

	// etcd (Phase 2 — placeholder)
	EtcdEndpoints []string

	// AI Brain (Phase 3 — placeholders)
	AIEnabled         bool
	DeepSeekAPIKey    string
	DeepSeekBaseURL   string
	GeminiAPIKey      string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Version: getEnvOrDefault("ZENGATE_VERSION", "0.1.0"),
		Port:    getEnvOrDefaultInt("ZENGATE_PORT", 8080),
		Env:     getEnvOrDefault("ZENGATE_ENV", "development"),

		UpstreamURL: getEnvOrDefault("ZENGATE_UPSTREAM_URL", "http://localhost:9090"),

		ReadTimeout:     getEnvOrDefaultDuration("ZENGATE_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    getEnvOrDefaultDuration("ZENGATE_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:     getEnvOrDefaultDuration("ZENGATE_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout: getEnvOrDefaultDuration("ZENGATE_SHUTDOWN_TIMEOUT", 30*time.Second),
		ProxyTimeout:    getEnvOrDefaultDuration("ZENGATE_PROXY_TIMEOUT", 10*time.Second),

		CORSAllowedOrigins: getEnvOrDefaultSlice("ZENGATE_CORS_ORIGINS", []string{"*"}),
		CORSAllowedMethods: getEnvOrDefaultSlice("ZENGATE_CORS_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}),
		CORSAllowedHeaders: getEnvOrDefaultSlice("ZENGATE_CORS_HEADERS", []string{"Content-Type", "Authorization", "X-API-Key", "X-Request-Id"}),

		RateLimitEnabled: getEnvOrDefaultBool("ZENGATE_RATELIMIT_ENABLED", false),
		RedisURL:         getEnvOrDefault("ZENGATE_REDIS_URL", "redis://localhost:6379"),

		EtcdEndpoints: getEnvOrDefaultSlice("ZENGATE_ETCD_ENDPOINTS", []string{"localhost:2379"}),

		AIEnabled:       getEnvOrDefaultBool("ZENGATE_AI_ENABLED", false),
		DeepSeekAPIKey:  getEnvOrDefault("DEEPSEEK_API_KEY", ""),
		DeepSeekBaseURL: getEnvOrDefault("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		GeminiAPIKey:    getEnvOrDefault("GEMINI_API_KEY", ""),
	}

	// Validate required fields
	if cfg.UpstreamURL == "" {
		return nil, fmt.Errorf("ZENGATE_UPSTREAM_URL is required")
	}

	return cfg, nil
}

// --- Helper functions ---

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvOrDefaultInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvOrDefaultBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvOrDefaultDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func getEnvOrDefaultSlice(key string, defaultVal []string) []string {
	if val := os.Getenv(key); val != "" {
		return strings.Split(val, ",")
	}
	return defaultVal
}
