// Package brain provides interfaces and implementations for LLM-based decision making.
//
// The package is designed to be model-agnostic via the ModelProvider interface.
// By default, it targets "gemini-1.5-flash" to ensure compatibility with the
// Google AI Studio free tier.
//
// Example:
//
//	client, err := brain.NewGeminiClient(ctx, key, brain.DefaultModel)
//	content, err := client.GenerateContent(ctx, "Explain the system state.")
package brain