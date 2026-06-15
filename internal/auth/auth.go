// Package auth provides authentication primitives for the ZenGate AI Gateway,
// including token validation and request interception middleware.
package auth

import (
	"context"
	"errors"
)

var (
	// ErrInvalidToken is returned when a token is malformed, expired, or otherwise invalid.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrMissingToken is returned when the authentication header is missing or empty.
	ErrMissingToken = errors.New("authentication token is missing")
)

// contextKey is a private type to prevent collisions in the request context.
type contextKey string

const (
	// IdentityKey is used to retrieve the UserIdentity from the request context.
	IdentityKey contextKey = "zengate.auth.identity"
)

// UserIdentity represents the authenticated entity extracted from the token.
// It contains the core identity markers used for authorization and multi-tenancy.
type UserIdentity struct {
	UserID   string   `json:"user_id"`
	Roles    []string `json:"roles"`
	TenantID string   `json:"tenant_id"`
}

// TokenValidator defines the contract for validating access tokens.
// Implementations can range from simple JWT validators to complex OAuth2 introspectors.
type TokenValidator interface {
	// Validate checks the token string and returns the associated identity.
	// It should return ErrInvalidToken if the token is not usable.
	Validate(ctx context.Context, token string) (*UserIdentity, error)
}

// FromContext retrieves the UserIdentity from the provided context.
// It returns nil if no identity is found, indicating the request was not authenticated.
func FromContext(ctx context.Context) *UserIdentity {
	if identity, ok := ctx.Value(IdentityKey).(*UserIdentity); ok {
		return identity
	}
	return nil
}