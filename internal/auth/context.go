// Package auth provides context helpers for propagating user identity.
package auth

import (
	"context"
)

type contextKey string

const (
	// UserIdentityCtxKey is used to store the full UserIdentity in the request context.
	UserIdentityCtxKey contextKey = "zengate.auth.identity"
	// ClientIDCtxKey is used for quick access to the UserID (required by ratelimit middleware).
	ClientIDCtxKey contextKey = "zengate.auth.client_id"
	// ClientTierCtxKey is used for quick access to the user's tier (required by ratelimit middleware).
	ClientTierCtxKey contextKey = "zengate.auth.client_tier"
)

// FromContext retrieves the UserIdentity from the request context.
func FromContext(ctx context.Context) (*UserIdentity, bool) {
	identity, ok := ctx.Value(UserIdentityCtxKey).(*UserIdentity)
	return identity, ok
}

// WithIdentity returns a new context containing the provided UserIdentity.
func WithIdentity(ctx context.Context, identity *UserIdentity) context.Context {
	ctx = context.WithValue(ctx, UserIdentityCtxKey, identity)
	ctx = context.WithValue(ctx, ClientIDCtxKey, identity.UserID)
	ctx = context.WithValue(ctx, ClientTierCtxKey, identity.Tier)
	return ctx
}