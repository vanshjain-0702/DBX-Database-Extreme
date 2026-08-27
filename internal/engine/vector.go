package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
	"github.com/edsrzf/mmap-go"
)

// MMapVectorIndex uses a memory-mapped file for zero-GC, massive scale vector storage.
type MMapVectorIndex struct {
	file        *os.File
	mmap        mmap.MMap
	dim         int
	count       int
	idMap       map[string]int // maps vector ID to row index
	idList      []string       // maps row index to vector ID
	tombstones  []bool
	generations []uint64
	rowInv      []float32
	graphs      []*HNSWGraph
	searchJobs  chan shardSearchJob
	searchWG    sync.WaitGroup
	mu          sync.RWMutex
}

type shardSearchJob struct {
	graph  *HNSWGraph
	q8     []int8
	qInv   float32
	mmap   []byte
	dim    int
	k      int
	filter func(id int) bool
	rowInv []float32
	out    *[]intNode
	wg     *sync.WaitGroup
}

type vectorMetadata struct {
	Version     int      `json:"version"`
	Dim         int      `json:"dim"`
	IDs         []string `json:"ids"`
	Tombstones  []bool   `json:"tombstones"`
	Generations []uint64 `json:"generations"`
}

func NewMMapVectorIndex(path string, dim int) (*MMapVectorIndex, error) {
	return newMMapVectorIndex(path, dim, 1000)
}

func newMMapVectorIndex(path string, dim, capacity int) (*MMapVectorIndex, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	if capacity < 1000 {
		capacity = 1000
	}
	info, _ := f.Stat()
	if info.Size() == 0 {
		initialSize := int64(capacity * (dim + 8))
		f.Truncate(initialSize)
	}

	m, err := mmap.Map(f, mmap.RDWR, 0)
	if err != nil {
		f.Close()
		return nil, err
	}

	idx := &MMapVectorIndex{
		file:        f,
		mmap:        m,
		dim:         dim,
		idMap:       make(map[string]int),
		idList:      make([]string, 0),
		tombstones:  make([]bool, 0),
		generations: make([]uint64, 0),
		graphs:      newHNSWShards(),
	}
	idx.startSearchWorkers()
	metaPath := path + ".meta"
	if data, readErr := os.ReadFile(metaPath); readErr == nil {
		var meta vectorMetadata
		if err := json.Unmarshal(data, &meta); err != nil || meta.Dim != dim {
			idx.Close()
			return nil, fmt.Errorf("invalid vector metadata: %w", err)
		}
		if len(meta.IDs) > len(m)/(dim+8) {
			idx.Close()
			return nil, fmt.Errorf("vector metadata exceeds index capacity")
		}
		idx.idList = append(idx.idList, meta.IDs...)
		idx.count = len(meta.IDs)
		idx.tombstones = append(idx.tombstones, meta.Tombstones...)
		idx.generations = append(idx.generations, meta.Generations...)
		for len(idx.tombstones) < idx.count {
			idx.tombstones = append(idx.tombstones, false)
		}
		for len(idx.generations) < idx.count {
			idx.generations = append(idx.generations, 1)
		}
		for row, id := range idx.idList {
			idx.idMap[id] = row
		}
		idx.syncRowInv()
	}
	hnswPath := path + ".hnsw"
	if graphs, err := loadHNSWGraphs(hnswPath); err == nil && validateShards(graphs, idx.count) == nil {
		idx.graphs = graphs
	} else {
		// The graph is only written on Close, so a crash leaves rows in the mmap
		// with no graph. WAL replay does not repair this: replayed VADDs see the
		// id already in metadata and skip graph insertion, so every vector would
		// stay permanently unsearchable while VSEARCH still returned success.
		idx.rebuildGraph()
	}
	return idx, nil
}

func newHNSWShards() []*HNSWGraph {
	graphs := make([]*HNSWGraph, hnswShards)
	for i := range graphs {
		graphs[i] = NewHNSWGraph()
	}
	return graphs
}

func shardIndex(row int) int {
	if row < 0 {
		return 0
	}
	return row % hnswShards
}

func shardExpectedSize(count, shard int) int {
	if count <= shard {
		return 0
	}
	return (count - shard + hnswShards - 1) / hnswShards
}

