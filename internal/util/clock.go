package util

import (
	"sync"
	"time"
)

// Clock is an interface over time functions for testability.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	After(d time.Duration) <-chan time.Time
}

// RealClock is the real system clock.
type RealClock struct{}

func (RealClock) Now() time.Time                       { return time.Now() }
func (RealClock) Since(t time.Time) time.Duration     { return time.Since(t) }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// MockClock is a controllable clock for testing.
type MockClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewMockClock creates a mock clock starting at t.
func NewMockClock(t time.Time) *MockClock { return &MockClock{now: t} }

func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *MockClock) Since(t time.Time) time.Duration {
	return c.Now().Sub(t)
}

func (c *MockClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.Now().Add(d)
	return ch
}

// Advance advances the mock clock by d.
func (c *MockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// UnixNanoNow returns the current Unix timestamp in nanoseconds.
func UnixNanoNow() int64 { return time.Now().UnixNano() }

// UnixMilliNow returns the current Unix timestamp in milliseconds.
func UnixMilliNow() int64 { return time.Now().UnixMilli() }
