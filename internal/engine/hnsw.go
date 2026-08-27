package engine

import (
	"encoding/binary"
	"math"
	"sort"
	"sync"
)

const (
	M               = 16
	M0              = 32
	DefaultEfSearch = 80
	EfConstruct     = 200
	hnswShards      = 8
	// Standard HNSW level distribution: P(level >= n) = M^-n.
	// 1/ln(M) with M=16 equals 1/(4*ln(2)).
	Mult = 1 / (4 * math.Ln2)
)

type HNSWGraph struct {
	mu         sync.RWMutex
	Nodes      map[int]*Node // Row ID -> Node (exported for checksummed persistence)
	byID       []*Node
	EntryPoint int
	MaxLayer   int
	Size       int
	EfSearch   int
}

type Node struct {
	ID    int
	Layer int
	Edges map[int][]int
}

type searchScratch struct {
	stamp      uint32
	visited    []uint32
	candidates maxHeap
	nearest    minHeap
}

var scratchPool = sync.Pool{New: func() any { return &searchScratch{} }}

func NewHNSWGraph() *HNSWGraph {
	return &HNSWGraph{
		Nodes:      make(map[int]*Node),
		EntryPoint: -1,
		MaxLayer:   -1,
		Size:       0,
		EfSearch:   DefaultEfSearch,
	}
}

func (g *HNSWGraph) node(id int) *Node {
	if id >= 0 && id < len(g.byID) {
		if n := g.byID[id]; n != nil {
			return n
		}
	}
	return g.Nodes[id]
}

func (g *HNSWGraph) attachNode(node *Node) {
	if node.ID >= cap(g.byID) {
		n := cap(g.byID)
		if n < 1024 {
			n = 1024
		}
		for n <= node.ID {
			n *= 2
		}
		grown := make([]*Node, n)
		copy(grown, g.byID)
		g.byID = grown
	}
	if node.ID >= len(g.byID) {
		g.byID = g.byID[:node.ID+1]
	}
	g.byID[node.ID] = node
	g.Nodes[node.ID] = node
}

func (g *HNSWGraph) reindex() {
	maxID := -1
	for id := range g.Nodes {
		if id > maxID {
			maxID = id
		}
	}
	if maxID < 0 {
		g.byID = nil
		return
	}
	g.byID = make([]*Node, maxID+1)
	for id, node := range g.Nodes {
		g.byID[id] = node
	}
}

func acquireScratch(size int) *searchScratch {
	s := scratchPool.Get().(*searchScratch)
	s.reset(size)
	return s
}

func releaseScratch(s *searchScratch) {
	if cap(s.visited) > 1<<22 {
		return
	}
	s.candidates = s.candidates[:0]
	s.nearest = s.nearest[:0]
	scratchPool.Put(s)
}

func (s *searchScratch) reset(size int) {
	s.stamp++
	if s.stamp == 0 {
		for i := range s.visited {
			s.visited[i] = 0
		}
		s.stamp = 1
	}
	if size < 1 {
		size = 1
	}
	if cap(s.visited) < size {
		s.visited = make([]uint32, size)
	} else {
		s.visited = s.visited[:cap(s.visited)]
	}
	s.candidates = s.candidates[:0]
	s.nearest = s.nearest[:0]
}

func (s *searchScratch) mark(id int) bool {
	if id < 0 {
		return true
	}
	if id >= len(s.visited) {
		n := cap(s.visited)
		if n < 16 {
			n = 16
		}
		for n <= id {
			n *= 2
		}
		grown := make([]uint32, n)
		copy(grown, s.visited)
		s.visited = grown
	}
	if s.visited[id] == s.stamp {
		return true
	}
	s.visited[id] = s.stamp
	return false
}

