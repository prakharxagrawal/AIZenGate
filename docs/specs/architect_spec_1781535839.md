# Technical Specification: Gemini Model Migration for Self-Healer

## Overview
The ZenGate AI self-healer is currently experiencing API failures due to the use of `gemini-2.5-flash`, which does not support the free tier quota. This specification outlines the migration to `gemini-1.5-flash` and the implementation of a configuration-driven model selection mechanism to prevent future hard-coding regressions.

## Interface Contracts

To ensure the `brain` package remains decoupled from specific model versions, we will refine the provider initialization.

### Brain Provider Interface
The `brain` package should utilize a provider pattern where the model version is injected during instantiation.

```go
package brain

import "context"

// LLMProvider defines the contract for interacting with the Large Language Model
type LLMProvider interface {
    // GenerateResponse sends a prompt to the model and returns the generated text
    GenerateResponse(ctx context.Context, prompt string) (string, error)
    // GetModelName returns the current model being used for telemetry/logging
    GetModelName() string
}

// Config holds the configuration for the Brain provider
type Config struct {
    APIKey     string
    ModelName  string // e.g., "gemini-1.5-flash"
    Temperature float64
    MaxTokens   int
}

// NewGeminiProvider initializes a new Gemini-backed brain
func NewGeminiProvider(cfg Config) (LLMProvider, error) {
    // Implementation will initialize the google-generative-ai-go client
    // using cfg.ModelName
    return &geminiProvider{
        config: cfg,
        // client: ...
    }, nil
}
```

## Data Flow

1.  **Initialization Phase**:
    *   The application starts and reads the environment variable `ZENGATE_BRAIN_MODEL`.
    *   If the variable is empty, it defaults to `gemini-1.5-flash`.
    *   The `Config` struct is populated and passed to `NewGeminiProvider`.
2.  **Execution Phase**:
    *   The **Self-Healer** triggers a recovery event.
    *   The Self-Healer calls `GenerateResponse(ctx, prompt)` on the `LLMProvider`.
    *   The `geminiProvider` wraps the prompt and sends it to the Gemini API endpoint specifying the configured model (`gemini-1.5-flash`).
    *   The API returns the response based on the free-tier quota.

## Design Decisions & Trade-offs

### 1. Configuration over Hard-coding
*   **Decision**: Move the model identifier from a constant in the code to an environment variable.
*   **Reasoning**: Model versions evolve rapidly. Hard-coding requires a full CI/CD cycle (build, test, deploy) just to change a string. Environment variables allow for instant updates via Kubernetes ConfigMaps or `.env` files.

### 2. Model Selection: `gemini-1.5-flash`
*   **Decision**: Target `gemini-1.5-flash` specifically.
*   **Reasoning**: It provides the best balance of latency and reasoning capabilities while maintaining a generous free tier quota, ensuring the self-healer remains operational without incurring unexpected costs.

### 3. Graceful Fallback
*   **Decision**: Implement a default value in the `brain` package.
*   **Reasoning**: To ensure the system is "plug-and-play," the system should not crash if the environment variable is missing; it should default to the most stable free-tier model.

## Dependencies

| Dependency | Version | Purpose |
| :--- | :--- | :--- |
| `github.com/google/generative-ai-go` | Latest | Official Go SDK for Gemini API |
| `google.golang.org/api` | Latest | Underlying Google API transport |
| `os` (Standard Lib) | N/A | Environment variable retrieval |