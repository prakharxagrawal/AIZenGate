package health

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/zengate-ai/zengate/internal/config"
)

// Handler serves the /health endpoint with gateway status info.
type Handler struct {
	cfg       *config.Config
	startTime time.Time
	connCount atomic.Int64
}

// Response is the JSON structure returned by the health endpoint.
type Response struct {
	Status            string  `json:"status"`
	Version           string  `json:"version"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
	ActiveConnections int64   `json:"active_connections"`
	LoadedPolicies    int     `json:"loaded_policies"`
	GoRoutines        int     `json:"goroutines"`
	MemoryAllocMB     float64 `json:"memory_alloc_mb"`
	Environment       string  `json:"environment"`
}

// NewHandler creates a new health check handler.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		cfg:       cfg,
		startTime: time.Now(),
	}
}

// IncrementConnections atomically increments the active connection count.
func (h *Handler) IncrementConnections() {
	h.connCount.Add(1)
}

// DecrementConnections atomically decrements the active connection count.
func (h *Handler) DecrementConnections() {
	h.connCount.Add(-1)
}

// ServeHTTP responds with the health status of the gateway.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	resp := Response{
		Status:            "healthy",
		Version:           h.cfg.Version,
		UptimeSeconds:     int64(time.Since(h.startTime).Seconds()),
		ActiveConnections: h.connCount.Load(),
		LoadedPolicies:    0, // Phase 2: will read from etcd
		GoRoutines:        runtime.NumGoroutine(),
		MemoryAllocMB:     float64(memStats.Alloc) / 1024 / 1024,
		Environment:       h.cfg.Env,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