func (g *HNSWGraph) Insert(row int, mmapSlice []byte, dim int, rowInv []float32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	l := deterministicLevel(row)
	newNode := &Node{
		ID:    row,
		Layer: l,
		Edges: make(map[int][]int, l+1),
	}
	g.attachNode(newNode)

	if g.Size == 0 {
		g.EntryPoint = row
		g.MaxLayer = l
		g.Size++
		return
	}

	scratch := acquireScratch(g.Size + 2)
	defer releaseScratch(scratch)
	q := sq8Query{mmap: mmapSlice, dim: dim, rowInv: rowInv, queryRow: row}

	ep := g.EntryPoint
	maxLayer := g.MaxLayer

	for lc := maxLayer; lc > l; lc-- {
		scratch.reset(len(g.byID) + 1)
		found := g.searchLayer(q, []int{ep}, 1, lc, nil, scratch)
		if len(found) > 0 {
			ep = found[0].ID
		}
	}
	entryPoints := []int{ep}

	for lc := min(maxLayer, l); lc >= 0; lc-- {
		scratch.reset(len(g.byID) + 1)
		ef := EfConstruct
		if lc > 0 {
			ef = 16
		}
		neighbors := g.searchLayer(q, entryPoints, ef, lc, nil, scratch)

		maxM := M
		if lc == 0 {
			maxM = M0
		}
		edges := selectClosestNeighbors(row, neighbors, maxM)
		newNode.Edges[lc] = edges

		for _, neighborID := range edges {
			neighbor := g.node(neighborID)
			if neighbor == nil {
				continue
			}
			if neighbor.Edges[lc] == nil {
				neighbor.Edges[lc] = make([]int, 0, maxM)
			}
			revLimit := maxM
			if lc == 0 {
				revLimit = maxM * 2
			}
			neighbor.Edges[lc] = insertNeighborID(
				neighborID, neighbor.Edges[lc], row, revLimit, mmapSlice, dim, rowInv,
			)
		}

		if len(neighbors) > 0 {
			entryPoints = []int{neighbors[0].ID}
		}
	}

	if l > maxLayer {
		for lc := maxLayer + 1; lc <= l; lc++ {
			if _, ok := newNode.Edges[lc]; !ok {
				newNode.Edges[lc] = []int{}
			}
		}
		g.MaxLayer = l
		g.EntryPoint = row
	}
	g.Size++
}

func deterministicLevel(row int) int {
	x := uint64(row+1) + 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	u := float64((x>>11)+1) / float64(uint64(1)<<53)
	level := int(-math.Log(u) * Mult)
	if level < 0 {
		return 0
	}
	return level
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (g *HNSWGraph) Search(q8 []int8, qInv float32, mmapSlice []byte, dim int, k int, filter func(id int) bool, rowInv []float32) []intNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.Size == 0 || k <= 0 {
		return nil
	}
	ep := g.EntryPoint
	maxLayer := g.MaxLayer
	scratch := acquireScratch(len(g.byID) + 1)
	defer releaseScratch(scratch)
	q := sq8Query{q8: q8, qInv: qInv, mmap: mmapSlice, dim: dim, rowInv: rowInv, queryRow: -1}

	for lc := maxLayer; lc > 0; lc-- {
		scratch.reset(len(g.byID) + 1)
		found := g.searchLayer(q, []int{ep}, 1, lc, nil, scratch)
		if len(found) > 0 {
			ep = found[0].ID
		}
	}

	ef := g.EfSearch
	if ef <= 0 {
		ef = DefaultEfSearch
	}
	if k > ef {
		ef = k
	}

	scratch.reset(len(g.byID) + 1)
	results := g.searchLayer(q, []int{ep}, ef, 0, filter, scratch)
	if len(results) > k {
		results = results[:k]
	}
	return results
}

func (g *HNSWGraph) SetEfSearch(value int) {
	g.mu.Lock()
	if value < 1 {
		value = 1
	}
	g.EfSearch = value
	g.mu.Unlock()
}

func insertNeighborID(owner int, edges []int, candidate int, limit int, mmapSlice []byte, dim int, rowInv []float32) []int {
	if candidate == owner {
		return edges
	}
	for _, edge := range edges {
		if edge == candidate {
			return edges
		}
	}
	score := cosineSQ8Rows(mmapSlice, dim, owner, candidate, rowInv)
	position := sort.Search(len(edges), func(i int) bool {
		return cosineSQ8Rows(mmapSlice, dim, owner, edges[i], rowInv) <= score
	})
	if len(edges) >= limit && position == len(edges) {
		return edges
	}
	edges = append(edges, 0)
	copy(edges[position+1:], edges[position:])
	edges[position] = candidate
	if len(edges) > limit {
		edges = edges[:limit]
	}
	return edges
}

