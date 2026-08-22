package transaction

import (
	"sync"
)

// WatchSet tracks which keys a client is watching.
type WatchSet struct {
	mu      sync.RWMutex
	watches map[uint64]map[string]Version // clientID -> key -> version at watch time
}

// NewWatchSet creates a new WatchSet.
func NewWatchSet() *WatchSet {
	return &WatchSet{watches: make(map[uint64]map[string]Version)}
}

// Watch adds keys to watch for clientID at the given version.
func (w *WatchSet) Watch(clientID uint64, keys []string, version Version) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.watches[clientID] == nil {
		w.watches[clientID] = make(map[string]Version)
	}
	for _, k := range keys {
		w.watches[clientID][k] = version
	}
}

// Unwatch removes all watches for clientID.
func (w *WatchSet) Unwatch(clientID uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.watches, clientID)
}

// IsDirty returns true if any watched key for clientID has been modified since watch time.
func (w *WatchSet) IsDirty(clientID uint64, mvcc *MVCCStore) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	watches := w.watches[clientID]
	for key, watchedAt := range watches {
		latest := mvcc.ReadLatest(key)
		if latest != nil && latest.Version > watchedAt {
			return true
		}
	}
	return false
}

// NotifyWrite is called when a key is written; it checks if any client was watching it.
// Returns list of client IDs that were watching this key.
func (w *WatchSet) NotifyWrite(key string, currentVersion Version) []uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var dirty []uint64
	for clientID, keys := range w.watches {
		if watchedAt, ok := keys[key]; ok && currentVersion > watchedAt {
			dirty = append(dirty, clientID)
		}
	}
	return dirty
}
