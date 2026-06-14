package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucketLimiter(t *testing.T) {
	// Refills 10 tokens/sec, capacity of 2 tokens
	limiter := NewTokenBucketLimiter(10.0, 2.0)

	ctx := context.Background()

	// 1st request should be allowed (uses token from capacity)
	allowed, err := limiter.Allow(ctx, "client_1", 10, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected 1st request to be allowed")
	}

	// 2nd request should be allowed (uses token from capacity)
	allowed, err = limiter.Allow(ctx, "client_1", 10, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected 2nd request to be allowed")
	}

	// 3rd request should be blocked (capacity exceeded)
	allowed, err = limiter.Allow(ctx, "client_1", 10, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected 3rd request to be blocked")
	}

	// Wait 100ms for 1 token to refill (refill rate: 10/sec = 1 per 100ms)
	time.Sleep(110 * time.Millisecond)

	// 4th request should be allowed after refill
	allowed, err = limiter.Allow(ctx, "client_1", 10, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed after token bucket refill")
	}
}
