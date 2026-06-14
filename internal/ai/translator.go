package ai

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zengate-ai/zengate/internal/controlplane"
)

const translatorSystemPrompt = `You are the ZenGate AI Config Translator agent.
Your task is to translate natural language configuration statements into a single structured JSON policy matching this schema:
{
  "id": "policy-id-string",
  "path": "/url/path",     // exact path or "*" for any
  "method": "GET",         // GET, POST, PUT, DELETE, PATCH, OPTIONS, or "*" for any
  "limit": 100,            // maximum number of requests allowed (positive integer)
  "window_sec": 60,        // rate limit window duration in seconds (positive integer)
  "tier": "basic"          // client tier: premium, basic, anonymous, or "*" for any
}

Rules:
1. Return ONLY the raw JSON object. Do NOT wrap it in markdown code block markers (like formatting with ` + "`" + `json ... ` + "`" + `).
2. Generate a clean, concise ID from the tier and path (e.g., "premium-anything-limit").
3. Make sure the output is strictly valid JSON.`

// TranslatorHandler serves translation administrative requests.
type TranslatorHandler struct {
	brain    *Brain
	cpClient *controlplane.Client
}

// NewTranslatorHandler creates a new policy translator HTTP handler.
func NewTranslatorHandler(brain *Brain, cpClient *controlplane.Client) *TranslatorHandler {
	return &TranslatorHandler{
		brain:    brain,
		cpClient: cpClient,
	}
}

type translateRequest struct {
	Prompt string `json:"prompt"`
}

type translateResponse struct {
	Status string              `json:"status"`
	Policy controlplane.Policy `json:"policy"`
}

func (h *TranslatorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method_not_allowed"})
		return
	}

	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_prompt_payload"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Query LLM Brain to translate the configuration prompt
	aiResponse, err := h.brain.GenerateCompletion(ctx, translatorSystemPrompt, req.Prompt)
	if err != nil {
		slog.Error("failed to translate prompt using AI brain", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "translation_failed", "message": err.Error()})
		return
	}

	// Clean up markdown block decorations if LLM ignores the system instruction
	cleanJSON := strings.TrimSpace(aiResponse)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	cleanJSON = strings.TrimSpace(cleanJSON)

	var p controlplane.Policy
	if err := json.Unmarshal([]byte(cleanJSON), &p); err != nil {
		slog.Error("AI brain generated invalid policy JSON", "response", aiResponse, "clean", cleanJSON, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_model_output", "details": err.Error()})
		return
	}

	// Schema Validation
	if p.ID == "" || p.Path == "" || p.Limit <= 0 || p.WindowSec <= 0 {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_policy_structure", "message": "model generated incomplete policy keys"})
		return
	}

	// Default fallback fields
	if p.Method == "" {
		p.Method = "*"
	}
	if p.Tier == "" {
		p.Tier = "*"
	}

	// Convert back to JSON string to write to etcd
	payload, err := json.Marshal(p)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "serialization_failed"})
		return
	}

	cli := h.cpClient.GetEtcdClient()
	if cli == nil {
		slog.Warn("etcd client is nil, saving policy directly to local configuration cache", "id", p.ID)
		h.cpClient.AddPolicyToCache(p)
	} else {
		key := h.cpClient.Prefix() + p.ID
		// Write directly to etcd
		_, err = cli.Put(ctx, key, string(payload))
		if err != nil {
			slog.Error("failed to write translated policy to etcd", "id", p.ID, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "etcd_write_failed"})
			return
		}
	}

	slog.Info("policy translated and saved successfully via AI Brain", "id", p.ID, "prompt", req.Prompt)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(translateResponse{
		Status: "saved",
		Policy: p,
	})
}
