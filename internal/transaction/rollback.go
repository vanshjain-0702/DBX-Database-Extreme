package transaction

import (
	"sync"

	"github.com/dbx/dbx/internal/engine"
)

// RollbackLog records changes made during a transaction for rollback.
type RollbackLog struct {
	mu      sync.Mutex
	entries []rollbackEntry
}

type rollbackEntry struct {
	Key      string
	OldValue interface{}
	Existed  bool
}

// NewRollbackLog creates a rollback log.
func NewRollbackLog() *RollbackLog {
	return &RollbackLog{}
}

// Record saves the old state of key before modification.
func (r *RollbackLog) Record(key string, entry *engine.Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry != nil {
		r.entries = append(r.entries, rollbackEntry{
			Key:      key,
			OldValue: entry.Value,
			Existed:  true,
		})
	} else {
		r.entries = append(r.entries, rollbackEntry{
			Key:     key,
			Existed: false,
		})
	}
}

// Rollback applies all recorded rollbacks to the KV store.
func (r *RollbackLog) Rollback(kv *engine.KVStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Apply in reverse order
	for i := len(r.entries) - 1; i >= 0; i-- {
		e := r.entries[i]
		if !e.Existed {
			kv.Delete(e.Key)
		}
	}
	r.entries = nil
}

// Clear clears the rollback log (called on commit).
func (r *RollbackLog) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = nil
}
