package security

import (
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter per client.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64 // tokens per second
	capacity float64
	enabled  bool
}

type bucket struct {
	tokens    float64
	lastRefill time.Time
}

// NewRateLimiter creates a rate limiter with the given rate (req/s) and burst capacity.
func NewRateLimiter(ratePerSec, burst int, enabled bool) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     float64(ratePerSec),
		capacity: float64(burst),
		enabled:  enabled,
	}
}

// Allow returns true if clientID is within rate limit.
func (r *RateLimiter) Allow(clientID string) bool {
	if !r.enabled {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[clientID]
	if !ok {
		b = &bucket{tokens: r.capacity, lastRefill: time.Now()}
		r.buckets[clientID] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * r.rate
	if b.tokens > r.capacity {
		b.tokens = r.capacity
	}
	b.lastRefill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Cleanup removes stale buckets older than maxAge.
func (r *RateLimiter) Cleanup(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, b := range r.buckets {
		if b.lastRefill.Before(cutoff) {
			delete(r.buckets, id)
		}
	}
}
