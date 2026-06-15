# Brain API Documentation

## GeminiClient

### NewGeminiClient
Initializes a new client instance.

**Parameters:**
- `ctx`: context.Context
- `apiKey`: string
- `modelName`: string (Use `brain.DefaultModel` for stable free-tier access)

### GenerateContent
Sends a prompt to the configured Gemini model.

**Request:**