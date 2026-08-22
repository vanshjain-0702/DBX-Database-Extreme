// Package transaction provides WATCH/MULTI/EXEC, MVCC, and lock management.
package transaction

import (
	"sync"
	"sync/atomic"
	"time"
)

// Version is a monotonic version number for MVCC.
type Version uint64

// VersionedValue stores a value with its write version and timestamp.
type VersionedValue struct {
	Value     interface{}
	Version   Version
	Timestamp time.Time
	Deleted   bool
}

// MVCCStore maintains a version history for each key.
type MVCCStore struct {
	mu      sync.RWMutex
	history map[string][]*VersionedValue
	current atomic.Uint64 // current global version
	maxVersionsPerKey int
}

// NewMVCCStore creates a new MVCC store.
func NewMVCCStore(maxVersions int) *MVCCStore {
	if maxVersions <= 0 {
		maxVersions = 64
	}
	return &MVCCStore{
		history:           make(map[string][]*VersionedValue),
		maxVersionsPerKey: maxVersions,
	}
}

// CurrentVersion returns the current global version.
func (m *MVCCStore) CurrentVersion() Version {
	return Version(m.current.Load())
}

// NextVersion atomically increments and returns the next version.
func (m *MVCCStore) NextVersion() Version {
	return Version(m.current.Add(1))
}

// Write records a new versioned value for key.
func (m *MVCCStore) Write(key string, value interface{}, deleted bool) Version {
	v := m.NextVersion()
	vv := &VersionedValue{
		Value:     value,
		Version:   v,
		Timestamp: time.Now(),
		Deleted:   deleted,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hist := m.history[key]
	hist = append(hist, vv)
	// Trim history if too long
	if len(hist) > m.maxVersionsPerKey {
		hist = hist[len(hist)-m.maxVersionsPerKey:]
	}
	m.history[key] = hist
	return v
}

// ReadAt returns the most recent value at or before snapshotVersion.
func (m *MVCCStore) ReadAt(key string, snapshotVersion Version) *VersionedValue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hist := m.history[key]
	// Walk backwards to find the latest version <= snapshotVersion
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Version <= snapshotVersion {
			return hist[i]
		}
	}
	return nil
}

// ReadLatest returns the most recent committed value.
func (m *MVCCStore) ReadLatest(key string) *VersionedValue {
	return m.ReadAt(key, m.CurrentVersion())
}

// HistoryAt returns all versions of key at or before snapshotVersion.
func (m *MVCCStore) HistoryAt(key string, snapshotVersion Version) []*VersionedValue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*VersionedValue
	for _, vv := range m.history[key] {
		if vv.Version <= snapshotVersion {
			result = append(result, vv)
		}
	}
	return result
}

// GC removes versions older than minVersion.
func (m *MVCCStore) GC(minVersion Version) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, hist := range m.history {
		var keep []*VersionedValue
		for _, vv := range hist {
			if vv.Version >= minVersion {
				keep = append(keep, vv)
			}
		}
		if len(keep) == 0 {
			delete(m.history, key)
		} else {
			m.history[key] = keep
		}
	}
}
