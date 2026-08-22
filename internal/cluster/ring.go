// Package cluster provides consistent hash ring, shard management, and cluster topology.
package cluster

import (
	"fmt"
	"sort"
	"sync"

	"github.com/dbx/dbx/internal/util"
)

const TotalSlots = 16384

// Node represents a cluster node.
type Node struct {
	ID      string
	Addr    string
	Slots   []SlotRange
	State   NodeState
	Role    string // "primary" or "replica"
}

// NodeState is the state of a cluster node.
type NodeState string

const (
	NodeOnline  NodeState = "online"
	NodeOffline NodeState = "offline"
	NodeFailing NodeState = "failing"
)

// SlotRange defines an inclusive range of hash slots.
type SlotRange struct {
	Start int
	End   int
}

// Ring is a consistent hash ring for cluster slot allocation.
type Ring struct {
	mu      sync.RWMutex
	nodes   []*Node
	nodeMap map[string]*Node    // nodeID -> Node
	slotMap [TotalSlots]string  // slot -> nodeID
}

// NewRing creates an empty hash ring.
func NewRing() *Ring {
	return &Ring{nodeMap: make(map[string]*Node)}
}

// AddNode adds a node to the ring.
func (r *Ring) AddNode(n *Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes = append(r.nodes, n)
	r.nodeMap[n.ID] = n
	r.rebalance()
}

// RemoveNode removes a node and rebalances slots.
func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var filtered []*Node
	for _, n := range r.nodes {
		if n.ID != nodeID {
			filtered = append(filtered, n)
		}
	}
	r.nodes = filtered
	delete(r.nodeMap, nodeID)
	r.rebalance()
}

// NodeForSlot returns the node owning the given slot.
func (r *Ring) NodeForSlot(slot int) *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if slot < 0 || slot >= TotalSlots {
		return nil
	}
	nodeID := r.slotMap[slot]
	return r.nodeMap[nodeID]
}

// NodeForKey returns the node responsible for the given key.
func (r *Ring) NodeForKey(key string) *Node {
	return r.NodeForSlot(int(util.HashSlot(key)))
}

// rebalance redistributes slots evenly across nodes.
func (r *Ring) rebalance() {
	n := len(r.nodes)
	if n == 0 {
		return
	}
	sort.Slice(r.nodes, func(i, j int) bool {
		return r.nodes[i].ID < r.nodes[j].ID
	})
	slotsPerNode := TotalSlots / n
	for i, node := range r.nodes {
		start := i * slotsPerNode
		end := start + slotsPerNode
		if i == n-1 {
			end = TotalSlots
		}
		node.Slots = []SlotRange{{Start: start, End: end - 1}}
		for s := start; s < end; s++ {
			r.slotMap[s] = node.ID
		}
	}
}

// Nodes returns all nodes.
func (r *Ring) Nodes() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Node, len(r.nodes))
	copy(result, r.nodes)
	return result
}

// SlotMap returns a copy of the slot->nodeID map for diagnostics.
func (r *Ring) SlotInfo() map[string][]SlotRange {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := make(map[string][]SlotRange)
	for _, n := range r.nodes {
		m[n.ID] = n.Slots
	}
	return m
}

// Redirect produces a MOVED redirect response for the wrong-shard case.
func Redirect(slot int, node *Node) string {
	if node == nil {
		return fmt.Sprintf("MOVED %d (no owner)", slot)
	}
	return fmt.Sprintf("MOVED %d %s", slot, node.Addr)
}
