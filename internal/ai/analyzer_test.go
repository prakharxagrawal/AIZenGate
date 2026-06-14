package ai

import (
	"testing"
	"time"

	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/controlplane"
	"github.com/zengate-ai/zengate/internal/metrics"
)

func TestTrafficAnalyzer_AnomalyScaling(t *testing.T) {
	// Configure in mock mode
	cfg := &config.Config{
		DeepSeekAPIKey: "",
		GeminiAPIKey:   "",
	}
	brain := NewBrain(cfg)

	// Create controlplane client (in-memory mode)
	cpClient := &controlplane.Client{}

	// Seed client cache with a base policy
	basePolicy := controlplane.Policy{
		ID:        "mock-policy-1",
		Path:      "/anything",
		Method:    "*",
		Limit:     100,
		WindowSec: 60,
		Tier:      "basic",
	}
	cpClient.AddPolicyToCache(basePolicy)

	metricsHandler := metrics.NewHandler()

	analyzer := NewTrafficAnalyzer(brain, cpClient, metricsHandler, 1*time.Second)

	// 1. Send baseline TPS measurements (5 baseline points) to establish standard deviation
	// Mean around 2.0 TPS
	analyzer.analyzeTPS(2.0)
	analyzer.analyzeTPS(1.9)
	analyzer.analyzeTPS(2.1)
	analyzer.analyzeTPS(2.0)
	analyzer.analyzeTPS(2.0)

	// 2. Trigger anomaly check by supplying a huge TPS spike (e.g., 20.0 TPS)
	// This is > 3 standard deviations above the baseline mean of 2.0 TPS
	analyzer.analyzeTPS(20.0)

	// Sleep slightly to let the async goroutine queryAIAndScale complete (uses mock brain)
	time.Sleep(100 * time.Millisecond)

	// Verify that the policy limit in controlplane client cache was scaled
	scaledPolicy, found := cpClient.GetPolicy("mock-policy-1")
	if !found {
		t.Fatalf("policy was removed unexpectedly")
	}

	// Mock completion uses factor 2.0 or standard scaled response.
	// In brain.go's mockCompletion, for anomaly suggestion, it returns:
	// "anomalous": true, "suggestion": "increase-limit", "reason": "detected traffic spike..."
	// Wait, in brain.go's mockCompletion:
	// case bytes.Contains([]byte(userPrompt), []byte("anomalous")) || bytes.Contains([]byte(userPrompt), []byte("anomaly")):
	// 	response = map[string]interface{}{
	// 		"anomalous":  true,
	// 		"suggestion": "increase-limit",
	// 		"reason":     "detected traffic spike exceeding 3 standard deviations",
	// 	}
	// Note that in mockCompletion, there is no "factor" field returned!
	// Wait! In analyzer.go's queryAIAndScale:
	// if res.Anomalous && res.Suggestion == "increase-limit" && res.Factor > 1.0
	// If the mock response doesn't have "factor", it defaults to 0.0, which won't scale!
	// Ah! Let's check what factor defaults to in Go JSON deserialization when missing: it is 0.0.
	// So we need to add "factor": 2.0 to the mock response in brain.go so that mock mode actually scales the limits!
	// Let's verify this by checking brain.go's mockCompletion:
	//	case bytes.Contains([]byte(userPrompt), []byte("anomalous")) || bytes.Contains([]byte(userPrompt), []byte("anomaly")):
	//		response = map[string]interface{}{
	//			"anomalous":  true,
	//			"suggestion": "increase-limit",
	//			"reason":     "detected traffic spike exceeding 3 standard deviations",
	//		}
	// Yes, there is no factor! Let's add `"factor": 2.0` in brain.go's mockCompletion.
	// Let's write the test first, then we will modify brain.go.

	if scaledPolicy.Limit != 200 {
		t.Errorf("expected rate limit to scale from 100 to 200, got %d", scaledPolicy.Limit)
	}
}
