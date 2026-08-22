package orchestrator

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// HashRing implements Consistent Hashing for distributing keys across nodes.
type HashRing struct {
	mu           sync.RWMutex
	nodes        []string
	ring         []uint32
	nodeMap      map[uint32]string
	virtualNodes int
}

func NewHashRing(virtualNodes int) *HashRing {
	return &HashRing{
		nodes:        make([]string, 0),
		ring:         make([]uint32, 0),
		nodeMap:      make(map[uint32]string),
		virtualNodes: virtualNodes,
	}
}

func (h *HashRing) hashKey(key string) uint32 {
	b := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(b[:4])
}

// AddNode adds a new physical node to the hash ring with virtual nodes.
func (h *HashRing) AddNode(node string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nodes = append(h.nodes, node)
	for i := 0; i < h.virtualNodes; i++ {
		vNodeKey := fmt.Sprintf("%s-v%d", node, i)
		hash := h.hashKey(vNodeKey)
		h.ring = append(h.ring, hash)
		h.nodeMap[hash] = node
	}
	sort.Slice(h.ring, func(i, j int) bool { return h.ring[i] < h.ring[j] })
}

// GetNode mathematically selects the shard responsible for the given key.
func (h *HashRing) GetNode(key string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.ring) == 0 {
		return ""
	}

	hash := h.hashKey(key)

	// Binary search to find the first node hash >= key hash
	idx := sort.Search(len(h.ring), func(i int) bool {
		return h.ring[i] >= hash
	})

	// Wrap around to the first node if we went past the end of the ring
	if idx == len(h.ring) {
		idx = 0
	}

	return h.nodeMap[h.ring[idx]]
}
