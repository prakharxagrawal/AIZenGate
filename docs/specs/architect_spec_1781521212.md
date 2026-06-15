# Technical Specification: JWT Validation Key Migration to Config

## Overview
Currently, the JWT signature validation in `internal/auth/middleware.go` retrieves its secret key from a non-centralized source (e.g., direct environment variable access or hardcoded values). This specification outlines the migration to a centralized configuration management system. 

The goal is to decouple the authentication logic from the environment retrieval mechanism, enabling better testability (via mocking) and supporting future transitions to secret management services (e.g., AWS Secrets Manager, HashiCorp Vault) without modifying the middleware logic.

## Interface Contracts

To ensure loose coupling, the middleware will no longer access the environment directly. Instead, it will depend on a configuration provider.

### 1. Configuration Interface
Defined in `internal/config/config.go` (or a dedicated auth config package):

```go
package config

// AuthConfig defines the required configuration parameters for authentication
type AuthConfig interface {
    GetJWTSecret() []byte
    GetJWTExpiration() int // Optional: if expiration is also managed via config
}
```

### 2. Middleware Structure
The middleware will be transitioned from a standalone function to a struct-based handler to allow for dependency injection.

```go
package auth

import (
    "net/http"
    "zengate/internal/config"
)

type JWTMiddleware struct {
    cfg config.AuthConfig
}

// NewJWTMiddleware initializes the middleware with the provided config
func NewJWTMiddleware(cfg config.AuthConfig) *JWTMiddleware {
    return &JWTMiddleware{
        cfg: cfg,
    }
}

// Handler returns the actual middleware function for the router
func (m *JWTMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Implementation uses m.cfg.GetJWTSecret() for validation
    })
}
```

## Data Flow

1.  **Initialization Phase**:
    *   `main.go` initializes the `Config` object (loading from `.env`, YAML, or Vault).
    *   `main.go` instantiates `JWTMiddleware` by passing the `Config` object.
    *   The `JWTMiddleware` instance is registered in the HTTP router.

2.  **Request Phase**:
    *   **Client** sends a request with an `Authorization: Bearer <token>` header.
    *   **JWTMiddleware.Handler** intercepts the request.
    *   **Middleware** calls `m.cfg.GetJWTSecret()` to retrieve the current validation key.
    *   **JWT Library** validates the token signature using the retrieved key.
    *   **Middleware** either allows the request to proceed to the next handler or returns `401 Unauthorized`.

## Design Decisions & Trade-offs

### Decision 1: Dependency Injection over Global Singleton
*   **Decision**: Use a struct with an injected `AuthConfig` interface rather than a global `config.Get()` call.
*   **Trade-off**: Increases boilerplate slightly (requires constructor), but allows for unit testing the middleware with a mock configuration without setting environment variables.

### Decision 2: Interface-based Config
*   **Decision**: Define an `AuthConfig` interface rather than passing a concrete `Config` struct.
*   **Trade-off**: Adds a layer of abstraction. However, this allows the system to switch from a static file-based config to a dynamic secret provider (that might refresh keys periodically) without changing the middleware code.

### Decision 3: Byte Slice Return Type
*   **Decision**: `GetJWTSecret()` returns `[]byte` instead of `string`.
*   **Trade-off**: Most Go JWT libraries (like `golang-jwt`) require `[]byte` for HMAC validation. Returning bytes directly avoids repeated casting within the middleware hot-path.

## Dependencies

| Dependency | Purpose | Version |
| :--- | :--- | :--- |
| `internal/config` | Provides the `AuthConfig` implementation | Internal |
| `github.com/golang-jwt/jwt` | JWT parsing and validation | v5.x |
| `net/http` | Standard library for middleware implementation | Standard Lib |