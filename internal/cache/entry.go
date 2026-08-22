// Package cache provides TTL, eviction, and admission control for DBX.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// Entry is an LRU/LFU cache entry.
type Entry struct {
	Key       string
	Value     interface{}
	ExpiresAt int64 // Unix nano
	Freq      int   // for LFU
	Size      int64 // byte estimate
	elem      *list.Element
}

// IsExpired returns true if the entry has expired.
func (e *Entry) IsExpired() bool {
	return e.ExpiresAt != 0 && time.Now().UnixNano() > e.ExpiresAt
}

// LRUCache is a thread-safe LRU cache.
type LRUCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*Entry
	lru      *list.List
}

// NewLRUCache creates an LRU cache with given capacity.
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*Entry),
		lru:      list.New(),
	}
}

// Get retrieves an entry and bumps it to front (most recent).
func (c *LRUCache) Get(key string) (*Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if e.IsExpired() {
		c.lru.Remove(e.elem)
		delete(c.items, key)
		return nil, false
	}
	c.lru.MoveToFront(e.elem)
	return e, true
}

// Set adds/updates an entry, evicting LRU if over capacity.
func (c *LRUCache) Set(key string, value interface{}, ttlNano int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp int64
	if ttlNano > 0 {
		exp = time.Now().UnixNano() + ttlNano
	}
	if e, ok := c.items[key]; ok {
		c.lru.MoveToFront(e.elem)
		e.Value = value
		e.ExpiresAt = exp
		return
	}
	for len(c.items) >= c.capacity {
		c.evict()
	}
	e := &Entry{Key: key, Value: value, ExpiresAt: exp}
	e.elem = c.lru.PushFront(e)
	c.items[key] = e
}

// Delete removes an entry.
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.lru.Remove(e.elem)
		delete(c.items, key)
	}
}

// Len returns number of cached items.
func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *LRUCache) evict() {
	elem := c.lru.Back()
	if elem == nil {
		return
	}
	e := elem.Value.(*Entry)
	c.lru.Remove(elem)
	delete(c.items, e.Key)
}
