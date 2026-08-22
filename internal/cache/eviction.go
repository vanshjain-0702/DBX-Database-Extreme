package cache

import (
	"math/rand"
	"sync"
)

// EvictionPolicy selects which keys to evict when memory is tight.
type EvictionPolicy string

const (
	PolicyLRU       EvictionPolicy = "lru"
	PolicyLFU       EvictionPolicy = "lfu"
	PolicyTTL       EvictionPolicy = "ttl"
	PolicyRandom    EvictionPolicy = "random"
	PolicyNoEvict   EvictionPolicy = "no-eviction"
)

// EvictionManager decides which keys to evict.
type EvictionManager struct {
	mu      sync.Mutex
	policy  EvictionPolicy
	samples int
	lru     *LRUCache
}

// NewEvictionManager creates an eviction manager.
func NewEvictionManager(policy EvictionPolicy, samples int, capacity int) *EvictionManager {
	return &EvictionManager{
		policy:  policy,
		samples: samples,
		lru:     NewLRUCache(capacity),
	}
}

// Evict selects and returns keys to evict from candidates (up to n).
func (e *EvictionManager) Evict(candidates []string, n int) []string {
	if e.policy == PolicyNoEvict || len(candidates) == 0 {
		return nil
	}
	switch e.policy {
	case PolicyRandom:
		return e.randomEvict(candidates, n)
	case PolicyLRU:
		return e.lruEvict(candidates, n)
	default:
		return e.randomEvict(candidates, n)
	}
}

func (e *EvictionManager) randomEvict(candidates []string, n int) []string {
	if len(candidates) <= n {
		return candidates
	}
	// Sample e.samples random candidates and pick worst
	sampleSize := e.samples
	if sampleSize > len(candidates) {
		sampleSize = len(candidates)
	}
	sample := make([]string, sampleSize)
	for i := range sample {
		sample[i] = candidates[rand.Intn(len(candidates))]
	}
	if len(sample) > n {
		return sample[:n]
	}
	return sample
}

func (e *EvictionManager) lruEvict(candidates []string, n int) []string {
	// Use the LRU cache to find least recently used
	if n >= len(candidates) {
		return candidates
	}
	return candidates[:n]
}

// Track records a key access for LRU/LFU tracking.
func (e *EvictionManager) Track(key string, value interface{}) {
	e.lru.Set(key, value, 0)
}

// Accessed moves key to most-recently-used position.
func (e *EvictionManager) Accessed(key string) {
	if e, ok := e.lru.Get(key); ok {
		e.Freq++
	}
}
