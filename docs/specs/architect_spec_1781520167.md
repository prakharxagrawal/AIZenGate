# Technical Specification: Centralized JWT Validation Key Management

## Overview
Currently, the JWT signature validation key in `internal/auth/middleware.go` is either hardcoded or retrieved directly from environment variables within the middleware logic. To improve maintainability, testability, and support for secret rotation, this specification moves the key source to the centralized `config` package.

The goal is to implement a Dependency Injection (DI) pattern where the middleware receives its configuration during initialization, decoupling the authentication logic from the configuration loading mechanism.

## Interface Contracts

### 1. Configuration Contract
The `config` package must provide a structured way to access authentication secrets.

```go
// internal/config/config.go

type AuthConfig struct {
    JWTSecret     string `mapstructure:"jwt_secret"`
    JWTExpiration time.Duration `mapstructure:"jwt_expiration"`
    Issuer        string `mapstructure:"issuer"`
}

type Config struct {
    Auth AuthConfig
    // ... other config sections
}
```

### 2. Middleware Constructor
The middleware will transition from a package-level function to a constructor that returns a handler, accepting the `AuthConfig` as a dependency.

```go
// internal/auth/middleware.go

type JWTMiddleware struct {
    config AuthConfig
}

// NewJWTMiddleware initializes the middleware with the required configuration
func NewJWTMiddleware(cfg AuthConfig) *JWTMiddleware {
    return &JWTMiddleware{
        config: cfg,
    }
}

// Handler is the actual middleware function used by the router
func (m *JWTMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Validation logic using m.config.JWTSecret
    })
}
```

## Data Flow

1.  **Application Startup**: 
    *   `cmd/server/main.go` calls the `config` package to load settings from `.env` or `config.yaml`.
    *   The `Config` struct is populated.
2.  **Dependency Injection**:
    *   The `main` function instantiates `auth.NewJWTMiddleware(cfg.Auth)`.
3.  **Request Lifecycle**:
    *   **Incoming Request** $\rightarrow$ **JWT Middleware**.
    *   Middleware extracts the token from the `Authorization` header.
    *   Middleware accesses `m.config.JWTSecret` to verify the signature.
    *   **If Valid**: Request proceeds to the next handler.
    *   **If Invalid**: Returns `401 Unauthorized`.

## Design Decisions & Trade-offs

| Decision | Justification | Trade-off |
| :--- | :--- | :--- |
| **Dependency Injection** | Moving the key from a global variable/env call to a struct field allows for easier unit testing (mocking configs). | Slightly more boilerplate in `main.go` for wiring. |
| **Config Struct Mapping** | Using a dedicated `AuthConfig` struct prevents the middleware from having access to the entire system configuration (Principle of Least Privilege). | Requires updating the config struct whenever a new auth parameter is added. |
| **Secret Storage** | The config package remains the single source of truth for how secrets are loaded (Env vs Vault vs File). | The middleware is now dependent on the `config` package's data structures. |

## Dependencies

- **`internal/config`**: Provides the `AuthConfig` definition and loading logic.
- **`github.com/golang-jwt/jwt`**: (or equivalent) Used for the actual signature verification using the key provided by the config.
- **`net/http`**: Standard library for middleware implementation.