// Package brain provides the reasoning engine for the self-healer.
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

// ErrQuotaExceeded is returned when the Gemini API returns a 429 Too Many Requests error.
var ErrQuotaExceeded = fmt.Errorf("gemini api quota exceeded")

type geminiClient struct {
	cfg    *Config
	logger *slog.Logger
}

// NewLLMClient creates a new instance of the Gemini LLM client.
func NewLLMClient(cfg *Config, logger *slog.Logger) LLMClient {
	return &geminiClient{
		cfg:    cfg,
		logger: logger,
	}
}

// GenerateContent sends a prompt to the configured Gemini model and parses the response.
func (c *geminiClient) GenerateContent(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutMS)*time.Millisecond)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(c.cfg.APIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(c.cfg.ModelID)
	c.configureModel(model, req)

	resp, err := model.GenerateContent(ctx, genai.Text(req.Prompt))
	if err != nil {
		return nil, c.handleAPIError(err)
	}

	return c.parseResponse(resp)
}

// GetModelVersion returns the current active model ID.
func (c *geminiClient) GetModelVersion() string {
	return c.cfg.ModelID
}

func (c *geminiClient) configureModel(model *genai.GenerativeModel, req LLMRequest) {
	if req.Temperature > 0 {
		model.SetTemperature(req.Temperature)
	}
	if req.MaxTokens > 0 {
		model.SetMaxOutputTokens(int32(req.MaxTokens))
	}
}

func (c *geminiClient) parseResponse(resp *genai.GenerateContentResponse) (*LLMResponse, error) {
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("gemini returned an empty response")
	}

	var content strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			content.WriteString(string(text))
		}
	}

	usage := TokenUsage{}
	if resp.UsageMetadata != nil {
		usage.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.CompletionTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &LLMResponse{
		Content: content.String(),
		Usage:   usage,
	}, nil
}

func (c *geminiClient) handleAPIError(err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "quota") {
		c.logger.Error("gemini api quota exceeded", 
			slog.String("model", c.cfg.ModelID), 
			slog.String("error", errStr),
		)
		return fmt.Errorf("%w: %v", ErrQuotaExceeded, err)
	}

	return fmt.Errorf("gemini api error: %w", err)
}