// Package auth provides authentication primitives for the ZenGate AI Gateway.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// AuthMiddleware is a constructor that returns a middleware function to guard routes.
func AuthMiddleware(validator TokenValidator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractBearerToken(r)
			if err != nil {
				logger.Warn("authentication failed: missing token", "error", err)
				writeErrorResponse(w, http.StatusUnauthorized, "Authentication token is missing")
				return
			}

			identity, err := validator.Validate(r.Context(), token)
			if err != nil {
				logger.Warn("authentication failed: invalid token", "error", err)
				writeErrorResponse(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			// Enrich context with user identity for downstream handlers
			ctx := context.WithValue(r.Context(), IdentityKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", ErrMissingToken
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", fmt.Errorf("%w: invalid authorization header format", ErrMissingToken)
	}

	return parts[1], nil
}

func writeErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}