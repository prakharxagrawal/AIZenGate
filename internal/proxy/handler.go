package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/metrics"
)

// Handler wraps httputil.ReverseProxy with ZenGate-specific logic:
// request ID injection, upstream latency tracking, and error handling.
type Handler struct {
	proxy   *httputil.ReverseProxy
	target  *url.URL
	metrics *metrics.Handler
}

// NewHandler creates a reverse proxy handler pointed at the configured upstream.
func NewHandler(cfg *config.Config, metricsHandler *metrics.Handler) (*Handler, error) {
	target, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL %q: %w", cfg.UpstreamURL, err)
	}

	rp := httputil.NewSingleHostReverseProxy(target)

	// Custom director: rewrites the request before forwarding
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)

		// Preserve the original Host header (important for virtual hosting)
		req.Host = target.Host

		// Forward client IP
		if clientIP := req.Header.Get("X-Forwarded-For"); clientIP == "" {
			req.Header.Set("X-Forwarded-For", req.RemoteAddr)
		}

		// Add gateway identification
		req.Header.Set("X-Forwarded-By", "ZenGate/"+cfg.Version)
	}

	// Custom error handler: returns a structured JSON error
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		requestID := r.Header.Get("X-Request-Id")
		slog.Error("upstream error",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"upstream", target.String(),
			"error", err,
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"bad_gateway","message":"upstream service unavailable","request_id":"%s"}`, requestID)
	}

	// Custom response modifier: add gateway headers to every response
	rp.ModifyResponse = func(resp *http.Response) error {
		// Track upstream latency if the start time was set
		if startStr := resp.Request.Header.Get("X-Zengate-Proxy-Start"); startStr != "" {
			if start, err := time.Parse(time.RFC3339Nano, startStr); err == nil {
				latency := time.Since(start)
				resp.Header.Set("X-Upstream-Latency", fmt.Sprintf("%dms", latency.Milliseconds()))
			}
		}

		resp.Header.Set("X-Powered-By", "ZenGate AI")
		return nil
	}

	// Set transport timeouts
	rp.Transport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		ResponseHeaderTimeout: cfg.ProxyTimeout,
	}

	return &Handler{
		proxy:   rp,
		target:  target,
		metrics: metricsHandler,
	}, nil
}

// ServeHTTP implements http.Handler — proxies the request to the upstream.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Skip internal routes that shouldn't be proxied
	if isInternalPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}

	// Set proxy start time for latency tracking
	r.Header.Set("X-Zengate-Proxy-Start", time.Now().Format(time.RFC3339Nano))

	slog.Debug("proxying request",
		"request_id", r.Header.Get("X-Request-Id"),
		"method", r.Method,
		"path", r.URL.Path,
		"upstream", h.target.String(),
	)

	h.proxy.ServeHTTP(w, r)
}

// isInternalPath returns true for paths that the gateway handles directly
// and should NOT be forwarded to the upstream.
func isInternalPath(path string) bool {
	internal := []string{"/health", "/healthz", "/metrics", "/api/v1/policies"}
	for _, p := range internal {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
