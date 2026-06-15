# System Architecture: Gemini API Integration Refactor

## Overview
The current `brain` package implementation is failing due to a model availability mismatch. The `gemini-2.5-flash` model is currently restricted or unavailable in the free tier. To restore functionality, we are migrating the `brain` package to utilize `gemini-1.5-flash`, which is the stable, high-performance model currently supported by the Google AI Studio free tier for development tasks.

## Interface Contracts (Go)

To ensure the `brain` package remains decoupled from specific model versions, we will enforce the following interface. This allows for future model swaps without breaking the business logic.

```go
package brain

import "context"

// ModelProvider defines the contract for interacting with LLM backends.
type ModelProvider interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// GeminiClient implements ModelProvider for Google's Gemini API.
type GeminiClient struct {
	APIKey    string
	ModelName string // Should be set to "gemini-1.5-flash"
}

func (g *GeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	// Implementation logic using the configured ModelName
}
```

## Data Flow

1.  **Initialization**: The `SelfHealer` service initializes the `GeminiClient` by injecting the `GEMINI_MODEL` environment variable (defaulting to `gemini-1.5-flash`).
2.  **Request**: The `SelfHealer` passes the diagnostic logs and error context to the `GenerateContent` method.
3.  **Execution**: The `GeminiClient` constructs the request payload targeting the `v1beta/models/gemini-1.5-flash:generateContent` endpoint.
4.  **Response**: The API returns the structured repair plan; the `GeminiClient` parses the response and returns the raw string to the `SelfHealer`.
5.  **Error Handling**: If a 404 or 429 error occurs, the `SelfHealer` logs the specific model failure and triggers a fallback mechanism (if configured).

## Design Decisions & Trade-offs

*   **Decision: Hard-coding vs. Configuration**: We are moving the model name to a configuration constant/environment variable rather than hard-coding it in the logic.
    *   *Trade-off*: Adds a minor configuration step, but prevents future hard-coded failures when models are deprecated.
*   **Decision: Model Selection**: `gemini-1.5-flash` was chosen over `gemini-1.5-pro`.
    *   *Trade-off*: `flash` provides significantly lower latency and higher rate limits on the free tier, which is critical for the `SelfHealer`'s real-time diagnostic loop.
*   **Resilience Strategy**: We are implementing a "fail-fast" approach. If the model request fails, the `SelfHealer` will not retry indefinitely, preventing resource exhaustion in the event of a sustained API outage.

## Dependencies

*   **Google Generative AI Go SDK**: `github.com/google/generative-ai-go`
*   **Environment Configuration**: `github.com/joho/godotenv` (for local development)
*   **Required Environment Variable**:
    *   `GEMINI_MODEL=gemini-1.5-flash`

---
**Action Item**: Update the `brain` package configuration constants immediately and verify the `GEMINI_API_KEY` scope has access to the `1.5-flash` endpoint.