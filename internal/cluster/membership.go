package cluster

import (
	"fmt"
	"sync"
	"time"
)

// Membership manages node join/leave and heartbeat.
type Membership struct {
	mu        sync.RWMutex
	ring      *Ring
	heartbeat map[string]time.Time
	timeout   time.Duration
}

// NewMembership creates a membership manager.
func NewMembership(ring *Ring, timeout time.Duration) *Membership {
	return &Membership{
		ring:      ring,
		heartbeat: make(map[string]time.Time),
		timeout:   timeout,
	}
}

// Join adds a node to the cluster.
func (m *Membership) Join(n *Node) {
	m.ring.AddNode(n)
	m.mu.Lock()
	m.heartbeat[n.ID] = time.Now()
	m.mu.Unlock()
}

// Leave removes a node from the cluster.
func (m *Membership) Leave(nodeID string) {
	m.ring.RemoveNode(nodeID)
	m.mu.Lock()
	delete(m.heartbeat, nodeID)
	m.mu.Unlock()
}

// Heartbeat records a heartbeat from a node.
func (m *Membership) Heartbeat(nodeID string) {
	m.mu.Lock()
	m.heartbeat[nodeID] = time.Now()
	m.mu.Unlock()
}

// FailedNodes returns nodes that have missed heartbeats.
func (m *Membership) FailedNodes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var failed []string
	cutoff := time.Now().Add(-m.timeout)
	for id, last := range m.heartbeat {
		if last.Before(cutoff) {
			failed = append(failed, id)
		}
	}
	return failed
}

// SlotAllocator manages the 16384 hash slot allocation.
type SlotAllocator struct {
	ring *Ring
}

// NewSlotAllocator creates a slot allocator.
func NewSlotAllocator(ring *Ring) *SlotAllocator {
	return &SlotAllocator{ring: ring}
}

// Allocate assigns slots evenly to nodes (called on ring changes).
func (s *SlotAllocator) Allocate() {
	s.ring.mu.Lock()
	defer s.ring.mu.Unlock()
	s.ring.rebalance()
}

// Owner returns the node ID owning a slot.
func (s *SlotAllocator) Owner(slot int) (string, error) {
	node := s.ring.NodeForSlot(slot)
	if node == nil {
		return "", fmt.Errorf("no owner for slot %d", slot)
	}
	return node.ID, nil
}

// Rebalancer handles slot migration during topology changes.
type Rebalancer struct {
	ring *Ring
}

// NewRebalancer creates a rebalancer.
func NewRebalancer(ring *Ring) *Rebalancer {
	return &Rebalancer{ring: ring}
}

// Rebalance triggers a ring rebalance.
func (r *Rebalancer) Rebalance() {
	r.ring.mu.Lock()
	defer r.ring.mu.Unlock()
	r.ring.rebalance()
}

// Redirect produces a MOVED redirect for wrong-shard requests.
type RedirectHandler struct {
	ring      *Ring
	selfID    string
	selfAddr  string
}

// NewRedirectHandler creates a redirect handler.
func NewRedirectHandler(ring *Ring, selfID, selfAddr string) *RedirectHandler {
	return &RedirectHandler{ring: ring, selfID: selfID, selfAddr: selfAddr}
}

// ShouldRedirect returns true + redirect string if key is not on this node.
func (r *RedirectHandler) ShouldRedirect(key string) (bool, string) {
	node := r.ring.NodeForKey(key)
	if node == nil || node.ID == r.selfID {
		return false, ""
	}
	return true, Redirect(0, node)
}
