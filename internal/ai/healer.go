package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/zengate-ai/zengate/internal/controlplane"
	"github.com/zengate-ai/zengate/internal/metrics"
)

const healerSystemPrompt = `You are the ZenGate AI Self-Healing agent.
You have detected a high failure rate in request forwarding to the primary upstream server.
Upstream failure rate: %.2f%% (based on 5xx responses).

Your task is to select a backup target from the list of available healthy failover upstreams:
- Backup 1: http://httpbin.org
- Backup 2: http://echo:9090 (local mock)

Decide if failover routing is required.
Return a JSON object conforming to this schema:
{
  "unhealthy": true,
  "failover": true,
  "new_target": "http://httpbin.org", // select one backup target
  "explanation": "explanation of failover routing"
}

Return ONLY raw JSON.`

type SelfHealer struct {
	brain         *Brain
	cpClient      *controlplane.Client
	metrics       *metrics.Handler
	interval      time.Duration
	shutdownCh    chan struct{}
	wg            sync.WaitGroup
	failureLimit  float64 // Failure percentage threshold (e.g. 5.0 for 5%)
	isFailingOver bool
	mu            sync.Mutex
}

// NewSelfHealer creates a new background self-healer agent.
func NewSelfHealer(brain *Brain, cpClient *controlplane.Client, metricsHandler *metrics.Handler, interval time.Duration, failureLimit float64) *SelfHealer {
	return &SelfHealer{
		brain:        brain,
		cpClient:     cpClient,
		metrics:      metricsHandler,
		interval:     interval,
		failureLimit: failureLimit,
		shutdownCh:   make(chan struct{}),
	}
}

// Start spawns the healer monitor loop in the background.
func (h *SelfHealer) Start() {
	h.wg.Add(1)
	go h.runLoop()
}

// Stop stops the monitor loop execution.
func (h *SelfHealer) Stop() {
	close(h.shutdownCh)
	h.wg.Wait()
}

func (h *SelfHealer) runLoop() {
	defer h.wg.Done()
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	slog.Info("background self-healing agent started", "interval", h.interval, "threshold_percent", h.failureLimit)

	// Keep track of counts per interval
	var lastTotalRequests int64
	var lastFailedRequests int64

	for {
		select {
		case <-h.shutdownCh:
			slog.Info("stopping background self-healing agent")
			return
		case <-ticker.C:
			snapshot := h.metrics.Snapshot()
			reqMap, ok := snapshot["requests_total"].(map[string]int64)
			if !ok {
				continue
			}

			// Calculate total requests and failed (5xx status codes) requests
			var currentTotal int64
			var currentFailed int64

			for key, count := range reqMap {
				currentTotal += count
				// Key is method:path:status, e.g. "GET:/health:200" or "GET:/anything:502"
				if strings.HasSuffix(key, ":502") || strings.HasSuffix(key, ":504") || strings.HasSuffix(key, ":500") {
					currentFailed += count
				}
			}

			// Calculate interval differences
			totalInInterval := currentTotal - lastTotalRequests
			failedInInterval := currentFailed - lastFailedRequests

			lastTotalRequests = currentTotal
			lastFailedRequests = currentFailed

			// We need at least 10 requests in this window to make a statistically sound failover decision
			if totalInInterval < 10 {
				continue
			}

			failureRate := (float64(failedInInterval) / float64(totalInInterval)) * 100.0
			h.evaluateHealth(failureRate)
		}
	}
}

func (h *SelfHealer) evaluateHealth(failureRate float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If we are already running failover operations, don't trigger again
	if h.isFailingOver {
		return
	}

	if failureRate >= h.failureLimit {
		slog.Warn("upstream failure threshold breached! Triggering AI Self-Healer brain", "failure_rate_percent", failureRate)
		h.isFailingOver = true
		go h.queryAIAndRecover(failureRate)
	}
}

func (h *SelfHealer) queryAIAndRecover(failureRate float64) {
	defer func() {
		h.mu.Lock()
		h.isFailingOver = false
		h.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userPrompt := fmt.Sprintf("Failure rate is %.2f%%", failureRate)
	aiResponse, err := h.brain.GenerateCompletion(ctx, healerSystemPrompt, userPrompt)
	if err != nil {
		slog.Error("self-healer failed to query LLM", "error", err)
		return
	}

	// Clean JSON payload
	cleanJSON := strings.TrimSpace(aiResponse)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	cleanJSON = strings.TrimSpace(cleanJSON)

	type healerResult struct {
		Unhealthy   bool   `json:"unhealthy"`
		Failover    bool   `json:"failover"`
		NewTarget   string `json:"new_target"`
		Explanation string `json:"explanation"`
	}

	var res healerResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		slog.Error("self-healer parsed invalid AI JSON completion", "response", aiResponse, "error", err)
		return
	}

	slog.Info("self-healer AI completed recovery evaluation", "unhealthy", res.Unhealthy, "failover", res.Failover, "new_target", res.NewTarget, "explanation", res.Explanation)

	if res.Unhealthy && res.Failover && res.NewTarget != "" {
		h.executeFailover(res.NewTarget, res.Explanation)
	}
}

func (h *SelfHealer) executeFailover(newTarget, explanation string) {
	slog.Warn("self-healer executing dynamic route failover", "target", newTarget)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli := h.cpClient.GetEtcdClient()
	if cli == nil {
		slog.Warn("etcd client is nil, updating local dynamic upstream target directly", "new_target", newTarget)
		h.cpClient.UpdateUpstream(newTarget)
		return
	}

	key := "/zengate/upstream"
	_, err := cli.Put(ctx, key, newTarget)
	if err != nil {
		slog.Error("self-healer failed to write new upstream target to etcd", "key", key, "error", err)
	} else {
		slog.Warn("self-healer updated upstream target dynamically in etcd", "new_target", newTarget, "reason", explanation)
	}
}
