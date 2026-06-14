package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/health"
	"github.com/zengate-ai/zengate/internal/metrics"
	"github.com/zengate-ai/zengate/internal/proxy"
)

func main() {
	// Structured JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting ZenGate AI",
		"version", cfg.Version,
		"port", cfg.Port,
		"upstream", cfg.UpstreamURL,
	)

	// Create the main HTTP mux
	mux := http.NewServeMux()

	// Health check endpoint
	healthHandler := health.NewHandler(cfg)
	mux.HandleFunc("GET /health", healthHandler.ServeHTTP)
	mux.HandleFunc("GET /healthz", healthHandler.ServeHTTP)

	// Prometheus metrics endpoint
	metricsHandler := metrics.NewHandler()
	mux.HandleFunc("GET /metrics", metricsHandler.ServeHTTP)

	// Reverse proxy — catch-all for all other routes
	proxyHandler, err := proxy.NewHandler(cfg, metricsHandler)
	if err != nil {
		slog.Error("failed to create proxy handler", "error", err)
		os.Exit(1)
	}
	mux.Handle("/", proxyHandler)

	// Build the middleware chain
	handler := proxy.Chain(
		mux,
		proxy.RecoveryMiddleware(logger),
		proxy.RequestIDMiddleware(),
		proxy.LoggingMiddleware(logger),
		proxy.CORSMiddleware(cfg),
		proxy.MetricsMiddleware(metricsHandler),
	)

	// Create the HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("gateway listening", "addr", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case sig := <-shutdown:
		slog.Info("shutdown signal received", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		slog.Info("draining connections", "timeout", cfg.ShutdownTimeout)
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		slog.Info("server stopped gracefully")
	}
}
