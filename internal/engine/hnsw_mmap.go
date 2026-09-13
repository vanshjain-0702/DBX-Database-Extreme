package engine

import (
	"os"
	"sync"
	"unsafe"

	"github.com/edsrzf/mmap-go"
)

// HNSW nodes have max M edges at layer > 0, and M0 edges at layer 0.
const (
	mmapM0       = 32
	mmapM        = 16
	mmapMaxLayer = 14
	
	// Layout per Node:
	// 0..3:   Layer (uint32)
	// 4..7:   CountL0 (uint32)
	// 8..135: EdgesL0 (32 * int32) = 128 bytes
	// For each layer 1..14 (14 total layers):
	//   Count (uint32) - 4 bytes
	//   Edges (16 * int32) - 64 bytes
	// Total per layer = 68 bytes
	// Total layers 1..14 = 14 * 68 = 952 bytes
	// Total per node = 4 + 4 + 128 + 952 = 1088 bytes
	mmapNodeSize = 1088
)

type MMapHNSWGraph struct {
	mu         sync.RWMutex
	file       *os.File
	mmap       mmap.MMap
	EntryPoint int
	MaxLayer   int
	Size       int
	capacity   int
}

func NewMMapHNSWGraph(path string, capacity int) (*MMapHNSWGraph, error) {
	if capacity < 1000 {
		capacity = 1000
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	
	fileSize := int64(capacity) * mmapNodeSize
	if err := f.Truncate(fileSize); err != nil {
		f.Close()
		return nil, err
	}
	
	m, err := mmap.Map(f, mmap.RDWR, 0)
	if err != nil {
		f.Close()
		return nil, err
	}
	
	return &MMapHNSWGraph{
		file:       f,
		mmap:       m,
		EntryPoint: -1,
		MaxLayer:   -1,
		Size:       0,
		capacity:   capacity,
	}, nil
}

func (g *MMapHNSWGraph) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.mmap != nil {
		g.mmap.Unmap()
		g.mmap = nil
	}
	if g.file != nil {
		err := g.file.Close()
		g.file = nil
		return err
	}
	return nil
}

// EnsureCapacity expands the mmap file if the node ID exceeds current capacity.
func (g *MMapHNSWGraph) EnsureCapacity(id int) error {
	if id < g.capacity {
		return nil
	}
	newCap := g.capacity * 2
	if newCap <= id {
		newCap = id + 1000
	}
	g.mmap.Unmap()
	g.file.Truncate(int64(newCap) * mmapNodeSize)
	m, err := mmap.Map(g.file, mmap.RDWR, 0)
	if err != nil {
		return err
	}
	g.mmap = m
	g.capacity = newCap
	return nil
}

// Helper to access raw bytes as pointers
func (g *MMapHNSWGraph) ptrLayer(id int) *uint32 {
	return (*uint32)(unsafe.Pointer(&g.mmap[id*mmapNodeSize]))
}

func (g *MMapHNSWGraph) ptrCountL0(id int) *uint32 {
	return (*uint32)(unsafe.Pointer(&g.mmap[id*mmapNodeSize+4]))
}

func (g *MMapHNSWGraph) sliceEdgesL0(id int) []int32 {
	start := id*mmapNodeSize + 8
	return unsafe.Slice((*int32)(unsafe.Pointer(&g.mmap[start])), mmapM0)
}

func (g *MMapHNSWGraph) ptrCountL(id, layer int) *uint32 {
	// layers 1..14
	start := id*mmapNodeSize + 136 + (layer-1)*68
	return (*uint32)(unsafe.Pointer(&g.mmap[start]))
}

func (g *MMapHNSWGraph) sliceEdgesL(id, layer int) []int32 {
	start := id*mmapNodeSize + 136 + (layer-1)*68 + 4
	return unsafe.Slice((*int32)(unsafe.Pointer(&g.mmap[start])), mmapM)
}

// API for VADD
func (g *MMapHNSWGraph) AddNode(id int, layer int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.EnsureCapacity(id); err != nil {
		return err
	}
	if layer > mmapMaxLayer {
		layer = mmapMaxLayer
	}
	*g.ptrLayer(id) = uint32(layer)
	
	if g.EntryPoint == -1 || layer > g.MaxLayer {
		g.EntryPoint = id
		g.MaxLayer = layer
	}
	g.Size++
	return nil
}

func (g *MMapHNSWGraph) GetLayer(id int) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if id >= g.capacity {
		return 0
	}
	return int(*g.ptrLayer(id))
}

func (g *MMapHNSWGraph) AddEdge(from, to, layer int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if from >= g.capacity {
		return
	}
	if layer == 0 {
		countPtr := g.ptrCountL0(from)
		count := int(*countPtr)
		if count < mmapM0 {
			edges := g.sliceEdgesL0(from)
			edges[count] = int32(to)
			*countPtr = uint32(count + 1)
		}
	} else if layer <= mmapMaxLayer {
		countPtr := g.ptrCountL(from, layer)
		count := int(*countPtr)
		if count < mmapM {
			edges := g.sliceEdgesL(from, layer)
			edges[count] = int32(to)
			*countPtr = uint32(count + 1)
		}
	}
}

func (g *MMapHNSWGraph) GetEdges(id, layer int) []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if id >= g.capacity {
		return nil
	}
	var res []int
	if layer == 0 {
		count := int(*g.ptrCountL0(id))
		edges := g.sliceEdgesL0(id)
		res = make([]int, count)
		for i := 0; i < count; i++ {
			res[i] = int(edges[i])
		}
	} else if layer <= mmapMaxLayer {
		count := int(*g.ptrCountL(id, layer))
		edges := g.sliceEdgesL(id, layer)
		res = make([]int, count)
		for i := 0; i < count; i++ {
			res[i] = int(edges[i])
		}
	}
	return res
}
