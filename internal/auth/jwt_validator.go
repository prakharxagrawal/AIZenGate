// Package auth provides authentication primitives for the ZenGate AI Gateway.
package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator implements TokenValidator using JSON Web Tokens.
type JWTValidator struct {
	secretKey []byte
	logger    *slog.Logger
	blacklist Blacklist
}

// NewJWTValidator creates a new instance of JWTValidator.
func NewJWTValidator(secret string, logger *slog.Logger, blacklist Blacklist) *JWTValidator {
	return &JWTValidator{
		secretKey: []byte(secret),
		logger:    logger,
		blacklist: blacklist,
	}
}

func (v *JWTValidator) Validate(ctx context.Context, tokenStr string) (*UserIdentity, error) {
	// 1. Check if token is blacklisted (revoked)
	if v.blacklist != nil {
		isRevoked, err := v.blacklist.IsRevoked(ctx, tokenStr)
		if err != nil {
			v.logger.Error("blacklist check failed", "error", err)
			return nil, fmt.Errorf("internal validation error")
		}
		if isRevoked {
			return nil, ErrInvalidToken
		}
	}

	// 2. Parse and validate JWT
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.secretKey, nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	// 3. Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("%w: invalid claims format", ErrInvalidToken)
	}

	return v.mapClaimsToIdentity(claims)
}

func (v *JWTValidator) mapClaimsToIdentity(claims jwt.MapClaims) (*UserIdentity, error) {
	userID, ok1 := claims["sub"].(string)
	tenantID, ok2 := claims["tid"].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%w: missing required claims", ErrInvalidToken)
	}

	var roles []string
	if r, ok := claims["roles"].([]interface{}); ok {
		for _, role := range r {
			if s, ok := role.(string); ok {
				roles = append(roles, s)
			}
		}
	}

	return &UserIdentity{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    roles,
	}, nil
}