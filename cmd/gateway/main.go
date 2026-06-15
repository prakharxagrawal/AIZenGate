package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"github.com/zengate-ai/zengate/internal/ai"
	"github.com/zengate-ai/zengate/internal/auth"
	"github.com/zengate-ai/zengate/internal/config"
	"github.com/zengate-ai/zengate/internal/controlplane"
	"github.com/zengate-ai/zengate/internal/health"
	"github.com/zengate-ai/zengate/internal/metrics"
	"github.com/zengate-ai/zengate/internal/proxy"
	"github.com/zengate-ai/zengate/internal/ratelimit"
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

	// Initialize etcd configuration client (Control Plane)
	cpClient, err := controlplane.NewClient(cfg.EtcdEndpoints, 5*time.Second)
	if err != nil {
		slog.Error("failed to create etcd client", "error", err)
		os.Exit(1)
	}
	defer cpClient.Close()

	// Start configuration watcher (handles hot reloads) with a 3-second timeout context
	startCtx, startCancel := context.WithTimeout(context.Background(), 3*time.Second)
	startErr := cpClient.Start(startCtx)
	startCancel()
	if startErr != nil {
		slog.Warn("failed to connect to etcd cluster, dynamic configuration disabled (will use baseline fallbacks)", "endpoints", cfg.EtcdEndpoints, "error", startErr)
	}

	// Initialize rate limiter (Redis sliding-window with in-memory Token Bucket fallback)
	var limiter ratelimit.Limiter
	if cfg.RateLimitEnabled {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("failed to parse Redis URL", "url", cfg.RedisURL, "error", err)
			os.Exit(1)
		}
		rdb := redis.NewClient(opt)

		// Ping redis to check connectivity
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			slog.Warn("failed to connect to Redis, falling back to local in-memory rate limiting", "url", cfg.RedisURL, "error", err)
			limiter = ratelimit.NewTokenBucketLimiter(10.0, 20.0) // 10 req/s refill, 20 capacity
		} else {
			slog.Info("connected to Redis for sliding window rate limiting", "url", cfg.RedisURL)
			redisLimiter, err := ratelimit.NewRedisSlidingWindowLimiter(rdb)
			if err != nil {
				slog.Error("failed to initialize Redis rate limiter script", "error", err)
				os.Exit(1)
			}
			limiter = redisLimiter
		}
		pingCancel()
	} else {
		slog.Info("distributed rate limiting disabled, using local in-memory token bucket")
		limiter = ratelimit.NewTokenBucketLimiter(10.0, 20.0)
	}

	// Create Go AI Brain
	brain := ai.NewBrain(cfg)

	// Create the main HTTP mux
	mux := http.NewServeMux()

	// Health check endpoint
	healthHandler := health.NewHandler(cfg)
	mux.HandleFunc("GET /health", healthHandler.ServeHTTP)
	mux.HandleFunc("GET /healthz", healthHandler.ServeHTTP)

	// Prometheus metrics endpoint
	metricsHandler := metrics.NewHandler()
	mux.HandleFunc("GET /metrics", metricsHandler.ServeHTTP)

	// Control Plane API endpoints
	cpHandler := controlplane.NewHandler(cpClient)
	mux.Handle("/api/v1/policies", cpHandler)

	// Config Translator endpoint
	translatorHandler := ai.NewTranslatorHandler(brain, cpClient)
	mux.Handle("/api/v1/policies/translate", translatorHandler)

	// Reverse proxy — catch-all for all other routes
	proxyHandler, err := proxy.NewHandler(cfg, metricsHandler)
	if err != nil {
		slog.Error("failed to create proxy handler", "error", err)
		os.Exit(1)
	}
	mux.Handle("/", proxyHandler)

	// Background loop to watch for dynamic upstream updates from Self-Healer
	go func() {
		cli := cpClient.GetEtcdClient()
		if cli == nil {
			slog.Info("etcd client is nil, setting up in-memory updater for upstream target updates")
			cpClient.SetUpstreamUpdateCallback(func(newTargetStr string) {
				newTarget, err := url.Parse(newTargetStr)
				if err != nil {
					slog.Error("failed to parse updated upstream URL from local callback", "url", newTargetStr, "error", err)
					return
				}
				proxyHandler.UpdateTarget(newTarget)
			})
			return
		}

		watchChan := cli.Watch(context.Background(), "/zengate/upstream")
		slog.Info("started upstream target configuration watcher", "key", "/zengate/upstream")

		for wresp := range watchChan {
			for _, ev := range wresp.Events {
				if ev.Type == clientv3.EventTypePut {
					newTargetStr := string(ev.Kv.Value)
					newTarget, err := url.Parse(newTargetStr)
					if err != nil {
						slog.Error("failed to parse updated upstream URL from etcd", "url", newTargetStr, "error", err)
						continue
					}
					proxyHandler.UpdateTarget(newTarget)
				}
			}
		}
	}()

	// Start background micro-agents if enabled
	var analyzer *ai.TrafficAnalyzer
	var healer *ai.SelfHealer

	if cfg.AIEnabled {
		slog.Info("AI features enabled, starting background micro-agents")

		analyzer = ai.NewTrafficAnalyzer(brain, cpClient, metricsHandler, 10*time.Second)
		analyzer.Start()

		healer = ai.NewSelfHealer(brain, cpClient, metricsHandler, 5*time.Second, 5.0)
		healer.Start()
	}

	// Build the middleware chain
	authMiddleware := auth.NewJWTMiddleware(cfg.JWTSecret)
	rateLimitMiddleware := ratelimit.NewRateLimitMiddleware(limiter, cpClient, metricsHandler)

	handler := proxy.Chain(
		mux,
		proxy.RecoveryMiddleware(logger),
		proxy.RequestIDMiddleware(),
		proxy.LoggingMiddleware(logger),
		proxy.CORSMiddleware(cfg),
		authMiddleware.Handler,
		rateLimitMiddleware.Handler,
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

		if analyzer != nil {
			slog.Info("stopping background traffic analyzer agent")
			analyzer.Stop()
		}
		if healer != nil {
			slog.Info("stopping background self-healing agent")
			healer.Stop()
		}

		slog.Info("draining connections", "timeout", cfg.ShutdownTimeout)
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		slog.Info("server stopped gracefully")
	}
}
