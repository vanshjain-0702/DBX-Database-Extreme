// Package engine is the DBX in-memory storage engine.
// It manages all data types in a sharded concurrent map.
package engine

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

const defaultNumShards = 256

// Entry holds a stored value along with metadata.
type Entry struct {
	Value     interface{}
	Type      protocol.DataType
	ExpiresAt int64 // Unix nano; 0 = no expiry
	Version   uint64
	CreatedAt int64
	UpdatedAt int64
}

// IsExpired returns true if the entry has expired.
func (e *Entry) IsExpired() bool {
	if e.ExpiresAt == 0 {
		return false
	}
	return time.Now().UnixNano() > e.ExpiresAt
}

// shard is a single partition of the KV map.
type shard struct {
	mu       sync.RWMutex
	data     map[string]*Entry
	expiries expiryHeap
}

type expiryItem struct {
	key      string
	deadline int64
	version  uint64
}

type expiryHeap []expiryItem

func (h expiryHeap) Len() int            { return len(h) }
func (h expiryHeap) Less(i, j int) bool  { return h[i].deadline < h[j].deadline }
func (h expiryHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *expiryHeap) Push(x interface{}) { *h = append(*h, x.(expiryItem)) }
func (h *expiryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// KVStore is the sharded in-memory key-value store.
type KVStore struct {
	shards    []*shard
	numShards int
	version   atomic.Uint64
}

// New creates a new KVStore with n shards.
func New(numShards int) *KVStore {
	if numShards <= 0 {
		numShards = defaultNumShards
	}
	kv := &KVStore{
		shards:    make([]*shard, numShards),
		numShards: numShards,
	}
	for i := range kv.shards {
		kv.shards[i] = &shard{data: make(map[string]*Entry)}
	}
	return kv
}

// shard returns the shard for key.
func (kv *KVStore) shard(key string) *shard {
	return kv.shards[util.ShardIndex(key, kv.numShards)]
}

// nextVersion returns a monotonically increasing version number.
func (kv *KVStore) nextVersion() uint64 {
	return kv.version.Add(1)
}

// Set stores a value with optional TTL in nanoseconds (0 = no expiry).
func (kv *KVStore) Set(key string, value interface{}, typ protocol.DataType, ttlNano int64) {
	s := kv.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixNano()
	var exp int64
	if ttlNano > 0 {
		exp = now + ttlNano
	}
	if e, ok := s.data[key]; ok {
		e.Value = value
		e.Type = typ
		e.ExpiresAt = exp
		e.Version = kv.nextVersion()
		e.UpdatedAt = now
	} else {
		s.data[key] = &Entry{
			Value:     value,
			Type:      typ,
			ExpiresAt: exp,
			Version:   kv.nextVersion(),
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	if exp > 0 {
		e := s.data[key]
		heap.Push(&s.expiries, expiryItem{key: key, deadline: exp, version: e.Version})
	}
}

// Get retrieves an entry by key. Returns nil if not found or expired.
func (kv *KVStore) Get(key string) *Entry {
	s := kv.shard(key)
	s.mu.RLock()
	e, ok := s.data[key]
	s.mu.RUnlock()
	if !ok || e.IsExpired() {
		return nil
	}
	return e
}

// GetForRead retrieves an entry with read lock held; caller must call the returned unlock func.
func (kv *KVStore) GetForRead(key string) (*Entry, func()) {
	s := kv.shard(key)
	s.mu.RLock()
	e, ok := s.data[key]
	if !ok || e.IsExpired() {
		s.mu.RUnlock()
		return nil, func() {}
	}
	return e, func() { s.mu.RUnlock() }
}

// GetForWrite retrieves an entry with write lock held; caller must call s.mu.Unlock().
func (kv *KVStore) GetForWrite(key string) (*Entry, func()) {
	s := kv.shard(key)
	s.mu.Lock()
	e := s.data[key]
	if e != nil && e.IsExpired() {
		delete(s.data, key)
		e = nil
	}
	return e, func() { s.mu.Unlock() }
}

// Delete removes a key. Returns true if the key existed.
func (kv *KVStore) Delete(keys ...string) int {
	deleted := 0
	for _, key := range keys {
		s := kv.shard(key)
		s.mu.Lock()
		if e, ok := s.data[key]; ok {
			closeVectorEntry(e)
			delete(s.data, key)
			deleted++
		}
		s.mu.Unlock()
	}
	return deleted
}

// Exists returns true if key exists and is not expired.
func (kv *KVStore) Exists(key string) bool {
	return kv.Get(key) != nil
}

// Type returns the data type of key, or TypeNone.
func (kv *KVStore) Type(key string) protocol.DataType {
	e := kv.Get(key)
	if e == nil {
		return protocol.TypeNone
	}
	return e.Type
}

// Expire sets a TTL (in seconds) on key. Returns true if key exists.
func (kv *KVStore) Expire(key string, seconds int64) bool {
	s := kv.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok || e.IsExpired() {
		return false
	}
	if seconds <= 0 {
		delete(s.data, key)
	} else {
		e.ExpiresAt = time.Now().UnixNano() + seconds*int64(time.Second)
		e.Version = kv.nextVersion()
		heap.Push(&s.expiries, expiryItem{key: key, deadline: e.ExpiresAt, version: e.Version})
	}
	return true
}

// TTL returns the remaining TTL for key in seconds.
// Returns -2 if key doesn't exist, -1 if no expiry.
func (kv *KVStore) TTL(key string) int64 {
	e := kv.Get(key)
	if e == nil {
		return -2
	}
	if e.ExpiresAt == 0 {
		return -1
	}
	remaining := e.ExpiresAt - time.Now().UnixNano()
	if remaining <= 0 {
		return -2
	}
	return remaining / int64(time.Second)
}

// Persist removes the expiry from key. Returns true if expiry was removed.
func (kv *KVStore) Persist(key string) bool {
	s := kv.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok || e.IsExpired() {
		return false
	}
	if e.ExpiresAt == 0 {
		return false
	}
	e.ExpiresAt = 0
	return true
}

// Keys returns all non-expired keys matching a simple glob pattern.
func (kv *KVStore) Keys(pattern string) []string {
	var keys []string
	for _, s := range kv.shards {
		s.mu.RLock()
		for k, e := range s.data {
			if !e.IsExpired() && matchGlob(pattern, k) {
				keys = append(keys, k)
			}
		}
		s.mu.RUnlock()
	}
	return keys
}

// DBSize returns the count of non-expired keys.
func (kv *KVStore) DBSize() int {
	count := 0
	for _, s := range kv.shards {
		s.mu.RLock()
		for _, e := range s.data {
			if !e.IsExpired() {
				count++
			}
		}
		s.mu.RUnlock()
	}
	return count
}

// KeyspaceStats returns the count of non-expired keys grouped by DataType.
func (kv *KVStore) KeyspaceStats() map[string]int {
	stats := make(map[string]int)
	for _, s := range kv.shards {
		s.mu.RLock()
		for _, e := range s.data {
			if !e.IsExpired() {
				stats[string(e.Type)]++
			}
		}
		s.mu.RUnlock()
	}
	return stats
}

// FlushAll removes all keys.
func (kv *KVStore) FlushAll() {
	for _, s := range kv.shards {
		s.mu.Lock()
		for _, e := range s.data {
			closeVectorEntry(e)
		}
		s.data = make(map[string]*Entry)
		s.mu.Unlock()
	}
}

func closeVectorEntry(e *Entry) {
	if e == nil || e.Type != protocol.TypeVector {
		return
	}
	if idx, ok := e.Value.(*MMapVectorIndex); ok && idx != nil {
		idx.Close()
		e.Value = nil
	}
}

// Rename renames key to newKey. Returns error if key doesn't exist.
func (kv *KVStore) Rename(key, newKey string) error {
	e := kv.Get(key)
	if e == nil {
		return util.ErrNotFound
	}
	kv.Delete(key)
	ttl := int64(0)
	if e.ExpiresAt > 0 {
		ttl = e.ExpiresAt - time.Now().UnixNano()
		if ttl <= 0 {
			return util.ErrNotFound
		}
	}
	kv.Set(newKey, e.Value, e.Type, ttl)
	return nil
}

// ExpiredKeys returns keys that have expired (for the expiration reaper).
func (kv *KVStore) ExpiredKeys() []string {
	var expired []string
	for _, s := range kv.shards {
		s.mu.RLock()
		for k, e := range s.data {
			if e.IsExpired() {
				expired = append(expired, k)
			}
		}
		s.mu.RUnlock()
	}
	return expired
}

// DeleteExpired removes expired keys from all shards.
func (kv *KVStore) DeleteExpired() int {
	return kv.DeleteExpiredLimit(0)
}

// DeleteExpiredLimit removes at most limit keys using per-shard deadline
// heaps. A zero limit drains every currently due key.
func (kv *KVStore) DeleteExpiredLimit(limit int) int {
	count := 0
	now := time.Now().UnixNano()
	for _, s := range kv.shards {
		s.mu.Lock()
		for s.expiries.Len() > 0 && (limit <= 0 || count < limit) {
			item := s.expiries[0]
			if item.deadline > now {
				break
			}
			heap.Pop(&s.expiries)
			entry := s.data[item.key]
			if entry == nil || entry.Version != item.version || entry.ExpiresAt != item.deadline {
				continue
			}
			delete(s.data, item.key)
			count++
		}
		s.mu.Unlock()
		if limit > 0 && count >= limit {
			break
		}
	}
	return count
}

// Snapshot returns a copy of all non-expired entries for persistence.
func (kv *KVStore) Snapshot() map[string]*Entry {
	snap := make(map[string]*Entry)
	for _, s := range kv.shards {
		s.mu.RLock()
		for k, e := range s.data {
			if !e.IsExpired() {
				snap[k] = e
			}
		}
		s.mu.RUnlock()
	}
	return snap
}

// MemoryUsage returns conservative tenant-owned heap bytes for keys, entry
// metadata, and string values. Vector mmap usage is reported by VectorStore.
func (kv *KVStore) MemoryUsage() int64 {
	var total int64
	for _, s := range kv.shards {
		s.mu.RLock()
		for key, entry := range s.data {
			if entry.IsExpired() {
				continue
			}
			total += int64(len(key) + 96)
			if entry.Type == protocol.TypeString {
				if value, ok := entry.Value.([]byte); ok {
					total += int64(len(value))
				}
			}
		}
		total += int64(len(s.expiries)) * 48
		s.mu.RUnlock()
	}
	return total
}

// matchGlob performs simple Redis-style glob matching (* and ? supported).
func matchGlob(pattern, s string) bool {
	if pattern == "*" {
		return true
	}
	return globMatch(pattern, s)
}

func globMatch(pattern, str string) bool {
	p, s := 0, 0
	star := -1
	match := 0
	for s < len(str) {
		if p < len(pattern) && (pattern[p] == '?' || pattern[p] == str[s]) {
			p++
			s++
		} else if p < len(pattern) && pattern[p] == '*' {
			star = p
			match = s
			p++
		} else if star != -1 {
			p = star + 1
			match++
			s = match
		} else {
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
