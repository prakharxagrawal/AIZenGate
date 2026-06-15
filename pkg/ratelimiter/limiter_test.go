package ratelimiter

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisLimiter_Allow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	limiter := NewRedisLimiter(client, logger)

	tests := []struct {
		name     string
		key      string
		limit    int
		window   time.Duration
		expected bool
	}{
		{"First request", "user1", 1, time.Second, true},
		{"Second request", "user1", 1, time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := limiter.Allow(context.Background(), tt.key, tt.limit, tt.window)
			if got != tt.expected {
				t.Errorf("Allow() = %v, want %v", got, tt.expected)
			}
		})
	}
}