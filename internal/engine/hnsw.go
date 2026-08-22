package engine

import (
	"encoding/binary"
	"math"
	"math/rand"
	"sort"
	"sync"
)

const (
	M           = 16 // Max edges per node
	M0          = 32 // Max edges for layer 0
	EfSearch    = 64
	EfConstruct = 64
	Mult        = 1 / math.Ln2 // Level generation multiplier
)

type HNSWGraph struct {
	mu         sync.RWMutex
	Nodes      map[int]*Node // Row ID -> Node
	EntryPoint int           // Row ID of the entry point
	MaxLayer   int
	Size       int
}

type Node struct {
	ID    int
	Layer int
	// Edges: Layer -> list of neighbor IDs
	Edges map[int][]int
}

func NewHNSWGraph() *HNSWGraph {
	return &HNSWGraph{
		Nodes:      make(map[int]*Node),
		EntryPoint: -1,
		MaxLayer:   -1,
		Size:       0,
	}
}

func (g *HNSWGraph) Insert(row int, query []float32, mmapSlice []byte, dim int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	l := int(-math.Log(rand.Float64()) * Mult)
	newNode := &Node{
		ID:    row,
		Layer: l,
		Edges: make(map[int][]int),
	}
	g.Nodes[row] = newNode

	if g.Size == 0 {
		g.EntryPoint = row
		g.MaxLayer = l
		g.Size++
		return
	}

	ep := g.EntryPoint
	maxLayer := g.MaxLayer

	// Phase 1: Greedily find entry point for insertion layer
	for lc := maxLayer; lc > l; lc-- {
		ep = g.searchLayerRawGreedy(query, mmapSlice, dim, ep, lc)
	}

	// Phase 2: Insert into all layers from min(maxLayer, l) down to 0
	for lc := min(maxLayer, l); lc >= 0; lc-- {
		neighbors := g.searchLayerRaw(query, mmapSlice, dim, ep, EfConstruct, lc, nil)
		
		edges := make([]int, 0, M)
		for i := 0; i < len(neighbors) && i < M; i++ {
			edges = append(edges, neighbors[i].ID)
		}
		newNode.Edges[lc] = edges

		for _, neighborID := range edges {
			neighbor := g.Nodes[neighborID]
			if neighbor.Edges[lc] == nil {
				neighbor.Edges[lc] = make([]int, 0, M)
			}
			nEdges := neighbor.Edges[lc]
			nEdges = append(nEdges, row)
			
			maxM := M
			if lc == 0 {
				maxM = M0
			}
			if len(nEdges) > maxM {
				nEdges = nEdges[:maxM]
			}
			neighbor.Edges[lc] = nEdges
		}
		
		if len(neighbors) > 0 {
			ep = neighbors[0].ID
		}
	}

	if l > maxLayer {
		g.MaxLayer = l
		g.EntryPoint = row
	}
	g.Size++
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (g *HNSWGraph) Search(query []float32, mmapSlice []byte, dim int, k int, filter func(id int) bool) []intNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ep := g.EntryPoint
	maxLayer := g.MaxLayer

	if g.Size == 0 {
		return nil
	}

	for lc := maxLayer; lc > 0; lc-- {
		ep = g.searchLayerRawGreedy(query, mmapSlice, dim, ep, lc)
	}

	ef := EfSearch
	if k > ef {
		ef = k
	}

	results := g.searchLayerRaw(query, mmapSlice, dim, ep, ef, 0, filter)
	if len(results) > k {
		results = results[:k]
	}
	return results
}

