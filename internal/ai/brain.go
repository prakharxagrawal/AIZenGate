package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/zengate-ai/zengate/internal/config"
)

// Brain coordinates LLM executions inside the gateway runtime.
type Brain struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewBrain creates a new AI client container.
func NewBrain(cfg *config.Config) *Brain {
	return &Brain{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GenerateCompletion queries the primary LLM (DeepSeek) with a fallback to Gemini or Mock mode.
func (b *Brain) GenerateCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// 1. Check if mock mode should trigger
	if b.cfg.DeepSeekAPIKey == "" && b.cfg.GeminiAPIKey == "" {
		slog.Warn("AI credentials not configured. Executing AI completion in Mock Mode")
		return b.mockCompletion(systemPrompt, userPrompt)
	}

	// 2. Try Primary Model: DeepSeek
	if b.cfg.DeepSeekAPIKey != "" {
		slog.Info("executing AI completion via DeepSeek API")
		res, err := b.queryDeepSeek(ctx, systemPrompt, userPrompt)
		if err == nil {
			return res, nil
		}
		slog.Warn("DeepSeek API execution failed, attempting Gemini fallback", "error", err)
	}

	// 3. Try Fallback Model: Gemini
	if b.cfg.GeminiAPIKey != "" {
		slog.Info("executing AI completion via Gemini API")
		res, err := b.queryGemini(ctx, systemPrompt, userPrompt)
		if err == nil {
			return res, nil
		}
		slog.Error("Gemini API execution failed as well", "error", err)
		return "", fmt.Errorf("both primary and fallback LLM queries failed: %w", err)
	}

	return "", fmt.Errorf("no available LLM credentials to process execution")
}

// --- DeepSeek Implementation ---

func (b *Brain) queryDeepSeek(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	apiURL := fmt.Sprintf("%s/chat/completions", b.cfg.DeepSeekBaseURL)

	reqBody := map[string]interface{}{
		"model": "deepseek-v4-flash-free",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.2,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", b.cfg.DeepSeekAPIKey))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("http error status %d: %s", resp.StatusCode, string(body))
	}

	var completionResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&completionResponse); err != nil {
		return "", err
	}

	if len(completionResponse.Choices) == 0 {
		return "", fmt.Errorf("empty choice array returned from DeepSeek")
	}

	return completionResponse.Choices[0].Message.Content, nil
}

// --- Gemini Implementation ---

func (b *Brain) queryGemini(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Gemini v1beta endpoint
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", b.cfg.GeminiAPIKey)

	combinedPrompt := fmt.Sprintf("%s\n\nUser Input:\n%s", systemPrompt, userPrompt)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": combinedPrompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.2,
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("http error status %d: %s", resp.StatusCode, string(body))
	}

	var completionResponse struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&completionResponse); err != nil {
		return "", err
	}

	if len(completionResponse.Candidates) == 0 || len(completionResponse.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response returned from Gemini")
	}

	return completionResponse.Candidates[0].Content.Parts[0].Text, nil
}

// --- Mock Fallback Logic ---

func (b *Brain) mockCompletion(systemPrompt, userPrompt string) (string, error) {
	// Standard deterministic mocks for local development when keys are omitted
	var response map[string]interface{}

	switch {
	// 1. Mock policy translations
	case bytes.Contains([]byte(userPrompt), []byte("rate limit")) || bytes.Contains([]byte(userPrompt), []byte("limit")) || bytes.Contains([]byte(userPrompt), []byte("translate")):
		response = map[string]interface{}{
			"id":         "mock-policy-1",
			"path":       "/anything",
			"method":     "*",
			"limit":      100,
			"window_sec": 60,
			"tier":       "basic",
		}
	// 2. Mock traffic analyzer anomaly suggestions
	case bytes.Contains([]byte(userPrompt), []byte("anomalous")) || bytes.Contains([]byte(userPrompt), []byte("anomaly")) || bytes.Contains([]byte(userPrompt), []byte("Baseline mean")) || bytes.Contains([]byte(userPrompt), []byte("spike")):
		response = map[string]interface{}{
			"anomalous":  true,
			"suggestion": "increase-limit",
			"factor":     2.0,
			"reason":     "detected traffic spike exceeding 3 standard deviations",
		}
	// 3. Mock self-healer routes
	case bytes.Contains([]byte(userPrompt), []byte("upstream")) || bytes.Contains([]byte(userPrompt), []byte("failover")) || bytes.Contains([]byte(userPrompt), []byte("Failure rate")):
		response = map[string]interface{}{
			"unhealthy":   true,
			"failover":    true,
			"new_target":  "http://backup-server:9090",
			"explanation": "upstream returned excessive 502 Bad Gateway responses",
		}
	default:
		return "Mock AI completed task successfully", nil
	}

	resBytes, _ := json.Marshal(response)
	return string(resBytes), nil
}
