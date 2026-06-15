package brain

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// LLMRequest defines the input for the brain's reasoning engine.
type LLMRequest struct {
	Prompt      string
	Temperature float64
	MaxTokens   int
}

// TokenUsage defines the token consumption of a request.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// LLMResponse defines the structured output from the brain.
type LLMResponse struct {
	Content string
	Usage   TokenUsage
}

// LLMClient defines the contract for interacting with Gemini.
type LLMClient interface {
	// GenerateContent sends a prompt to the configured Gemini model.
	GenerateContent(ctx context.Context, req LLMRequest) (*LLMResponse, error)
	// GetModelVersion returns the current active model ID.
	GetModelVersion() string
}

type geminiClient struct {
	client  *genai.Client
	modelID string
	logger  *slog.Logger
}

// NewClient initializes a new Gemini LLM client based on the provided configuration.
func NewClient(ctx context.Context, cfg Config, logger *slog.Logger) (LLMClient, error) {
	if logger == nil {
		logger = slog.Default()
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.APIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &geminiClient{
		client:  client,
		modelID: cfg.ModelID,
		logger:  logger,
	}, nil
}

// GenerateContent executes the prompt against the configured Gemini model.
func (c *geminiClient) GenerateContent(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	model := c.client.GenerativeModel(c.modelID)
	
	// Configure model parameters
	model.SetTemperature(req.Temperature)
	if req.MaxTokens > 0 {
		model.SetMaxOutputTokens(int32(req.MaxTokens))
	}

	c.logger.Debug("sending request to gemini", "model", c.modelID, "prompt_len", len(req.Prompt))

	resp, err := model.GenerateContent(ctx, genai.Text(req.Prompt))
	if err != nil {
		return nil, c.handleAPIError(err)
	}

	return c.parseResponse(resp)
}

// GetModelVersion returns the current active model ID.
func (c *geminiClient) GetModelVersion() string {
	return c.modelID
}

func (c *geminiClient) handleAPIError(err error) error {
	errMsg := err.Error()
	// Check for 429 Too Many Requests or Quota Exceeded patterns in the SDK error
	if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "quota exceeded") {
		c.logger.Warn("gemini api quota exceeded", "error", errMsg)
		return fmt.Errorf("%w: %v", ErrQuotaExceeded, err)
	}
	
	return fmt.Errorf("gemini api error: %w", err)
}

func (c *geminiClient) parseResponse(resp *genai.GenerateContentResponse) (*LLMResponse, error) {
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, ErrInvalidResponse
	}

	var contentBuilder strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			contentBuilder.WriteString(string(text))
		}
	}

	// Note: The current genai Go SDK might not provide detailed token usage 
	// in the same way as the REST API for all models. 
	// We initialize usage to zero if not available.
	usage := TokenUsage{}

	return &LLMResponse{
		Content: contentBuilder.String(),
		Usage:   usage,
	}, nil
}