func (g *HNSWGraph) searchLayerRawGreedy(query []float32, mmapSlice []byte, dim int, ep int, lc int) int {
	visited := make(map[int]bool)
	visited[ep] = true
	bestNode := ep
	bestScore := cosineSimilarityRaw(query, mmapSlice, ep, dim)

	for {
		changed := false
		node := g.Nodes[bestNode]
		for _, neighbor := range node.Edges[lc] {
			if !visited[neighbor] {
				visited[neighbor] = true
				score := cosineSimilarityRaw(query, mmapSlice, neighbor, dim)
				if score > bestScore {
					bestScore = score
					bestNode = neighbor
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return bestNode
}

type intNode struct {
	ID    int
	Score float32
}

func (g *HNSWGraph) searchLayerRaw(query []float32, mmapSlice []byte, dim int, ep int, ef int, lc int, filter func(id int) bool) []intNode {
	visited := make(map[int]bool)
	visited[ep] = true

	epScore := cosineSimilarityRaw(query, mmapSlice, ep, dim)
	candidates := []intNode{{ID: ep, Score: epScore}}
	results := []intNode{}
	
	if filter == nil || filter(ep) {
		results = append(results, intNode{ID: ep, Score: epScore})
	}

	for len(candidates) > 0 {
		cIndex := 0
		cScore := float32(-2.0)
		for i, c := range candidates {
			if c.Score > cScore {
				cScore = c.Score
				cIndex = i
			}
		}
		c := candidates[cIndex]

		candidates[cIndex] = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]

		worstResultScore := float32(2.0)
		for _, r := range results {
			if r.Score < worstResultScore {
				worstResultScore = r.Score
			}
		}

		if c.Score < worstResultScore && len(results) == ef {
			break
		}

		node := g.Nodes[c.ID]
		for _, neighbor := range node.Edges[lc] {
			if !visited[neighbor] {
				visited[neighbor] = true
				nScore := cosineSimilarityRaw(query, mmapSlice, neighbor, dim)

				worstScore := float32(2.0)
				for _, r := range results {
					if r.Score < worstScore {
						worstScore = r.Score
					}
				}

				if nScore > worstScore || len(results) < ef {
					candidates = append(candidates, intNode{ID: neighbor, Score: nScore})
					
					if filter == nil || filter(neighbor) {
						results = append(results, intNode{ID: neighbor, Score: nScore})

						if len(results) > ef {
							wIndex := 0
							wScore := float32(2.0)
							for i, r := range results {
								if r.Score < wScore {
									wScore = r.Score
									wIndex = i
								}
							}
							results[wIndex] = results[len(results)-1]
							results = results[:len(results)-1]
						}
					}
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

func cosineSimilarityRaw(query []float32, mmapSlice []byte, row int, dim int) float32 {
	off := row * (dim + 8)
	if off < 0 || off+dim+8 > len(mmapSlice) {
		return 0
	}
	
	// Read scale and norm from the end of the vector
	scale := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[off+dim : off+dim+4]))
	normB := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[off+dim+4 : off+dim+8]))
	
	var dotProduct, normA float32
	
	i := 0
	for ; i+7 < dim; i += 8 {
		a0, a1, a2, a3, a4, a5, a6, a7 := query[i], query[i+1], query[i+2], query[i+3], query[i+4], query[i+5], query[i+6], query[i+7]
		b0 := float32(int8(mmapSlice[off+i]))
		b1 := float32(int8(mmapSlice[off+i+1]))
		b2 := float32(int8(mmapSlice[off+i+2]))
		b3 := float32(int8(mmapSlice[off+i+3]))
		b4 := float32(int8(mmapSlice[off+i+4]))
		b5 := float32(int8(mmapSlice[off+i+5]))
		b6 := float32(int8(mmapSlice[off+i+6]))
		b7 := float32(int8(mmapSlice[off+i+7]))
		
		dotProduct += a0*b0 + a1*b1 + a2*b2 + a3*b3 + a4*b4 + a5*b5 + a6*b6 + a7*b7
		normA += a0*a0 + a1*a1 + a2*a2 + a3*a3 + a4*a4 + a5*a5 + a6*a6 + a7*a7
	}
	for ; i < dim; i++ {
		a := query[i]
		b := float32(int8(mmapSlice[off+i]))
		dotProduct += a * b
		normA += a * a
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	
	// Apply SQ8 scale to the dot product
	dotProduct *= scale
	
	return dotProduct / float32(math.Sqrt(float64(normA))*math.Sqrt(float64(normB)))
}
