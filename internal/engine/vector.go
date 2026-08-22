package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
	"github.com/edsrzf/mmap-go"
)

// MMapVectorIndex uses a memory-mapped file for zero-GC, massive scale vector storage.
type MMapVectorIndex struct {
	file   *os.File
	mmap   mmap.MMap
	dim    int
	count  int
	idMap  map[string]int // maps vector ID to row index
	idList []string       // maps row index to vector ID
	hnsw   *HNSWGraph     // HNSW index
	mu     sync.RWMutex
}

type vectorMetadata struct {
	Dim int      `json:"dim"`
	IDs []string `json:"ids"`
}

func NewMMapVectorIndex(path string, dim int) (*MMapVectorIndex, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	// Pre-allocate 1000 vectors worth of space if empty
	info, _ := f.Stat()
	if info.Size() == 0 {
		initialSize := int64(1000 * (dim + 8))
		f.Truncate(initialSize)
	}

	m, err := mmap.Map(f, mmap.RDWR, 0)
	if err != nil {
		f.Close()
		return nil, err
	}

	idx := &MMapVectorIndex{
		file:   f,
		mmap:   m,
		dim:    dim,
		idMap:  make(map[string]int),
		idList: make([]string, 0),
	}
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
		for row, id := range idx.idList {
			idx.idMap[id] = row
		}
	}
	hnswPath := path + ".hnsw"
	if h, err := LoadHNSWGraph(hnswPath); err == nil {
		idx.hnsw = h
	} else {
		idx.hnsw = NewHNSWGraph()
	}
	return idx, nil
}

func (idx *MMapVectorIndex) persistMetadata() error {
	data, err := json.Marshal(vectorMetadata{Dim: idx.dim, IDs: idx.idList})
	if err != nil {
		return err
	}
	path := idx.file.Name() + ".meta"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0600)
	if err == nil {
		err = f.Sync()
		f.Close()
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(path) // Windows fix for Access Denied on Rename
	return os.Rename(tmp, path)
}

func (idx *MMapVectorIndex) Close() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.hnsw.Save(idx.file.Name() + ".hnsw")
	idx.mmap.Unmap()
	idx.file.Close()
}

// VectorStore provides vector operations.
type VectorStore struct {
	kv         *KVStore
	dataDir    string
	maxVectors int
}

func NewVectorStore(kv *KVStore, dataDir string, maxVectors int) *VectorStore {
	os.MkdirAll(dataDir, 0755)
	return &VectorStore{kv: kv, dataDir: dataDir, maxVectors: maxVectors}
}

