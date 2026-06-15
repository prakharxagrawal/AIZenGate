package auth

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

func TestJWTValidator_Validate(t *testing.T) {
	secret := "test-secret-key-12345"
	logger := slog.Default()
	
	// Use a real Redis client pointing to a mock or local if available, 
	// but for unit tests we can use a mock or a small local instance.
	// Here we assume a local redis for integration-style unit test or mock.
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	// Note: In a real CI environment, use a mock redis or testcontainer.
	
	v := NewJWTValidator(secret, rdb, logger)
	ctx := context.Background()

	t.Run("Valid Token", func(t *testing.T) {
		claims := &CustomClaims{
			UserID: "user-1",
			ID:     "jti-1",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		ss, _ := token.SignedString([]byte(secret))

		identity, err := v.Validate(ctx, ss)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if identity.UserID != "user-1" {
			t.Errorf("expected user-1, got %s", identity.UserID)
		}
	})

	t.Run("Expired Token", func(t *testing.T) {
		claims := &CustomClaims{
			UserID: "user-1",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		ss, _ := token.SignedString([]byte(secret))

		_, err := v.Validate(ctx, ss)
		if err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("Invalid Signing Method", func(t *testing.T) {
		// Create token with None signing method
		token := jwt.NewWithClaims(jwt.SigningMethodNone, &CustomClaims{UserID: "user-1"})
		ss, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

		_, err := v.Validate(ctx, ss)
		if err == nil {
			t.Fatal("expected error for alg: none")
		}
	})
}