package replication

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// LagMonitor tracks replication lag per replica.
type LagMonitor struct {
	mu      sync.RWMutex
	lags    map[string]*replicaLag
	threshold time.Duration
}

type replicaLag struct {
	replicaID   string
	lag         int64 // nanoseconds
	lastUpdated int64 // unix nano
	ackSeq      uint64
}

// NewLagMonitor creates a lag monitor.
func NewLagMonitor(threshold time.Duration) *LagMonitor {
	return &LagMonitor{
		lags:      make(map[string]*replicaLag),
		threshold: threshold,
	}
}

// UpdateLag records an ACK from replicaID at sequence seq.
func (m *LagMonitor) UpdateLag(replicaID string, seq uint64, primarySeq uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lag := int64(primarySeq) - int64(seq)
	if lag < 0 {
		lag = 0
	}
	l, ok := m.lags[replicaID]
	if !ok {
		l = &replicaLag{replicaID: replicaID}
		m.lags[replicaID] = l
	}
	atomic.StoreInt64(&l.lag, lag)
	atomic.StoreInt64(&l.lastUpdated, time.Now().UnixNano())
	atomic.StoreUint64(&l.ackSeq, seq)
}

// IsLagging returns true if replicaID lag exceeds threshold.
func (m *LagMonitor) IsLagging(replicaID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.lags[replicaID]
	if !ok {
		return false
	}
	return atomic.LoadInt64(&l.lag) > int64(m.threshold)
}

// LagReport returns a summary of all replica lags.
func (m *LagMonitor) LagReport() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	report := make(map[string]int64, len(m.lags))
	for id, l := range m.lags {
		report[id] = atomic.LoadInt64(&l.lag)
	}
	return report
}

// QuorumACK returns true if at least quorum replicas have ACKed seq.
func (m *LagMonitor) QuorumACK(seq uint64, quorum int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, l := range m.lags {
		if atomic.LoadUint64(&l.ackSeq) >= seq {
			count++
		}
	}
	return count >= quorum
}

// FailoverCoordinator manages leader election on primary failure.
type FailoverCoordinator struct {
	mu        sync.Mutex
	leader    string
	replicas  []string
	isLeader  bool
}

// NewFailoverCoordinator creates a failover coordinator.
func NewFailoverCoordinator(replicas []string) *FailoverCoordinator {
	return &FailoverCoordinator{replicas: replicas}
}

// ElectLeader selects a new leader from available replicas.
func (f *FailoverCoordinator) ElectLeader(available []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(available) == 0 {
		return "", fmt.Errorf("no available replicas for election")
	}
	// Simple: pick first available (in production, use Raft)
	f.leader = available[0]
	f.isLeader = true
	return f.leader, nil
}

// Leader returns the current leader.
func (f *FailoverCoordinator) Leader() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.leader
}