func (s *VectorStore) getOrCreate(key string, dim int) (*MMapVectorIndex, func(), error) {
	e, unlock := s.kv.GetForWrite(key)
	if e == nil {
		path := filepath.Join(s.dataDir, vectorIndexFilename(key))
		idx, err := NewMMapVectorIndex(path, dim)
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
		idx, err := NewMMapVectorIndex(path, dim)
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
	if dim == 0 {
		return fmt.Errorf("empty vector")
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

	// Calculate needed capacity
	neededBytes := (idx.count + 1) * (dim + 8)
	if neededBytes > len(idx.mmap) {
		// Remap with double capacity
		idx.mmap.Unmap()
		newSize := int64(len(idx.mmap) * 2)
		if newSize < int64(neededBytes) {
			newSize = int64(neededBytes)
		}
		idx.file.Truncate(newSize)
		idx.mmap, _ = mmap.Map(idx.file, mmap.RDWR, 0)
	}

	isNew := false
	row := idx.count
	if existingRow, exists := idx.idMap[id]; exists {
		row = existingRow
	} else {
		idx.idMap[id] = row
		idx.idList = append(idx.idList, id)
		idx.count++
		isNew = true
	}

	// The on-disk vector format is SQ8 (Scalar Quantization 8-bit).
	// Format: dim bytes (int8), 4 bytes float32 (scale), 4 bytes float32 (norm).
	off := row * (dim + 8)
	qVec, scale, norm := quantizeVector(vec)
	writeSQ8(idx.mmap[off:off+dim+8], qVec, scale, norm)

	if err := idx.mmap.Flush(); err != nil {
		return err
	}
	if err := idx.file.Sync(); err != nil {
		return err
	}
	if err := idx.persistMetadata(); err != nil {
		return err
	}

	if isNew {
		idx.hnsw.Insert(row, vec, idx.mmap, dim)
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
	for _, vec := range vecs {
		if len(vec) != dim {
			return fmt.Errorf("dimension mismatch in batch")
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

	newAdditions := 0
	for _, id := range ids {
		if _, exists := idx.idMap[id]; !exists {
			newAdditions++
		}
	}

	if s.maxVectors > 0 && idx.count+newAdditions > s.maxVectors {
		return fmt.Errorf("tenant vector quota exceeded: maximum of %d vectors allowed", s.maxVectors)
	}

	neededBytes := (idx.count + len(ids)) * (dim + 8)
	if neededBytes > len(idx.mmap) {
		idx.mmap.Unmap()
		newSize := int64(len(idx.mmap) * 2)
		if newSize < int64(neededBytes) {
			newSize = int64(neededBytes)
		}
		idx.file.Truncate(newSize)
		idx.mmap, _ = mmap.Map(idx.file, mmap.RDWR, 0)
	}

	newRows := make(map[int][]float32)
	for i, id := range ids {
		vec := vecs[i]
		row := idx.count
		if existingRow, exists := idx.idMap[id]; exists {
			row = existingRow
		} else {
			idx.idMap[id] = row
			idx.idList = append(idx.idList, id)
			idx.count++
			newRows[row] = vec
		}

		// Persist using DBX's canonical SQ8 vector encoding.
		off := row * (dim + 8)
		qVec, scale, norm := quantizeVector(vec)
		writeSQ8(idx.mmap[off:off+dim+8], qVec, scale, norm)
	}
	if err := idx.mmap.Flush(); err != nil {
		return err
	}
	if err := idx.file.Sync(); err != nil {
		return err
	}
	if err := idx.persistMetadata(); err != nil {
		return err
	}

	for row, vec := range newRows {
		idx.hnsw.Insert(row, vec, idx.mmap, dim)
	}

	return nil
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
	
	var hnswFilter func(id int) bool
	if filter != nil {
		hnswFilter = func(row int) bool {
			if row < 0 || row >= len(idx.idList) {
				return false
			}
			return filter(idx.idList[row])
		}
	}
	
	hnswResults := idx.hnsw.Search(query, idx.mmap, idx.dim, k, hnswFilter)
	
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

// quantizeVector computes SQ8 representations (int8 vector, scale factor, and L2 norm).
func quantizeVector(vec []float32) ([]int8, float32, float32) {
	var maxAbs float32
	var norm float32
	for _, v := range vec {
		absV := v
		if absV < 0 {
			absV = -absV
		}
		if absV > maxAbs {
			maxAbs = absV
		}
		norm += v * v
	}

	scale := maxAbs / 127.0
	if scale == 0 {
		scale = 1.0
	}

	qVec := make([]int8, len(vec))
	for i, v := range vec {
		qVec[i] = int8(math.Round(float64(v / scale)))
	}

	return qVec, scale, norm
}

// writeSQ8 serializes vectors in the DBX SQ8 on-disk format.
func writeSQ8(dst []byte, qVec []int8, scale float32, norm float32) {
	dim := len(qVec)
	for i, v := range qVec {
		dst[i] = byte(v)
	}
	binary.LittleEndian.PutUint32(dst[dim:dim+4], math.Float32bits(scale))
	binary.LittleEndian.PutUint32(dst[dim+4:dim+8], math.Float32bits(norm))
}
