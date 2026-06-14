package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ContextKey string

const (
	ClientIDCtxKey   ContextKey = "client_id"
	ClientTierCtxKey ContextKey = "client_tier"
)

// JWTMiddleware validates JSON Web Tokens and binds claims to request context.
type JWTMiddleware struct {
	secretKey []byte
}

// NewJWTMiddleware creates a new authentication middleware.
func NewJWTMiddleware(secret string) *JWTMiddleware {
	if secret == "" {
		secret = "zengate-default-jwt-secret-key-change-in-production"
	}
	return &JWTMiddleware{secretKey: []byte(secret)}
}

// Handler intercepts requests, extracts & validates Bearer tokens, and sets claims.
func (m *JWTMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// No token: set anonymous context and proceed (let proxy or rate limiter handle anonymous policy)
			ctx := context.WithValue(r.Context(), ClientIDCtxKey, "anonymous")
			ctx = context.WithValue(ctx, ClientTierCtxKey, "anonymous")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "invalid_authorization",
				"message": "Authorization header must be in format 'Bearer <token>'",
			})
			return
		}

		tokenStr := parts[1]
		claims := &jwt.MapClaims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return m.secretKey, nil
		})

		if err != nil || !token.Valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "invalid_token",
				"message": "The token is expired, corrupted, or has an invalid signature",
			})
			return
		}

		// Extract claims
		var clientID string
		var tier string

		if sub, ok := (*claims)["sub"].(string); ok {
			clientID = sub
		} else {
			clientID = "unknown"
		}

		if t, ok := (*claims)["tier"].(string); ok {
			tier = t
		} else {
			tier = "basic"
		}

		// Inject into context
		ctx := context.WithValue(r.Context(), ClientIDCtxKey, clientID)
		ctx = context.WithValue(ctx, ClientTierCtxKey, tier)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateMockToken creates a mock JWT token for testing purposes.
func (m *JWTMiddleware) GenerateMockToken(clientID, tier string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  clientID,
		"tier": tier,
	})
	return token.SignedString(m.secretKey)
}