func validateShards(graphs []*HNSWGraph, count int) error {
	if len(graphs) != hnswShards {
		return fmt.Errorf("HNSW shard count %d, want %d", len(graphs), hnswShards)
	}
	total := 0
	for shard, graph := range graphs {
		if graph == nil {
			return fmt.Errorf("missing HNSW shard %d", shard)
		}
		expected := shardExpectedSize(count, shard)
		if err := graph.Validate(expected); err != nil {
			return err
		}
		total += graph.Size
	}
	if total != count {
		return fmt.Errorf("HNSW shard sizes %d, metadata %d", total, count)
	}
	return nil
}

// rebuildGraph reinserts every persisted row into an empty HNSW graph.
func (idx *MMapVectorIndex) rebuildGraph() {
	idx.syncRowInv()
	idx.graphs = newHNSWShards()
	var wg sync.WaitGroup
	for shard := 0; shard < hnswShards; shard++ {
		wg.Add(1)
		go func(shard int) {
			defer wg.Done()
			graph := idx.graphs[shard]
			rowSize := idx.dim + 8
			for row := shard; row < idx.count; row += hnswShards {
				off := row * rowSize
				if off+rowSize > len(idx.mmap) {
					return
				}
				graph.Insert(row, idx.mmap, idx.dim, idx.rowInv)
			}
		}(shard)
	}
	wg.Wait()
}

func (idx *MMapVectorIndex) syncRowInv() {
	if cap(idx.rowInv) < idx.count {
		idx.rowInv = make([]float32, idx.count)
	} else {
		idx.rowInv = idx.rowInv[:idx.count]
	}
	for row := 0; row < idx.count; row++ {
		idx.rowInv[row] = mmapRowInv(idx.mmap, idx.dim, row)
	}
}

func (idx *MMapVectorIndex) setRowInv(row int) {
	if row < 0 {
		return
	}
	if row >= cap(idx.rowInv) {
		n := cap(idx.rowInv)
		if n < 1024 {
			n = 1024
		}
		for n <= row {
			n *= 2
		}
		grown := make([]float32, n)
		copy(grown, idx.rowInv)
		idx.rowInv = grown
	}
	if row >= len(idx.rowInv) {
		idx.rowInv = idx.rowInv[:row+1]
	}
	idx.rowInv[row] = mmapRowInv(idx.mmap, idx.dim, row)
}

func (idx *MMapVectorIndex) persistMetadata() error {
	return idx.writeMetadata(true)
}

func (idx *MMapVectorIndex) writeMetadata(durable bool) error {
	data, err := json.Marshal(vectorMetadata{
		Version: 2, Dim: idx.dim, IDs: idx.idList,
		Tombstones: idx.tombstones, Generations: idx.generations,
	})
	if err != nil {
		return err
	}
	path := idx.file.Name() + ".meta"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if durable {
		f, openErr := os.OpenFile(tmp, os.O_RDWR, 0600)
		if openErr != nil {
			os.Remove(tmp)
			return openErr
		}
		err = f.Sync()
		f.Close()
		if err != nil {
			os.Remove(tmp)
			return err
		}
	}
	os.Remove(path) // Windows fix for Access Denied on Rename
	return os.Rename(tmp, path)
}

func (idx *MMapVectorIndex) startSearchWorkers() {
	if idx.searchJobs != nil {
		return
	}
	jobs := make(chan shardSearchJob, hnswShards)
	idx.searchJobs = jobs
	idx.searchWG.Add(hnswShards)
	for i := 0; i < hnswShards; i++ {
		go func() {
			defer idx.searchWG.Done()
			for job := range jobs {
				*job.out = job.graph.Search(job.q8, job.qInv, job.mmap, job.dim, job.k, job.filter, job.rowInv)
				job.wg.Done()
			}
		}()
	}
}

