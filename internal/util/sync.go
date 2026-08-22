package util

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// SpinLock is a simple spinlock (useful for very short critical sections).
type SpinLock struct {
	state int32
}

// Lock acquires the spinlock.
func (s *SpinLock) Lock() {
	for !atomic.CompareAndSwapInt32(&s.state, 0, 1) {
		runtime.Gosched()
	}
}

// Unlock releases the spinlock.
func (s *SpinLock) Unlock() {
	atomic.StoreInt32(&s.state, 0)
}

// KeyedMutex provides per-key mutexes for fine-grained locking.
type KeyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyMutexEntry
}

type keyMutexEntry struct {
	mu      sync.Mutex
	waiters int
}

// NewKeyedMutex creates a new KeyedMutex.
func NewKeyedMutex() *KeyedMutex {
	return &KeyedMutex{locks: make(map[string]*keyMutexEntry)}
}

// Lock acquires the mutex for key.
func (km *KeyedMutex) Lock(key string) {
	km.mu.Lock()
	e, ok := km.locks[key]
	if !ok {
		e = &keyMutexEntry{}
		km.locks[key] = e
	}
	e.waiters++
	km.mu.Unlock()
	e.mu.Lock()
}

// Unlock releases the mutex for key.
func (km *KeyedMutex) Unlock(key string) {
	km.mu.Lock()
	e := km.locks[key]
	e.waiters--
	if e.waiters == 0 {
		delete(km.locks, key)
	}
	km.mu.Unlock()
	e.mu.Unlock()
}

// WaitGroup is a re-exported sync.WaitGroup for convenience.
type WaitGroup = sync.WaitGroup

// Once is a re-exported sync.Once.
type Once = sync.Once

// AtomicBool is a thread-safe boolean.
type AtomicBool struct {
	v int32
}

// Set sets the bool.
func (a *AtomicBool) Set(v bool) {
	if v {
		atomic.StoreInt32(&a.v, 1)
	} else {
		atomic.StoreInt32(&a.v, 0)
	}
}

// Get returns the current bool value.
func (a *AtomicBool) Get() bool {
	return atomic.LoadInt32(&a.v) == 1
}

// AtomicInt64 is a thread-safe int64.
type AtomicInt64 struct {
	v int64
}

func (a *AtomicInt64) Add(delta int64) int64 { return atomic.AddInt64(&a.v, delta) }
func (a *AtomicInt64) Set(v int64)           { atomic.StoreInt64(&a.v, v) }
func (a *AtomicInt64) Get() int64            { return atomic.LoadInt64(&a.v) }
func (a *AtomicInt64) CAS(old, new int64) bool {
	return atomic.CompareAndSwapInt64(&a.v, old, new)
}
