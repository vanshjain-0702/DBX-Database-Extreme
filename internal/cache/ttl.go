package cache

import (
	"sync"
	"time"
)

// TTLStore tracks TTL expiry for keys.
type TTLStore struct {
	mu      sync.RWMutex
	expires map[string]int64 // key -> Unix nano expiry
}

// NewTTLStore creates a new TTL store.
func NewTTLStore() *TTLStore {
	return &TTLStore{expires: make(map[string]int64)}
}

// Set sets a TTL for key (in nanoseconds from now).
func (t *TTLStore) Set(key string, ttlNano int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ttlNano <= 0 {
		delete(t.expires, key)
		return
	}
	t.expires[key] = time.Now().UnixNano() + ttlNano
}

// Get returns the expiry time for key. Returns 0 if no TTL.
func (t *TTLStore) Get(key string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.expires[key]
}

// Delete removes a TTL entry.
func (t *TTLStore) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.expires, key)
}

// IsExpired returns true if key has an expired TTL.
func (t *TTLStore) IsExpired(key string) bool {
	exp := t.Get(key)
	return exp != 0 && time.Now().UnixNano() > exp
}

// TTL returns remaining TTL in seconds. Returns -2 if expired/not set, -1 if no expiry.
func (t *TTLStore) TTL(key string) int64 {
	exp := t.Get(key)
	if exp == 0 {
		return -1
	}
	remaining := exp - time.Now().UnixNano()
	if remaining <= 0 {
		return -2
	}
	return remaining / int64(time.Second)
}

// Expired returns all expired keys.
func (t *TTLStore) Expired() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := time.Now().UnixNano()
	var expired []string
	for k, exp := range t.expires {
		if exp != 0 && now > exp {
			expired = append(expired, k)
		}
	}
	return expired
}
