package util

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// Backoff is an exponential backoff with jitter.
type Backoff struct {
	Base    time.Duration
	Max     time.Duration
	Factor  float64
	Jitter  float64
	attempt int
}

// NewBackoff creates a new Backoff with sensible defaults.
func NewBackoff() *Backoff {
	return &Backoff{
		Base:   100 * time.Millisecond,
		Max:    30 * time.Second,
		Factor: 2.0,
		Jitter: 0.3,
	}
}

// Next returns the duration to wait before the next retry.
func (b *Backoff) Next() time.Duration {
	delay := float64(b.Base) * math.Pow(b.Factor, float64(b.attempt))
	if delay > float64(b.Max) {
		delay = float64(b.Max)
	}
	jitter := delay * b.Jitter * (rand.Float64()*2 - 1)
	delay += jitter
	if delay < 0 {
		delay = 0
	}
	b.attempt++
	return time.Duration(delay)
}

// Reset resets the attempt counter.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Wait sleeps for the next backoff duration, respecting context cancellation.
func (b *Backoff) Wait(ctx context.Context) error {
	d := b.Next()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// RetryWithBackoff retries fn up to maxAttempts times using exponential backoff.
func RetryWithBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	b := NewBackoff()
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			if err := b.Wait(ctx); err != nil {
				return err
			}
		}
	}
	return lastErr
}