func (idx *MMapVectorIndex) Close() {
	idx.mu.Lock()
	jobs := idx.searchJobs
	idx.searchJobs = nil
	if jobs != nil {
		close(jobs)
	}
	idx.mu.Unlock()
	if jobs != nil {
		idx.searchWG.Wait()
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.mmap != nil {
		_ = idx.writeMetadata(true)
		_ = idx.mmap.Flush()
	}
	if len(idx.graphs) > 0 && idx.file != nil {
		_ = saveHNSWGraphs(idx.file.Name()+".hnsw", idx.graphs)
	}
	if idx.mmap != nil {
		_ = idx.mmap.Unmap()
		idx.mmap = nil
	}
	if idx.file != nil {
		_ = idx.file.Sync()
		_ = idx.file.Close()
		idx.file = nil
	}
}

func (idx *MMapVectorIndex) ensureCapacity(neededBytes int) error {
	if neededBytes <= len(idx.mmap) {
		return nil
	}
	oldSize := len(idx.mmap)
	newSize := oldSize * 2
	if newSize < neededBytes {
		newSize = neededBytes
	}
	if err := idx.mmap.Flush(); err != nil {
		return err
	}
	if err := idx.mmap.Unmap(); err != nil {
		return err
	}
	idx.mmap = nil
	if err := idx.file.Truncate(int64(newSize)); err != nil {
		if restored, mapErr := mmap.Map(idx.file, mmap.RDWR, 0); mapErr == nil {
			idx.mmap = restored
		}
		return fmt.Errorf("grow vector file: %w", err)
	}
	remapped, err := mmap.Map(idx.file, mmap.RDWR, 0)
	if err != nil {
		_ = idx.file.Truncate(int64(oldSize))
		if restored, mapErr := mmap.Map(idx.file, mmap.RDWR, 0); mapErr == nil {
			idx.mmap = restored
		}
		return fmt.Errorf("remap vector file: %w", err)
	}
	idx.mmap = remapped
	return nil
}

// VectorStore provides vector operations.
type VectorStore struct {
	kv         *KVStore
	dataDir    string
	maxVectors int
	maxMemory  int64
}

const (
	maxVectorDimensions = 4096
	maxVectorIDBytes    = 512
	maxVectorBatch      = 1000
)

func NewVectorStore(kv *KVStore, dataDir string, maxVectors int) *VectorStore {
	os.MkdirAll(dataDir, 0755)
	return &VectorStore{kv: kv, dataDir: dataDir, maxVectors: maxVectors}
}

// SetMemoryLimit applies the tenant's shared no-eviction memory limit.
func (s *VectorStore) SetMemoryLimit(bytes int64) { s.maxMemory = bytes }

// ValidateAdd performs all deterministic checks before a vector mutation is
// appended to the WAL.
func (s *VectorStore) ValidateAdd(key, id string, vec []float32) error {
	if err := validateVector(id, vec); err != nil {
		return err
	}
	return s.validateBatchCapacity(key, len(vec), []string{id})
}

// ValidateAddBatch performs deterministic dimension, ID, count, and memory
// admission checks before WAL append.
func (s *VectorStore) ValidateAddBatch(key string, dim int, ids []string, vecs [][]float32) error {
	if len(ids) != len(vecs) || len(ids) == 0 || len(ids) > maxVectorBatch {
		return fmt.Errorf("invalid vector batch size")
	}
	if dim <= 0 || dim > maxVectorDimensions {
		return fmt.Errorf("vector dimension must be between 1 and %d", maxVectorDimensions)
	}
	for i := range ids {
		if len(vecs[i]) != dim {
			return fmt.Errorf("dimension mismatch in batch")
		}
		if err := validateVector(ids[i], vecs[i]); err != nil {
			return err
		}
	}
	return s.validateBatchCapacity(key, dim, ids)
}

func (s *VectorStore) validateBatchCapacity(key string, dim int, ids []string) error {
	currentUsage := s.kv.MemoryUsage() + s.MemoryUsage()
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil {
		return err
	}
	newIDs := make(map[string]struct{}, len(ids))
	additional := int64(0)
	currentCount := 0
	currentCapacityBytes := 0
	if idx != nil {
		idx.mu.RLock()
		defer idx.mu.RUnlock()
		if idx.dim != dim {
			return fmt.Errorf("dimension mismatch: expected %d, got %d", idx.dim, dim)
		}
		currentCount = idx.count
		currentCapacityBytes = len(idx.mmap)
		for _, id := range ids {
			if _, exists := idx.idMap[id]; !exists {
				newIDs[id] = struct{}{}
			}
		}
	} else {
		for _, id := range ids {
			newIDs[id] = struct{}{}
		}
		additional += int64(1000*(dim+8) + len(key) + 96)
	}
	if s.maxVectors > 0 && currentCount+len(newIDs) > s.maxVectors {
		return fmt.Errorf("tenant vector quota exceeded: maximum of %d vectors allowed", s.maxVectors)
	}
	for id := range newIDs {
		additional += int64(len(id) + 160)
	}
	neededBytes := (currentCount + len(newIDs)) * (dim + 8)
	if idx != nil && neededBytes > currentCapacityBytes {
		newCapacity := currentCapacityBytes * 2
		if newCapacity < neededBytes {
			newCapacity = neededBytes
		}
		additional += int64(newCapacity - currentCapacityBytes)
	}
	if s.maxMemory > 0 && currentUsage+additional > s.maxMemory {
		return fmt.Errorf("OOM tenant quota exceeded")
	}
	return nil
}

func (s *VectorStore) getOrCreate(key string, dim int) (*MMapVectorIndex, func(), error) {
	e, unlock := s.kv.GetForWrite(key)
	if e == nil {
		path := filepath.Join(s.dataDir, vectorIndexFilename(key))
		idx, err := newMMapVectorIndex(path, dim, s.maxVectors)
		if err != nil {
			unlock()
			return nil, func() {}, err
		}
		sh := s.kv.shard(key)
		sh.data[key] = &Entry{Value: idx, Type: protocol.TypeVector}
		return idx, unlock, nil
	}
	if e.Type != protocol.TypeVector {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	if e.Value == nil {
		path := filepath.Join(s.dataDir, vectorIndexFilename(key))
		idx, err := newMMapVectorIndex(path, dim, s.maxVectors)
		if err != nil {
			unlock()
			return nil, func() {}, err
		}
		e.Value = idx
	}
	return e.Value.(*MMapVectorIndex), unlock, nil
}

func vectorIndexFilename(key string) string {
	return fmt.Sprintf("%x.vec", sha256.Sum256([]byte(key)))
}

// CloseAll persists and closes every open vector index. Without this, the HNSW
// graph is never written during a normal shutdown and the next start has to
// rebuild it from scratch.
func (s *VectorStore) CloseAll() {
	for _, sh := range s.kv.shards {
		sh.mu.Lock()
		for _, e := range sh.data {
			if e.Type != protocol.TypeVector {
				continue
			}
			if idx, ok := e.Value.(*MMapVectorIndex); ok && idx != nil {
				idx.Close()
				e.Value = nil
			}
		}
		sh.mu.Unlock()
	}
}

func (s *VectorStore) getReadOnly(key string) (*MMapVectorIndex, func(), error) {
	e, unlock := s.kv.GetForRead(key)
	if e == nil {
		unlock()
		metaPath := filepath.Join(s.dataDir, vectorIndexFilename(key)+".meta")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, func() {}, nil
			}
			return nil, func() {}, err
		}
		var meta vectorMetadata
		if err := json.Unmarshal(data, &meta); err != nil || meta.Dim <= 0 {
			return nil, func() {}, fmt.Errorf("invalid vector metadata")
		}
		return s.getOrCreate(key, meta.Dim)
	}
	if e.Type != protocol.TypeVector {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	if e.Value == nil {
		return nil, unlock, nil
	}
	return e.Value.(*MMapVectorIndex), unlock, nil
}

// VAdd adds a vector to the mmap index.
func (s *VectorStore) VAdd(key string, id string, vec []float32) error {
	dim := len(vec)
	if err := validateVector(id, vec); err != nil {
		return err
	}

	idx, unlock, err := s.getOrCreate(key, dim)
	defer unlock()
	if err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.dim != dim {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", idx.dim, dim)
	}

	if s.maxVectors > 0 && idx.count >= s.maxVectors {
		if _, exists := idx.idMap[id]; !exists {
			return fmt.Errorf("tenant vector quota exceeded: maximum of %d vectors allowed", s.maxVectors)
		}
	}

	existingRow, exists := idx.idMap[id]
	neededCount := idx.count
	if !exists {
		neededCount++
	}
	if err := idx.ensureCapacity(neededCount * (dim + 8)); err != nil {
		return err
	}

	isNew := false
	row := idx.count
	if exists {
		row = existingRow
		if idx.tombstones[row] {
			idx.tombstones[row] = false
			idx.generations[row]++
		}
	} else {
		idx.idMap[id] = row
		idx.idList = append(idx.idList, id)
		idx.tombstones = append(idx.tombstones, false)
		idx.generations = append(idx.generations, 1)
		idx.count++
		isNew = true
	}

	off := row * (dim + 8)
	writeQuantized(idx.mmap[off:off+dim+8], vec)
	idx.setRowInv(row)

	if isNew {
		idx.graphs[shardIndex(row)].Insert(row, idx.mmap, dim, idx.rowInv)
	}
	if err := idx.writeMetadata(false); err != nil {
		return err
	}

	return nil
}

// VAddBatch adds multiple vectors to the mmap index in a single operation.
func (s *VectorStore) VAddBatch(key string, dim int, ids []string, vecs [][]float32) error {
	if len(ids) != len(vecs) {
		return fmt.Errorf("length of ids and vecs must match")
	}
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > maxVectorBatch {
		return fmt.Errorf("batch size exceeds maximum of %d", maxVectorBatch)
	}
	if dim <= 0 || dim > maxVectorDimensions {
		return fmt.Errorf("vector dimension must be between 1 and %d", maxVectorDimensions)
	}
	for i, vec := range vecs {
		if len(vec) != dim {
			return fmt.Errorf("dimension mismatch in batch")
		}
		if err := validateVector(ids[i], vec); err != nil {
			return err
		}
	}

	idx, unlock, err := s.getOrCreate(key, dim)
	defer unlock()
	if err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.dim != dim {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", idx.dim, dim)
	}

	newIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := idx.idMap[id]; !exists {
			newIDs[id] = struct{}{}
		}
	}
	newAdditions := len(newIDs)

	if s.maxVectors > 0 && idx.count+newAdditions > s.maxVectors {
		return fmt.Errorf("tenant vector quota exceeded: maximum of %d vectors allowed", s.maxVectors)
	}

	if err := idx.ensureCapacity((idx.count + newAdditions) * (dim + 8)); err != nil {
		return err
	}

	newRows := make([]int, 0, newAdditions)
	for i, id := range ids {
		vec := vecs[i]
		row := idx.count
		if existingRow, exists := idx.idMap[id]; exists {
			row = existingRow
			if idx.tombstones[row] {
				idx.tombstones[row] = false
				idx.generations[row]++
			}
		} else {
			idx.idMap[id] = row
			idx.idList = append(idx.idList, id)
			idx.tombstones = append(idx.tombstones, false)
			idx.generations = append(idx.generations, 1)
			idx.count++
			newRows = append(newRows, row)
		}

		off := row * (dim + 8)
		writeQuantized(idx.mmap[off:off+dim+8], vec)
		idx.setRowInv(row)
	}

	var wg sync.WaitGroup
	for shard := 0; shard < hnswShards; shard++ {
		wg.Add(1)
		go func(shard int) {
			defer wg.Done()
			graph := idx.graphs[shard]
			for _, row := range newRows {
				if shardIndex(row) != shard {
					continue
				}
				graph.Insert(row, idx.mmap, dim, idx.rowInv)
			}
		}(shard)
	}
	wg.Wait()
	return nil
}

