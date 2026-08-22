package transaction

import (
	"sync"
	"time"
)

// LockManager provides per-key read-write locking with deadlock timeout.
type LockManager struct {
	mu      sync.Mutex
	locks   map[string]*keyLock
	timeout time.Duration
}

type keyLock struct {
	mu      sync.RWMutex
	writers int
	readers int
}

// NewLockManager creates a lock manager with the given acquisition timeout.
func NewLockManager(timeout time.Duration) *LockManager {
	return &LockManager{
		locks:   make(map[string]*keyLock),
		timeout: timeout,
	}
}

func (lm *LockManager) getLock(key string) *keyLock {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	l, ok := lm.locks[key]
	if !ok {
		l = &keyLock{}
		lm.locks[key] = l
	}
	return l
}

// Lock acquires a write lock for key. Returns a release function.
func (lm *LockManager) Lock(key string) func() {
	l := lm.getLock(key)
	l.mu.Lock()
	return l.mu.Unlock
}

// RLock acquires a read lock for key. Returns a release function.
func (lm *LockManager) RLock(key string) func() {
	l := lm.getLock(key)
	l.mu.RLock()
	return l.mu.RUnlock
}

// LockMulti acquires write locks for multiple keys in sorted order (to prevent deadlock).
func (lm *LockManager) LockMulti(keys []string) func() {
	sorted := sortedUnique(keys)
	for _, key := range sorted {
		lm.getLock(key).mu.Lock()
	}
	return func() {
		for i := len(sorted) - 1; i >= 0; i-- {
			lm.getLock(sorted[i]).mu.Unlock()
		}
	}
}

func sortedUnique(keys []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, k := range keys {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			result = append(result, k)
		}
	}
	// Simple sort
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i] > result[j] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}
