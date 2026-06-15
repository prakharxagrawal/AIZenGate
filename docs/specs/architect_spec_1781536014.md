# Technical Specification: Gemini Model Migration for Self-Healer

## Overview
The `self-healer` component is currently experiencing service interruptions due to `429 Too Many Requests` or `Quota Exceeded` errors. This is caused by the `brain` package targeting `gemini-2.5-flash`, which is not available on the Google AI Studio free tier. 

This specification outlines the migration of the `brain` package to use `gemini-1.5-flash`, the standard model used by the ADK for code development, ensuring compatibility with free-tier quotas while maintaining the reasoning capabilities required for self-healing operations.

## Interface Contracts

To prevent future hardcoding of model versions, the `brain` package will move toward a configuration-driven model selection.

### Brain Package Interface
The `LLMClient` interface will be updated to ensure the model version is injectable during initialization.

```go
package brain

import "context"

// LLMRequest defines the input for the brain's reasoning engine
type LLMRequest struct {
    Prompt      string
    Temperature float64
    MaxTokens   int
}

// LLMResponse defines the structured output from the brain
type LLMResponse struct {
    Content string
    Usage   TokenUsage
}

type TokenUsage struct {
    PromptTokens int
    CompletionTokens int
}

// LLMClient defines the contract for interacting with Gemini
type LLMClient interface {
    // GenerateContent sends a prompt to the configured Gemini model
    GenerateContent(ctx context.Context, req LLMRequest) (*LLMResponse, error)
    
    // GetModelVersion returns the current active model ID
    GetModelVersion() string
}

// Config holds the brain's operational parameters
type Config struct {
    APIKey    string
    ModelID   string // e.g., "gemini-1.5-flash"
    TimeoutMS int
}
```

## Data Flow

The following flow describes how the `self-healer` triggers a recovery action via the `brain` package:

1.  **Trigger**: `self-healer` detects a system failure (e.g., crash loop or failed health check).
2.  **Request**: `self-healer` constructs a prompt containing the error logs and system state, calling `brain.GenerateContent()`.
3.  **Configuration Lookup**: The `brain` package retrieves the `ModelID` (now `gemini-1.5-flash`) from the environment configuration.
4.  **API Call**: The `brain` package signs the request with the API Key and forwards it to the Gemini API endpoint.
5.  **Response**: Gemini returns the suggested fix; `brain` parses the response and returns it to the `self-healer`.
6.  **Execution**: `self-healer` applies the suggested fix to the infrastructure.

**Sequence Diagram:**
`Self-Healer` $\rightarrow$ `Brain (LLMClient)` $\rightarrow$ `Gemini API (gemini-1.5-flash)` $\rightarrow$ `Brain` $\rightarrow$ `Self-Healer`

## Design Decisions & Trade-offs

### 1. Model Selection: `gemini-1.5-flash`
*   **Decision**: Migrate from `gemini-2.5-flash` to `gemini-1.5-flash`.
*   **Reasoning**: `gemini-1.5-flash` is optimized for speed and efficiency, has a generous free tier quota, and is the proven baseline for the ADK's code generation tasks.
*   **Trade-off**: While `2.5` (or higher versions) may offer superior reasoning, the availability and cost-effectiveness of `1.5-flash` are critical for the stability of the self-healing loop.

### 2. Configuration over Hardcoding
*   **Decision**: Move the model string from a constant in the `brain` package to an environment variable (`GEMINI_MODEL_ID`).
*   **Reasoning**: This allows the Ops team to toggle between `flash` and `pro` models without recompiling the Go binary if quota limits change or if a specific healing task requires higher reasoning capabilities.

### 3. Error Handling for Quota Exhaustion
*   **Decision**: Implement a specific error wrapper for `429` responses.
*   **Reasoning**: If the free tier quota is hit even on `1.5-flash`, the `self-healer` should enter a "back-off" state rather than continuously hammering the API, which could lead to a temporary IP ban.

## Dependencies

| Dependency | Version | Role |
| :--- | :--- | :--- |
| `google-generative-ai-go` | Latest | Official Go SDK for Gemini |
| `envconfig` | v1.4.0 | For mapping environment variables to `brain.Config` |
| `context` | Standard Lib | For managing request timeouts and cancellations |