package ai

import (
	"testing"
	"time"

	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/controlplane"
	"github.com/zengate-ai/zengate/internal/metrics"
)

func TestSelfHealer_Failover(t *testing.T) {
	// Configure in mock mode
	cfg := &config.Config{
		DeepSeekAPIKey: "",
		GeminiAPIKey:   "",
	}
	brain := NewBrain(cfg)

	// Create controlplane client (in-memory mode)
	cpClient := &controlplane.Client{}

	metricsHandler := metrics.NewHandler()

	healer := NewSelfHealer(brain, cpClient, metricsHandler, 1*time.Second, 5.0)

	// Register a callback to listen for upstream updates
	var updatedTarget string
	cpClient.SetUpstreamUpdateCallback(func(target string) {
		updatedTarget = target
	})

	// Trigger health check evaluation directly with a 10% failure rate
	healer.evaluateHealth(10.0)

	// Sleep slightly to let the async goroutine queryAIAndRecover complete
	time.Sleep(100 * time.Millisecond)

	// Verify that the upstream target was updated dynamically
	expectedTarget := "http://backup-server:9090"
	if updatedTarget != expectedTarget {
		t.Errorf("expected failover target to be updated to %q, got %q", expectedTarget, updatedTarget)
	}
}
