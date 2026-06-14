package ai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/controlplane"
)

func TestTranslatorHandler_ServeHTTP_Success(t *testing.T) {
	// Configure in mock mode
	cfg := &config.Config{
		DeepSeekAPIKey: "",
		GeminiAPIKey:   "",
	}
	brain := NewBrain(cfg)

	// Create controlplane client with nil etcd cli (in-memory mode)
	cpClient := &controlplane.Client{}

	handler := NewTranslatorHandler(brain, cpClient)

	// Prepare translate request payload
	reqBody := map[string]string{
		"prompt": "give premium tier 100 limit on /anything",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/translate", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var res translateResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Status != "saved" {
		t.Errorf("expected status 'saved', got %q", res.Status)
	}

	if res.Policy.ID == "" {
		t.Errorf("expected policy to have a generated ID")
	}

	// Verify that the policy was saved to the controlplane memory cache
	policy, found := cpClient.GetPolicy(res.Policy.ID)
	if !found {
		t.Fatalf("expected to find policy in controlplane client cache")
	}

	if policy.Path != "/anything" {
		t.Errorf("expected path to be '/anything', got %q", policy.Path)
	}
	if policy.Limit != 100 {
		t.Errorf("expected limit to be 100, got %d", policy.Limit)
	}
}

func TestTranslatorHandler_ServeHTTP_InvalidMethod(t *testing.T) {
	handler := NewTranslatorHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/policies/translate", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestTranslatorHandler_ServeHTTP_EmptyPayload(t *testing.T) {
	handler := NewTranslatorHandler(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/translate", strings.NewReader(""))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}