func selectClosestNeighbors(queryRow int, candidates []intNode, limit int) []int {
	result := make([]int, 0, limit)
	for _, candidate := range candidates {
		if candidate.ID == queryRow {
			continue
		}
		exists := false
		for _, id := range result {
			if id == candidate.ID {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		result = append(result, candidate.ID)
		if len(result) == limit {
			break
		}
	}
	return result
}

type intNode struct {
	ID    int
	Score float32
}

type maxHeap []intNode

func (h *maxHeap) push(n intNode) {
	*h = append(*h, n)
	h.up(len(*h) - 1)
}

func (h *maxHeap) pop() intNode {
	old := *h
	n := old[0]
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	if len(*h) > 0 {
		(*h)[0] = last
		h.down(0)
	}
	return n
}

func (h maxHeap) up(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if h[p].Score >= h[i].Score {
			break
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
}

func (h maxHeap) down(i int) {
	n := len(h)
	for {
		l := 2*i + 1
		if l >= n {
			break
		}
		j := l
		if r := l + 1; r < n && h[r].Score > h[l].Score {
			j = r
		}
		if h[i].Score >= h[j].Score {
			break
		}
		h[i], h[j] = h[j], h[i]
		i = j
	}
}

type minHeap []intNode

func (h *minHeap) push(n intNode) {
	*h = append(*h, n)
	h.up(len(*h) - 1)
}

func (h *minHeap) pop() intNode {
	old := *h
	n := old[0]
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	if len(*h) > 0 {
		(*h)[0] = last
		h.down(0)
	}
	return n
}

func (h minHeap) up(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if h[p].Score <= h[i].Score {
			break
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
}

func (h minHeap) down(i int) {
	n := len(h)
	for {
		l := 2*i + 1
		if l >= n {
			break
		}
		j := l
		if r := l + 1; r < n && h[r].Score < h[l].Score {
			j = r
		}
		if h[i].Score <= h[j].Score {
			break
		}
		h[i], h[j] = h[j], h[i]
		i = j
	}
}

type sq8Query struct {
	q8       []int8
	qInv     float32
	mmap     []byte
	dim      int
	rowInv   []float32
	queryRow int
}

func (q sq8Query) score(row int) float32 {
	if q.queryRow >= 0 {
		return cosineSQ8Rows(q.mmap, q.dim, q.queryRow, row, q.rowInv)
	}
	return cosineSQ8Query(q.q8, q.qInv, q.mmap, q.dim, row, q.rowInv)
}

func (g *HNSWGraph) searchLayer(q sq8Query, entryPoints []int, ef int, lc int, filter func(id int) bool, scratch *searchScratch) []intNode {
	if ef < 1 {
		ef = 1
	}
	for _, entryPoint := range entryPoints {
		if g.node(entryPoint) == nil || scratch.mark(entryPoint) {
			continue
		}
		score := q.score(entryPoint)
		candidate := intNode{ID: entryPoint, Score: score}
		scratch.candidates.push(candidate)
		scratch.nearest.push(candidate)
	}
	for len(scratch.nearest) > ef {
		scratch.nearest.pop()
	}

	for len(scratch.candidates) > 0 {
		c := scratch.candidates.pop()
		if len(scratch.nearest) >= ef && c.Score < scratch.nearest[0].Score {
			break
		}

		node := g.node(c.ID)
		if node == nil {
			continue
		}
		for _, neighbor := range node.Edges[lc] {
			if scratch.mark(neighbor) {
				continue
			}
			nScore := q.score(neighbor)
			if len(scratch.nearest) < ef || nScore > scratch.nearest[0].Score {
				candidate := intNode{ID: neighbor, Score: nScore}
				scratch.candidates.push(candidate)
				scratch.nearest.push(candidate)
				if len(scratch.nearest) > ef {
					scratch.nearest.pop()
				}
			}
		}
	}

	results := make([]intNode, 0, len(scratch.nearest))
	for len(scratch.nearest) > 0 {
		candidate := scratch.nearest.pop()
		if filter == nil || filter(candidate.ID) {
			results = append(results, candidate)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func cosineSimilarityRaw(query []float32, mmapSlice []byte, row int, dim int) float32 {
	return cosineF32Query(query, l2Norm(query), mmapSlice, dim, row, []float32{})
}

func l2Norm(vector []float32) float32 {
	var squared float32
	for _, value := range vector {
		squared += value * value
	}
	return float32(math.Sqrt(float64(squared)))
}

func cosineF32Query(query []float32, queryNorm float32, mmapSlice []byte, dim, row int, rowInv []float32) float32 {
	inv := float32(0)
	if row >= 0 && row < len(rowInv) {
		inv = rowInv[row]
	} else {
		inv = mmapRowInv(mmapSlice, dim, row)
	}
	if queryNorm == 0 || inv == 0 {
		return 0
	}
	off := row * (dim + 8)
	if off < 0 || off+dim > len(mmapSlice) {
		return 0
	}
	var dot float32
	i := 0
	rowBytes := mmapSlice[off:]
	for ; i+7 < dim; i += 8 {
		dot += query[i]*float32(int8(rowBytes[i])) +
			query[i+1]*float32(int8(rowBytes[i+1])) +
			query[i+2]*float32(int8(rowBytes[i+2])) +
			query[i+3]*float32(int8(rowBytes[i+3])) +
			query[i+4]*float32(int8(rowBytes[i+4])) +
			query[i+5]*float32(int8(rowBytes[i+5])) +
			query[i+6]*float32(int8(rowBytes[i+6])) +
			query[i+7]*float32(int8(rowBytes[i+7]))
	}
	for ; i < dim; i++ {
		dot += query[i] * float32(int8(rowBytes[i]))
	}
	return (dot * inv) / queryNorm
}

func invScale(scale, reconNorm float32) float32 {
	if reconNorm <= 0 {
		return 0
	}
	return scale / float32(math.Sqrt(float64(reconNorm)))
}

func mmapRowInv(mmapSlice []byte, dim, row int) float32 {
	rowSize := dim + 8
	off := row * rowSize
	if off < 0 || off+rowSize > len(mmapSlice) {
		return 0
	}
	scale := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[off+dim : off+dim+4]))
	recon := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[off+dim+4 : off+dim+8]))
	return invScale(scale, recon)
}

func cosineSQ8Rows(mmapSlice []byte, dim, rowA, rowB int, rowInv []float32) float32 {
	if rowA < 0 || rowB < 0 || rowA >= len(rowInv) || rowB >= len(rowInv) {
		return 0
	}
	invA, invB := rowInv[rowA], rowInv[rowB]
	if invA == 0 || invB == 0 {
		return 0
	}
	rowSize := dim + 8
	offA := rowA * rowSize
	offB := rowB * rowSize
	if offA < 0 || offB < 0 || offA+dim > len(mmapSlice) || offB+dim > len(mmapSlice) {
		return 0
	}
	return float32(int8DotBytes(mmapSlice[offA:], mmapSlice[offB:], dim)) * invA * invB
}

func cosineSQ8Query(q8 []int8, qInv float32, mmapSlice []byte, dim, row int, rowInv []float32) float32 {
	if qInv == 0 || row < 0 || row >= len(rowInv) || rowInv[row] == 0 {
		return 0
	}
	off := row * (dim + 8)
	if off < 0 || off+dim > len(mmapSlice) {
		return 0
	}
	return float32(int8DotQuery(q8, mmapSlice[off:], dim)) * qInv * rowInv[row]
}

func int8DotBytes(a, b []byte, dim int) int32 {
	var dot int32
	i := 0
	for ; i+7 < dim; i += 8 {
		dot += int32(int8(a[i]))*int32(int8(b[i])) +
			int32(int8(a[i+1]))*int32(int8(b[i+1])) +
			int32(int8(a[i+2]))*int32(int8(b[i+2])) +
			int32(int8(a[i+3]))*int32(int8(b[i+3])) +
			int32(int8(a[i+4]))*int32(int8(b[i+4])) +
			int32(int8(a[i+5]))*int32(int8(b[i+5])) +
			int32(int8(a[i+6]))*int32(int8(b[i+6])) +
			int32(int8(a[i+7]))*int32(int8(b[i+7]))
	}
	for ; i < dim; i++ {
		dot += int32(int8(a[i])) * int32(int8(b[i]))
	}
	return dot
}

func int8DotQuery(q []int8, row []byte, dim int) int32 {
	var dot int32
	i := 0
	for ; i+7 < dim; i += 8 {
		dot += int32(q[i])*int32(int8(row[i])) +
			int32(q[i+1])*int32(int8(row[i+1])) +
			int32(q[i+2])*int32(int8(row[i+2])) +
			int32(q[i+3])*int32(int8(row[i+3])) +
			int32(q[i+4])*int32(int8(row[i+4])) +
			int32(q[i+5])*int32(int8(row[i+5])) +
			int32(q[i+6])*int32(int8(row[i+6])) +
			int32(q[i+7])*int32(int8(row[i+7]))
	}
	for ; i < dim; i++ {
		dot += int32(q[i]) * int32(int8(row[i]))
	}
	return dot
}
