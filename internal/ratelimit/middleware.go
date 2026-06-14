package ratelimit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zengate-ai/zengate/internal/auth"
	"github.com/zengate-ai/zengate/internal/controlplane"
	"github.com/zengate-ai/zengate/internal/metrics"
)

// RateLimitMiddleware enforces dynamic rate limiting based on client identity and etcd policies.
type RateLimitMiddleware struct {
	limiter  Limiter
	cpClient *controlplane.Client
	metrics  *metrics.Handler
}

// NewRateLimitMiddleware creates a new rate limiting middleware instance.
func NewRateLimitMiddleware(limiter Limiter, cpClient *controlplane.Client, metrics *metrics.Handler) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter:  limiter,
		cpClient: cpClient,
		metrics:  metrics,
	}
}

// Handler intercepts requests and applies rate limiting checks.
func (m *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client identity and tier from context (injected by JWT middleware)
		clientID, _ := r.Context().Value(auth.ClientIDCtxKey).(string)
		clientTier, _ := r.Context().Value(auth.ClientTierCtxKey).(string)

		if clientID == "" {
			clientID = "anonymous"
		}
		if clientTier == "" {
			clientTier = "anonymous"
		}

		// Find matching policy from etcd config map
		policy, found := m.cpClient.GetMatchingPolicy(r.URL.Path, r.Method, clientTier)
		
		// If no policy is found, we fall back to a default limit for the tier
		if !found {
			policy = m.getDefaultPolicy(r.URL.Path, r.Method, clientTier)
		}

		// Execute rate limit check
		window := time.Duration(policy.WindowSec) * time.Second
		allowed, err := m.limiter.Allow(ctxForRequest(r), clientID, policy.Limit, window)
		if err != nil {
			slog.Error("rate limiter execution error", "client_id", clientID, "error", err)
			// On limiter errors, fail-open to ensure service availability but log the issue
			next.ServeHTTP(w, r)
			return
		}

		if !allowed {
			slog.Warn("rate limit exceeded", "client_id", clientID, "path", r.URL.Path, "limit", policy.Limit)
			m.metrics.RecordRateLimited()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please try again later.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitMiddleware) getDefaultPolicy(path, method, tier string) controlplane.Policy {
	// Simple baseline fallback policies when etcd has no overrides
	switch tier {
	case "premium":
		return controlplane.Policy{
			ID:        "default-premium",
			Path:      "*",
			Method:    "*",
			Limit:     1000,
			WindowSec: 60,
			Tier:      "premium",
		}
	case "basic":
		return controlplane.Policy{
			ID:        "default-basic",
			Path:      "*",
			Method:    "*",
			Limit:     100,
			WindowSec: 60,
			Tier:      "basic",
		}
	default: // anonymous / free
		return controlplane.Policy{
			ID:        "default-anonymous",
			Path:      "*",
			Method:    "*",
			Limit:     20,
			WindowSec: 60,
			Tier:      "anonymous",
		}
	}
}

func ctxForRequest(r *http.Request) interface {
	context.Context
} {
	return r.Context()
}