func validateVector(id string, vec []float32) error {
	if id == "" || len(id) > maxVectorIDBytes {
		return fmt.Errorf("vector id must contain 1..%d bytes", maxVectorIDBytes)
	}
	if len(vec) == 0 || len(vec) > maxVectorDimensions {
		return fmt.Errorf("vector dimension must be between 1 and %d", maxVectorDimensions)
	}
	for _, value := range vec {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("vector contains a non-finite value")
		}
	}
	return nil
}

// MemoryUsage returns conservative tenant-owned vector bytes, including mmap
// capacity and ID metadata. HNSW overhead is estimated per persisted row.
func (s *VectorStore) MemoryUsage() int64 {
	var total int64
	for _, sh := range s.kv.shards {
		sh.mu.RLock()
		for _, entry := range sh.data {
			if entry.Type != protocol.TypeVector {
				continue
			}
			idx, ok := entry.Value.(*MMapVectorIndex)
			if !ok || idx == nil {
				continue
			}
			idx.mu.RLock()
			total += int64(len(idx.mmap))
			for _, id := range idx.idList {
				total += int64(len(id) + 32)
			}
			total += int64(idx.count * 16 * 8)
			idx.mu.RUnlock()
		}
		sh.mu.RUnlock()
	}
	return total
}

// VDel records a generation tombstone. The graph keeps the node for
// navigation, but searches never return it.
func (s *VectorStore) VDel(key, id string) (bool, error) {
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || idx == nil {
		return false, err
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	row, exists := idx.idMap[id]
	if !exists || idx.tombstones[row] {
		return false, nil
	}
	idx.tombstones[row] = true
	idx.generations[row]++
	if err := idx.persistMetadata(); err != nil {
		idx.tombstones[row] = false
		idx.generations[row]--
		return false, err
	}
	return true, nil
}

// HasLiveVector reports whether an ID can be tombstoned without mutating it.
func (s *VectorStore) HasLiveVector(key, id string) (bool, error) {
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || idx == nil {
		return false, err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	row, exists := idx.idMap[id]
	return exists && !idx.tombstones[row], nil
}

// TombstoneRatio returns deleted rows divided by all physical rows.
func (s *VectorStore) TombstoneRatio(key string) (float64, error) {
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || idx == nil {
		return 0, err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.count == 0 {
		return 0, nil
	}
	deleted := 0
	for _, tombstone := range idx.tombstones {
		if tombstone {
			deleted++
		}
	}
	return float64(deleted) / float64(idx.count), nil
}

// VCompact removes tombstoned rows and rebuilds the graph. The mmap capacity
// remains allocated, avoiding a non-atomic truncate/remap sequence.
func (s *VectorStore) VCompact(key string) (int, error) {
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || idx == nil {
		return 0, err
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	rowSize := idx.dim + 8
	activeIDs := make([]string, 0, idx.count)
	activeGenerations := make([]uint64, 0, idx.count)
	activeRows := make([][]byte, 0, idx.count)
	removed := 0
	for row, id := range idx.idList {
		if idx.tombstones[row] {
			removed++
			continue
		}
		off := row * rowSize
		activeIDs = append(activeIDs, id)
		activeGenerations = append(activeGenerations, idx.generations[row])
		activeRows = append(activeRows, append([]byte(nil), idx.mmap[off:off+rowSize]...))
	}
	if removed == 0 {
		return 0, nil
	}
	for row, data := range activeRows {
		copy(idx.mmap[row*rowSize:(row+1)*rowSize], data)
	}
	idx.idList = activeIDs
	idx.generations = activeGenerations
	idx.tombstones = make([]bool, len(activeIDs))
	idx.idMap = make(map[string]int, len(activeIDs))
	for row, id := range activeIDs {
		idx.idMap[id] = row
	}
	idx.count = len(activeIDs)
	idx.syncRowInv()
	if err := idx.mmap.Flush(); err != nil {
		return 0, err
	}
	if err := idx.file.Sync(); err != nil {
		return 0, err
	}
	if err := idx.persistMetadata(); err != nil {
		return 0, err
	}
	idx.rebuildGraph()
	if err := saveHNSWGraphs(idx.file.Name()+".hnsw", idx.graphs); err != nil {
		return 0, err
	}
	return removed, nil
}

// SearchResult holds a matched vector ID and its score.
type SearchResult struct {
	ID    string
	Score float32
}

// VSearch performs ANN cosine KNN search using HNSW over persisted mmap rows.
func (s *VectorStore) VSearch(key string, query []float32, k int, filter func(id string) bool) ([]SearchResult, error) {
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil {
		return nil, err
	}
	if idx == nil || idx.count == 0 {
		return []SearchResult{}, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(query) != idx.dim {
		return nil, fmt.Errorf("dimension mismatch: expected %d, got %d", idx.dim, len(query))
	}

	if k <= 0 {
		return []SearchResult{}, nil
	}

	hnswFilter := func(row int) bool {
		if row < 0 || row >= len(idx.idList) || row >= len(idx.tombstones) || idx.tombstones[row] {
			return false
		}
		return filter == nil || filter(idx.idList[row])
	}

	retrieve := k
	if retrieve < 10 {
		retrieve = 10
	}
	q8 := make([]int8, idx.dim)
	scale, recon := quantizeInto(query, q8)
	qInv := invScale(scale, recon)
	parts := make([][]intNode, len(idx.graphs))
	var wg sync.WaitGroup
	if jobs := idx.searchJobs; jobs != nil {
		for i, graph := range idx.graphs {
			if graph == nil {
				continue
			}
			wg.Add(1)
			jobs <- shardSearchJob{
				graph: graph, q8: q8, qInv: qInv, mmap: idx.mmap, dim: idx.dim,
				k: retrieve, filter: hnswFilter, rowInv: idx.rowInv, out: &parts[i], wg: &wg,
			}
		}
		wg.Wait()
	} else {
		for i, graph := range idx.graphs {
			if graph == nil {
				continue
			}
			parts[i] = graph.Search(q8, qInv, idx.mmap, idx.dim, retrieve, hnswFilter, idx.rowInv)
		}
	}
	hnswResults := make([]intNode, 0, retrieve*len(idx.graphs))
	for _, part := range parts {
		hnswResults = append(hnswResults, part...)
	}
	queryNorm := l2Norm(query)
	for i, res := range hnswResults {
		hnswResults[i].Score = cosineF32Query(query, queryNorm, idx.mmap, idx.dim, res.ID, idx.rowInv)
	}
	sort.Slice(hnswResults, func(i, j int) bool {
		return hnswResults[i].Score > hnswResults[j].Score
	})
	if len(hnswResults) > k {
		hnswResults = hnswResults[:k]
	}

	results := make([]SearchResult, 0, len(hnswResults))
	for _, res := range hnswResults {
		if res.ID >= 0 && res.ID < len(idx.idList) {
			results = append(results, SearchResult{
				ID:    idx.idList[res.ID],
				Score: res.Score,
			})
		}
	}

	return results, nil
}

// SetEfSearch updates the candidate breadth used by VSearch on an index.
func (s *VectorStore) SetEfSearch(key string, ef int) error {
	idx, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || idx == nil {
		return err
	}
	for _, graph := range idx.graphs {
		if graph != nil {
			graph.SetEfSearch(ef)
		}
	}
	return nil
}

// quantizeVector computes SQ8 representations (int8 vector, scale factor, and L2 norm).
func quantizeVector(vec []float32) ([]int8, float32, float32) {
	qVec := make([]int8, len(vec))
	scale, norm := quantizeInto(vec, qVec)
	return qVec, scale, norm
}

func writeQuantized(dst []byte, vec []float32) {
	dim := len(vec)
	var maxAbs float32
	for _, v := range vec {
		absV := v
		if absV < 0 {
			absV = -absV
		}
		if absV > maxAbs {
			maxAbs = absV
		}
	}
	scale := maxAbs / 127.0
	if scale == 0 {
		scale = 1.0
	}
	var reconNorm float32
	for i, v := range vec {
		q := int(math.Round(float64(v / scale)))
		if q > 127 {
			q = 127
		} else if q < -127 {
			q = -127
		}
		dst[i] = byte(int8(q))
		recon := float32(int8(q)) * scale
		reconNorm += recon * recon
	}
	binary.LittleEndian.PutUint32(dst[dim:dim+4], math.Float32bits(scale))
	binary.LittleEndian.PutUint32(dst[dim+4:dim+8], math.Float32bits(reconNorm))
}

func quantizeInto(vec []float32, qVec []int8) (float32, float32) {
	var maxAbs float32
	for _, v := range vec {
		absV := v
		if absV < 0 {
			absV = -absV
		}
		if absV > maxAbs {
			maxAbs = absV
		}
	}

	scale := maxAbs / 127.0
	if scale == 0 {
		scale = 1.0
	}
	var reconNorm float32
	for i, v := range vec {
		q := int(math.Round(float64(v / scale)))
		if q > 127 {
			q = 127
		} else if q < -127 {
			q = -127
		}
		qVec[i] = int8(q)
		recon := float32(q) * scale
		reconNorm += recon * recon
	}
	return scale, reconNorm
}

// writeSQ8 serializes vectors in the DBX SQ8 on-disk format.
// readSQ8 reconstructs an approximate float32 vector from a quantized row. It is
// used to steer graph traversal during a rebuild; ranking distances are always
// recomputed from the mmap, so the dequantization error here does not affect
// search scores.
func readSQ8(src []byte, dim int) []float32 {
	scale := math.Float32frombits(binary.LittleEndian.Uint32(src[dim : dim+4]))
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		vec[i] = float32(int8(src[i])) * scale
	}
	return vec
}

func writeSQ8(dst []byte, qVec []int8, scale float32, norm float32) {
	dim := len(qVec)
	for i, v := range qVec {
		dst[i] = byte(v)
	}
	binary.LittleEndian.PutUint32(dst[dim:dim+4], math.Float32bits(scale))
	binary.LittleEndian.PutUint32(dst[dim+4:dim+8], math.Float32bits(norm))
}
