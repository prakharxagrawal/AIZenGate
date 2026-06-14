package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Handler collects and exposes Prometheus-compatible metrics.
// Phase 1 uses a simple in-memory implementation.
// Phase 5 will integrate the official prometheus/client_golang library.
type Handler struct {
	mu sync.RWMutex

	// Counters
	requestsTotal    map[string]*atomic.Int64 // key: "method:path:status"
	rateLimitedTotal atomic.Int64

	// Histograms (simplified: just tracking sum and count per bucket)
	latencySum   map[string]*atomic.Int64 // key: "method:path" → total ms
	latencyCount map[string]*atomic.Int64 // key: "method:path" → count

	// Gauges
	activeConnections atomic.Int64

	startTime time.Time
}

// NewHandler creates a new metrics handler.
func NewHandler() *Handler {
	return &Handler{
		requestsTotal: make(map[string]*atomic.Int64),
		latencySum:    make(map[string]*atomic.Int64),
		latencyCount:  make(map[string]*atomic.Int64),
		startTime:     time.Now(),
	}
}

// RecordRequest records a completed request with its method, path, status, and duration.
func (h *Handler) RecordRequest(method, path string, status int, duration time.Duration) {
	// Normalize path to prevent cardinality explosion
	normalizedPath := normalizePath(path)

	// Increment request counter
	counterKey := fmt.Sprintf("%s:%s:%d", method, normalizedPath, status)
	h.getOrCreateCounter(counterKey).Add(1)

	// Record latency
	latencyKey := fmt.Sprintf("%s:%s", method, normalizedPath)
	h.getOrCreateLatencySum(latencyKey).Add(duration.Milliseconds())
	h.getOrCreateLatencyCount(latencyKey).Add(1)

	// Track rate limited requests
	if status == http.StatusTooManyRequests {
		h.rateLimitedTotal.Add(1)
	}
}

// RecordRateLimited increments the rate-limited counter.
func (h *Handler) RecordRateLimited() {
	h.rateLimitedTotal.Add(1)
}

// ServeHTTP outputs metrics in Prometheus text exposition format.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var sb strings.Builder

	// HELP and TYPE for requests_total
	sb.WriteString("# HELP zengate_requests_total Total number of HTTP requests.\n")
	sb.WriteString("# TYPE zengate_requests_total counter\n")

	h.mu.RLock()
	// Sort keys for deterministic output
	counterKeys := make([]string, 0, len(h.requestsTotal))
	for k := range h.requestsTotal {
		counterKeys = append(counterKeys, k)
	}
	sort.Strings(counterKeys)

	for _, key := range counterKeys {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 {
			val := h.requestsTotal[key].Load()
			sb.WriteString(fmt.Sprintf("zengate_requests_total{method=\"%s\",path=\"%s\",status=\"%s\"} %d\n",
				parts[0], parts[1], parts[2], val))
		}
	}

	// Rate limited counter
	sb.WriteString("\n# HELP zengate_rate_limited_total Total rate-limited requests.\n")
	sb.WriteString("# TYPE zengate_rate_limited_total counter\n")
	sb.WriteString(fmt.Sprintf("zengate_rate_limited_total %d\n", h.rateLimitedTotal.Load()))

	// Latency metrics
	sb.WriteString("\n# HELP zengate_request_duration_ms Request duration in milliseconds.\n")
	sb.WriteString("# TYPE zengate_request_duration_ms summary\n")

	latencyKeys := make([]string, 0, len(h.latencySum))
	for k := range h.latencySum {
		latencyKeys = append(latencyKeys, k)
	}
	sort.Strings(latencyKeys)

	for _, key := range latencyKeys {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) == 2 {
			sum := h.latencySum[key].Load()
			count := h.latencyCount[key].Load()
			avg := int64(0)
			if count > 0 {
				avg = sum / count
			}
			sb.WriteString(fmt.Sprintf("zengate_request_duration_ms_sum{method=\"%s\",path=\"%s\"} %d\n",
				parts[0], parts[1], sum))
			sb.WriteString(fmt.Sprintf("zengate_request_duration_ms_count{method=\"%s\",path=\"%s\"} %d\n",
				parts[0], parts[1], count))
			sb.WriteString(fmt.Sprintf("zengate_request_duration_ms_avg{method=\"%s\",path=\"%s\"} %d\n",
				parts[0], parts[1], avg))
		}
	}
	h.mu.RUnlock()

	// Uptime
	sb.WriteString("\n# HELP zengate_uptime_seconds Gateway uptime in seconds.\n")
	sb.WriteString("# TYPE zengate_uptime_seconds gauge\n")
	sb.WriteString(fmt.Sprintf("zengate_uptime_seconds %d\n", int64(time.Since(h.startTime).Seconds())))

	w.Write([]byte(sb.String()))
}

// Snapshot returns a JSON-serializable snapshot of current metrics.
func (h *Handler) Snapshot() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	counters := make(map[string]int64)
	for k, v := range h.requestsTotal {
		counters[k] = v.Load()
	}

	return map[string]interface{}{
		"requests_total":     counters,
		"rate_limited_total": h.rateLimitedTotal.Load(),
		"uptime_seconds":     int64(time.Since(h.startTime).Seconds()),
	}
}

// SnapshotJSON returns the snapshot as JSON bytes.
func (h *Handler) SnapshotJSON() ([]byte, error) {
	return json.Marshal(h.Snapshot())
}

// --- Internal helpers ---

func (h *Handler) getOrCreateCounter(key string) *atomic.Int64 {
	h.mu.RLock()
	if counter, ok := h.requestsTotal[key]; ok {
		h.mu.RUnlock()
		return counter
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	// Double-check after acquiring write lock
	if counter, ok := h.requestsTotal[key]; ok {
		return counter
	}
	counter := &atomic.Int64{}
	h.requestsTotal[key] = counter
	return counter
}

func (h *Handler) getOrCreateLatencySum(key string) *atomic.Int64 {
	h.mu.RLock()
	if val, ok := h.latencySum[key]; ok {
		h.mu.RUnlock()
		return val
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	if val, ok := h.latencySum[key]; ok {
		return val
	}
	val := &atomic.Int64{}
	h.latencySum[key] = val
	return val
}

func (h *Handler) getOrCreateLatencyCount(key string) *atomic.Int64 {
	h.mu.RLock()
	if val, ok := h.latencyCount[key]; ok {
		h.mu.RUnlock()
		return val
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	if val, ok := h.latencyCount[key]; ok {
		return val
	}
	val := &atomic.Int64{}
	h.latencyCount[key] = val
	return val
}

// normalizePath reduces path cardinality by replacing dynamic segments.
// "/api/v1/users/123" → "/api/v1/users/:id"
func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// If it looks like an ID (numeric, UUID-like, etc.), replace it
		if len(part) > 0 && isLikelyID(part) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

// isLikelyID heuristically checks if a path segment is a dynamic ID.
func isLikelyID(s string) bool {
	// All digits
	allDigits := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits && len(s) > 0 {
		return true
	}

	// UUID-like (contains hyphens and hex chars, length >= 32)
	if len(s) >= 32 && strings.Contains(s, "-") {
		return true
	}

	return false
}
