// Package replication manages primary-replica replication.
package replication

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/persistence"
)

// PrimaryManager manages replication from a primary node.
type PrimaryManager struct {
	mu       sync.RWMutex
	replicas map[string]*ReplicaConn
	wal      *persistence.WAL
	kv       *engine.KVStore
}

// ReplicaConn represents a connected replica.
type ReplicaConn struct {
	ID       string
	Addr     string
	Conn     net.Conn
	LastACK  time.Time
	Lag      time.Duration
	mu       sync.Mutex
}

// NewPrimaryManager creates a replication primary manager.
func NewPrimaryManager(kv *engine.KVStore, wal *persistence.WAL) *PrimaryManager {
	return &PrimaryManager{
		replicas: make(map[string]*ReplicaConn),
		wal:      wal,
		kv:       kv,
	}
}

// AddReplica registers a new replica connection.
func (p *PrimaryManager) AddReplica(id, addr string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.replicas[id] = &ReplicaConn{
		ID:      id,
		Addr:    addr,
		Conn:    conn,
		LastACK: time.Now(),
	}
}

// RemoveReplica disconnects and removes a replica.
func (p *PrimaryManager) RemoveReplica(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r, ok := p.replicas[id]; ok {
		r.Conn.Close()
		delete(p.replicas, id)
	}
}

// BroadcastWAL sends a WAL record to all connected replicas.
func (p *PrimaryManager) BroadcastWAL(rec *persistence.WALRecord) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	data := fmt.Sprintf("*WAL:%d:%s:%s\n", rec.Type, rec.Key, rec.Value)
	for _, r := range p.replicas {
		r.mu.Lock()
		r.Conn.Write([]byte(data))
		r.mu.Unlock()
	}
}

// ReplicaCount returns the number of connected replicas.
func (p *PrimaryManager) ReplicaCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.replicas)
}

// Replicas returns snapshot of replica info.
func (p *PrimaryManager) Replicas() []*ReplicaConn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*ReplicaConn, 0, len(p.replicas))
	for _, r := range p.replicas {
		result = append(result, r)
	}
	return result
}
