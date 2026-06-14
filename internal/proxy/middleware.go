package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/metrics"
)

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares in order (outermost first).
// Chain(handler, m1, m2, m3) => m1(m2(m3(handler)))
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	// Apply in reverse so the first middleware in the list wraps outermost
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// --- Context Keys ---

type contextKey string

const (
	requestIDKey  contextKey = "request_id"
	startTimeKey  contextKey = "start_time"
)

// --- RequestID Middleware ---

// requestIDCounter is a simple atomic counter for generating unique request IDs.
// In production, use UUIDs — but for Phase 1 this is sufficient.
var requestIDCounter uint64

// RequestIDMiddleware injects a unique X-Request-Id header into every request.
func RequestIDMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestIDCounter++
				requestID = fmt.Sprintf("zg-%d-%d", time.Now().UnixMilli(), requestIDCounter)
			}

			// Set on request and response
			r.Header.Set("X-Request-Id", requestID)
			w.Header().Set("X-Request-Id", requestID)

			// Store in context for downstream access
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// --- Logging Middleware ---

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// LoggingMiddleware logs every request with structured fields.
func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := context.WithValue(r.Context(), startTimeKey, start)

			rw := newResponseWriter(w)
			next.ServeHTTP(rw, r.WithContext(ctx))

			duration := time.Since(start)

			// Choose log level based on status code
			level := slog.LevelInfo
			if rw.statusCode >= 500 {
				level = slog.LevelError
			} else if rw.statusCode >= 400 {
				level = slog.LevelWarn
			}

			logger.Log(r.Context(), level, "request completed",
				"request_id", GetRequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"bytes", rw.written,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		})
	}
}

// --- Recovery Middleware ---

// RecoveryMiddleware catches panics and returns a 500 JSON error.
func RecoveryMiddleware(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := GetRequestID(r.Context())
					logger.Error("panic recovered",
						"request_id", requestID,
						"error", fmt.Sprintf("%v", err),
						"method", r.Method,
						"path", r.URL.Path,
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, `{"error":"internal_server_error","message":"an unexpected error occurred","request_id":"%s"}`, requestID)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// --- CORS Middleware ---

// CORSMiddleware adds Cross-Origin Resource Sharing headers.
func CORSMiddleware(cfg *config.Config) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range cfg.CORSAllowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.CORSAllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.CORSAllowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// --- Metrics Middleware ---

// MetricsMiddleware records request counts and latencies.
func MetricsMiddleware(metricsHandler *metrics.Handler) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := newResponseWriter(w)

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			metricsHandler.RecordRequest(r.Method, r.URL.Path, rw.statusCode, duration)
		})
	}
}
