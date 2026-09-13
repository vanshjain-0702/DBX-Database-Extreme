package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ── Zero-Heap Shadow Migration ──────────────────────────────────────────

func (idx *MMapVectorIndex) startMigration(dim int, encoding, dataDir, key string) error {
	idx.migrationMu.Lock()
	defer idx.migrationMu.Unlock()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.isMigrating {
		return fmt.Errorf("migration already in progress")
	}
	encoding, err := NormalizeVectorEncoding(encoding)
	if err != nil {
		return err
	}
	shadowPath := filepath.Join(dataDir, key+".shadow")
	for _, suffix := range []string{"", ".meta", ".hnsw"} {
		if err := os.Remove(shadowPath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	shadow, err := newMMapVectorIndex(shadowPath, dim, 1000, idx.atRest, encoding)
	if err != nil {
		return err
	}
	idx.shadowDim = dim
	idx.shadowEncoding = encoding
	idx.shadowIndex = shadow
	idx.isMigrating = true
	return nil
}

func (idx *MMapVectorIndex) insertShadowVector(id string, vec []float32) error {
	idx.migrationMu.RLock()
	defer idx.migrationMu.RUnlock()
	idx.mu.RLock()
	shadow := idx.shadowIndex
	migrating := idx.isMigrating
	idx.mu.RUnlock()
	if !migrating || shadow == nil {
		return fmt.Errorf("no migration in progress")
	}
	if err := validateVector(id, vec); err != nil {
		return err
	}

	shadow.mu.Lock()
	if len(vec) != shadow.dim {
		shadow.mu.Unlock()
		return fmt.Errorf("vector dimension mismatch")
	}
	row, exists := shadow.idMap[id]
	if !exists {
		row = shadow.count
		if err := shadow.ensureCapacity((shadow.count + 1) * shadow.rowBytes()); err != nil {
			shadow.mu.Unlock()
			return err
		}
		shadow.idMap[id] = row
		shadow.idList = append(shadow.idList, id)
		shadow.tombstones = append(shadow.tombstones, false)
		shadow.generations = append(shadow.generations, 1)
		shadow.count++
	}
	shadow.writeRow(row, vec)
	shadow.setRowInv(row)
	shadow.metaDirty++
	if err := shadow.writeMetadata(false); err != nil {
		shadow.mu.Unlock()
		return err
	}
	graph := shadow.graphs[shardIndex(row)]
	mmap := shadow.mmap
	rowInv := shadow.rowInv
	encoding := shadow.encoding
	dim := shadow.dim
	shadow.mu.Unlock()
	shadow.mmapHold.RLock()
	graph.Insert(row, mmap, dim, rowInv, encoding)
	shadow.mmapHold.RUnlock()
	return nil
}

func (idx *MMapVectorIndex) swapMigration() error {
	idx.migrationMu.Lock()
	defer idx.migrationMu.Unlock()
	idx.mu.Lock()
	if !idx.isMigrating || idx.shadowIndex == nil {
		idx.mu.Unlock()
		return fmt.Errorf("no migration in progress")
	}
	shadow := idx.shadowIndex
	shadow.mu.RLock()
	shadowCount := shadow.count
	shadow.mu.RUnlock()
	if shadowCount == 0 {
		idx.mu.Unlock()
		return fmt.Errorf("cannot swap empty migration")
	}
	livePath := idx.path
	shadowPath := shadow.path
	idx.mu.Unlock()

	idx.Close()
	shadow.Close()
	for _, suffix := range []string{"", ".meta", ".hnsw"} {
		if err := os.Remove(livePath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(shadowPath+suffix, livePath+suffix); err != nil {
			return fmt.Errorf("promote shadow index: %w", err)
		}
	}
	replacement, err := newMMapVectorIndex(livePath, shadow.dim, 1000, idx.atRest, shadow.encoding)
	if err != nil {
		return err
	}
	replacement.stopSearchWorkers()
	idx.mu.Lock()
	idx.file = replacement.file
	idx.mmap = replacement.mmap
	idx.dim = replacement.dim
	idx.count = replacement.count
	idx.idMap = replacement.idMap
	idx.idList = replacement.idList
	idx.tombstones = replacement.tombstones
	idx.generations = replacement.generations
	idx.rowInv = replacement.rowInv
	idx.graphs = replacement.graphs
	idx.searchJobs = nil
	idx.searchWG = sync.WaitGroup{}
	idx.metaDirty = replacement.metaDirty
	idx.lastMeta = replacement.lastMeta
	idx.encoding = replacement.encoding
	idx.shadowIndex = nil
	idx.shadowGraphs = nil
	idx.isMigrating = false
	idx.startSearchWorkers()
	idx.mu.Unlock()
	return nil
}

func (idx *MMapVectorIndex) cancelMigration() error {
	idx.migrationMu.Lock()
	defer idx.migrationMu.Unlock()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if !idx.isMigrating {
		return fmt.Errorf("no migration in progress")
	}

	if idx.shadowIndex != nil {
		idx.shadowIndex.Close()
		idx.shadowIndex = nil
	}
	idx.shadowGraphs = nil
	idx.isMigrating = false
	return nil
}
