package brain

import "errors"

// Common brain package errors
var (
	// ErrQuotaExceeded is returned when the Gemini API returns a 429 Too Many Requests error.
	ErrQuotaExceeded = errors.New("gemini api quota exceeded")
	// ErrInvalidResponse is returned when the LLM returns an empty or malformed response.
	ErrInvalidResponse = errors.New("received invalid response from llm")
)