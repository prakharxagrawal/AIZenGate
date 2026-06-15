# Technical Specification: Request Authentication Guard

## Overview
The goal is to implement a mandatory authentication layer within the ZenGate AI Gateway. This layer acts as a "Guard" that intercepts all incoming HTTP requests to ensure they contain a valid login token. If a token is missing, malformed, or expired, the gateway must terminate the request immediately and return a `401 Unauthorized` response, preventing any traffic from reaching downstream microservices.

## Interface Contracts

The authentication logic will be decoupled from the HTTP transport layer using a `TokenValidator` interface. This allows the gateway to switch between different authentication strategies (e.g., JWT, Opaque Tokens, or OAuth2) without changing the middleware logic.

```go
package auth

import (
	"context"
	"errors"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrMissingToken = errors.New("authentication token is missing")
)

// UserIdentity represents the authenticated entity extracted from the token
type UserIdentity struct {
	UserID   string
	Roles    []string
	TenantID string
}

// TokenValidator defines the contract for validating access tokens
type TokenValidator interface {
	// Validate checks the token string and returns the associated identity
	Validate(ctx context.Context, token string) (*UserIdentity, error)
}

// AuthMiddleware defines the signature for the gateway interceptor
type AuthMiddleware func(next http.Handler) http.Handler
```

## Data Flow

1.  **Request Entry**: An incoming HTTP request hits the ZenGate AI Gateway.
2.  **Header Extraction**: The `AuthMiddleware` extracts the `Authorization` header.
    *   Expected format: `Authorization: Bearer <token>`
3.  **Validation Trigger**:
    *   **If header is missing/malformed**: The middleware immediately returns `401 Unauthorized` with a JSON body `{"error": "Authentication token is missing"}`.
    *   **If header exists**: The middleware passes the token string to the `TokenValidator.Validate()` method.
4.  **Token Verification**:
    *   The `TokenValidator` verifies the signature, expiration date, and issuer.
    *   **Failure**: Returns `ErrInvalidToken` $\rightarrow$ Middleware returns `401 Unauthorized`.
    *   **Success**: Returns `UserIdentity` object.
5.  **Context Enrichment**: The `UserIdentity` is injected into the request `context.Context`. This allows downstream services to know who the user is without re-validating the token.
6.  **Proxy Forwarding**: The request is passed to the next handler in the chain and eventually proxied to the destination microservice.

## Design Decisions & Trade-offs

### 1. Fail-Fast at the Edge
**Decision**: Authentication is performed at the outermost layer of the gateway.
**Trade-off**: This increases the CPU load on the gateway but protects downstream services from processing unauthorized requests, reducing overall system noise and preventing potential DoS attacks on internal APIs.

### 2. Stateless Validation (JWT)
**Decision**: The system will prioritize JSON Web Tokens (JWT) for validation.
**Trade-off**: 
- *Pro*: No database lookup is required for every request (high performance).
- *Con*: Token revocation is harder. To mitigate this, we will implement a short TTL (Time-to-Live) for tokens and a distributed blacklist (Redis) for revoked tokens.

### 3. Standardized Header
**Decision**: Strict adherence to the `Authorization: Bearer` scheme.
**Trade-off**: While custom headers (e.g., `X-ZenGate-Token`) are possible, using the industry standard ensures compatibility with standard API clients and frontend libraries.

### 4. Context Propagation
**Decision**: Once validated, the token is stripped or replaced by a internal `X-User-ID` header before being sent to downstream services.
**Trade-off**: This prevents downstream services from needing to implement the same complex validation logic, though it requires a trusted network between the gateway and the services.

## Dependencies

| Dependency | Purpose |
| :--- | :--- |
| `golang-jwt/jwt` | For parsing and validating JWT signatures and claims. |
| `Redis` | (Optional) For storing a blacklist of revoked tokens to handle immediate logouts. |
| `net/http` | Standard Go library for middleware implementation. |
| `context` | For passing user identity through the request lifecycle. |