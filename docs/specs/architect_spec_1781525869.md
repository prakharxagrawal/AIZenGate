# Technical Specification: JWT Middleware Configuration Refactor

## Overview
The objective of this change is to decouple the `JWTMiddleware` initialization from raw string parameters and instead integrate it with the centralized `config.Config` structure. This ensures that the authentication layer remains consistent with the application's global configuration management and simplifies the dependency injection process during server startup.

## Interface Contracts

### Modified Constructor
The `NewJWTMiddleware` function signature will be updated to accept the configuration pointer.

```go
package auth

import (
    "github.com/zengate-ai/zengate/internal/config"
)

// NewJWTMiddleware initializes the JWT validation middleware using the provided application configuration.
// It extracts the JWT secret key from the config object to validate incoming tokens.
func NewJWTMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
    // Implementation will now reference cfg.JWTSecret (or equivalent field)
    // instead of a passed-in string.
}
```

### Expected Config Structure
The `config.Config` struct is expected to contain the following field:
```go
type Config struct {
    // ... other fields
    JWTSecret string `env:"JWT_SECRET" json:"jwt_secret"`
}
```

## Data Flow

1.  **Initialization Phase**: 
    *   `main.go` loads the environment variables into a `*config.Config` instance.
    *   The `*config.Config` instance is passed as an argument to `auth.NewJWTMiddleware(cfg)`.
2.  **Middleware Execution Phase**:
    *   The middleware captures the secret key from the config object during the closure creation.
    *   For every incoming request, the middleware uses this stored key to verify the signature of the JWT provided in the `Authorization` header.
3.  **Test Execution Phase**:
    *   `middleware_test.go` instantiates a mock `config.Config` object.
    *   The mock config is passed to the middleware to validate both successful and failed token scenarios.

## Design Decisions & Trade-offs

| Decision | Rationale | Trade-off |
| :--- | :--- | :--- |
| **Config Injection** | Centralizes secret management. If the config structure changes (e.g., moving to a Key Vault), only the config package needs updates, not every middleware call site. | Increases the coupling between the `auth` package and the `config` package. |
| **Closure-based Secret Storage** | The secret is read once during `NewJWTMiddleware` and stored in the closure, avoiding pointer dereferencing on every single HTTP request. | If the config were to support dynamic secret rotation without restart, this approach would require a mutex or an atomic value. |
| **Pointer Passing** | Passing `*config.Config` avoids copying a potentially large configuration struct on every initialization call. | Requires ensuring the config object is not mutated elsewhere during the application lifecycle. |

## Dependencies

- **`internal/config`**: Required for the `Config` type definition.
- **`net/http`**: Standard library for middleware handler signatures.
- **`github.com/golang-jwt/jwt`** (or equivalent): Used within the middleware implementation for token parsing.

## Test Update Requirements
The `internal/auth/middleware_test.go` must be updated as follows:
1.  **Setup**: Replace `secret := "test-secret"` with `cfg := &config.Config{JWTSecret: "test-secret"}`.
2.  **Invocation**: Update `NewJWTMiddleware(secret)` $\rightarrow$ `NewJWTMiddleware(cfg)`.
3.  **Coverage**: Ensure tests cover cases where `cfg.JWTSecret` might be empty to validate error handling during middleware initialization.