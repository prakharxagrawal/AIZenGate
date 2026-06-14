package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/zengate-ai/zengate/internal/controlplane"
	"github.com/zengate-ai/zengate/internal/metrics"
	"go.etcd.io/etcd/client/v3"
)

const analyzerSystemPrompt = `You are the ZenGate AI Traffic Analyzer agent.
You are analyzing a sudden request throughput anomaly (TPS spike).
Based on the metrics:
- Baseline Average TPS: %.2f
- Current Spike TPS: %.2f

Decide if the gateway limits should be scaled up (e.g., to handle organic burst load) or left alone.
Return a JSON object conforming to this schema:
{
  "anomalous": true,
  "suggestion": "increase-limit", // "increase-limit" or "none"
  "factor": 2.0,                  // numeric scaling factor (e.g. 1.5, 2.0)
  "reason": "explanation of traffic analysis"
}

Return ONLY raw JSON.`

type TrafficAnalyzer struct {
	brain      *Brain
	cpClient   *controlplane.Client
	metrics    *metrics.Handler
	interval   time.Duration
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	// Historical TPS tracking for standard deviation calculations
	mu          sync.Mutex
	tpsHistory  []float64
	maxHistory  int
}

// NewTrafficAnalyzer creates a new background traffic analyzer agent.
func NewTrafficAnalyzer(brain *Brain, cpClient *controlplane.Client, metricsHandler *metrics.Handler, interval time.Duration) *TrafficAnalyzer {
	return &TrafficAnalyzer{
		brain:      brain,
		cpClient:   cpClient,
		metrics:    metricsHandler,
		interval:   interval,
		shutdownCh: make(chan struct{}),
		maxHistory: 60, // Track up to 60 historical intervals
		tpsHistory: make([]float64, 0, 60),
	}
}

// Start spawns the analyzer loop in the background.
func (a *TrafficAnalyzer) Start() {
	a.wg.Add(1)
	go a.runLoop()
}

// Stop stops the background execution loop.
func (a *TrafficAnalyzer) Stop() {
	close(a.shutdownCh)
	a.wg.Wait()
}

func (a *TrafficAnalyzer) runLoop() {
	defer a.wg.Done()
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	slog.Info("background traffic analyzer agent started", "interval", a.interval)

	// Keep track of total requests from metrics to calculate intervals
	var lastRequestCount int64

	for {
		select {
		case <-a.shutdownCh:
			slog.Info("stopping background traffic analyzer agent")
			return
		case <-ticker.C:
			// Fetch snapshot requests count
			snapshot := a.metrics.Snapshot()
			reqMap, ok := snapshot["requests_total"].(map[string]int64)
			if !ok {
				continue
			}

			var currentTotal int64
			for _, count := range reqMap {
				currentTotal += count
			}

			// Calculate current TPS
			diff := currentTotal - lastRequestCount
			lastRequestCount = currentTotal
			currentTPS := float64(diff) / a.interval.Seconds()

			a.analyzeTPS(currentTPS)
		}
	}
}

func (a *TrafficAnalyzer) analyzeTPS(tps float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Need at least 5 baseline points before evaluating standard deviation anomalies
	if len(a.tpsHistory) < 5 {
		a.tpsHistory = append(a.tpsHistory, tps)
		slog.Debug("gathering traffic baseline", "history_count", len(a.tpsHistory), "current_tps", tps)
		return
	}

	// Calculate average and standard deviation
	var sum float64
	for _, val := range a.tpsHistory {
		sum += val
	}
	mean := sum / float64(len(a.tpsHistory))

	var varianceSum float64
	for _, val := range a.tpsHistory {
		varianceSum += math.Pow(val-mean, 2)
	}
	stdDev := math.Sqrt(varianceSum / float64(len(a.tpsHistory)))

	// Check if current TPS violates 3-sigma (3 standard deviations above mean)
	threshold := mean + (3.0 * stdDev)
	isAnomalous := tps > threshold && tps > 5.0 // Min threshold to prevent tiny spikes from triggering

	slog.Debug("tps validation check", "current", tps, "mean", mean, "std_dev", stdDev, "threshold", threshold, "anomalous", isAnomalous)

	if isAnomalous {
		slog.Warn("traffic anomaly detected! Triggering AI analyzer brain", "tps", tps, "threshold", threshold)
		
		go a.queryAIAndScale(mean, tps)
	}

	// Rotate history buffer
	if len(a.tpsHistory) >= a.maxHistory {
		a.tpsHistory = a.tpsHistory[1:]
	}
	a.tpsHistory = append(a.tpsHistory, tps)
}

func (a *TrafficAnalyzer) queryAIAndScale(mean, tps float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userPrompt := fmt.Sprintf("Baseline mean: %.2f, Current spike: %.2f", mean, tps)
	aiResponse, err := a.brain.GenerateCompletion(ctx, analyzerSystemPrompt, userPrompt)
	if err != nil {
		slog.Error("traffic analyzer failed to execute LLM decision", "error", err)
		return
	}

	// Clean JSON payload
	cleanJSON := strings.TrimSpace(aiResponse)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	cleanJSON = strings.TrimSpace(cleanJSON)

	type analyzerResult struct {
		Anomalous  bool    `json:"anomalous"`
		Suggestion string  `json:"suggestion"`
		Factor     float64 `json:"factor"`
		Reason     string  `json:"reason"`
	}

	var res analyzerResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		slog.Error("traffic analyzer parsed invalid AI JSON completion", "response", aiResponse, "error", err)
		return
	}

	slog.Info("traffic analyzer AI completed analysis", "anomalous", res.Anomalous, "suggestion", res.Suggestion, "factor", res.Factor, "reason", res.Reason)

	if res.Anomalous && res.Suggestion == "increase-limit" && res.Factor > 1.0 {
		a.executeRateLimitScaling(res.Factor, res.Reason)
	}
}

func (a *TrafficAnalyzer) executeRateLimitScaling(factor float64, reason string) {
	slog.Info("traffic analyzer scaling rate limits dynamically", "factor", factor)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli := a.cpClient.GetEtcdClient()
	if cli == nil {
		slog.Warn("etcd client is nil, scaling policies directly in local cache")
		policies := a.cpClient.GetAllPolicies()
		for _, p := range policies {
			originalLimit := p.Limit
			p.Limit = int(float64(p.Limit) * factor)
			a.cpClient.AddPolicyToCache(p)
			slog.Warn("traffic policy scaled dynamically in cache by AI", "id", p.ID, "original_limit", originalLimit, "new_limit", p.Limit, "reason", reason)
		}
		return
	}

	prefix := a.cpClient.Prefix()

	// Read existing policies from etcd
	resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		slog.Error("traffic analyzer failed to fetch policies to scale", "error", err)
		return
	}

	for _, kv := range resp.Kvs {
		var p controlplane.Policy
		if err := json.Unmarshal(kv.Value, &p); err != nil {
			continue
		}

		// Scale limit
		originalLimit := p.Limit
		p.Limit = int(float64(p.Limit) * factor)

		payload, err := json.Marshal(p)
		if err != nil {
			continue
		}

		// Save scaled limit back to etcd
		_, err = cli.Put(ctx, string(kv.Key), string(payload))
		if err != nil {
			slog.Error("traffic analyzer failed to update scaled policy in etcd", "id", p.ID, "error", err)
		} else {
			slog.Warn("traffic policy scaled dynamically by AI", "id", p.ID, "original_limit", originalLimit, "new_limit", p.Limit, "reason", reason)
		}
	}
}
