package engine

import (
	"math"
	"testing"
)

func TestVectorStore_VAdd_VSearch(t *testing.T) {
	kv := New(16)
	vecStore := NewVectorStore(kv, t.TempDir(), 0)
	key := "test_idx"
	defer func() {
		if idx, _, err := vecStore.getReadOnly(key); err == nil && idx != nil {
			idx.Close()
		}
	}()
	err := vecStore.VAdd(key, "doc1", []float32{1.0, 0.0, 0.0})
	if err != nil {
		t.Fatalf("VAdd failed: %v", err)
	}

	err = vecStore.VAdd(key, "doc2", []float32{0.0, 1.0, 0.0})
	if err != nil {
		t.Fatalf("VAdd failed: %v", err)
	}

	err = vecStore.VAdd(key, "doc3", []float32{0.9, 0.1, 0.0})
	if err != nil {
		t.Fatalf("VAdd failed: %v", err)
	}

	// Search for a vector closest to doc1
	results, err := vecStore.VSearch(key, []float32{1.0, 0.0, 0.0}, 2, nil)
	if err != nil {
		t.Fatalf("VSearch failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// doc1 should be a perfect match (score 1.0)
	if results[0].ID != "doc1" {
		t.Errorf("Expected doc1 as top result, got %s", results[0].ID)
	}
	if math.Abs(float64(results[0].Score-1.0)) > 1e-5 {
		t.Errorf("Expected doc1 score to be ~1.0, got %f", results[0].Score)
	}

	// doc3 should be second best match
	if results[1].ID != "doc3" {
		t.Errorf("Expected doc3 as second result, got %s", results[1].ID)
	}
}

func TestVectorStore_RestoresMetadataAndReplacesID(t *testing.T) {
	dir := t.TempDir()
	key := "tenant/index/with unsafe path"

	first := NewVectorStore(New(16), dir, 0)
	if err := first.VAdd(key, "doc1", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if err := first.VAdd(key, "doc2", []float32{0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	idx, _, err := first.getReadOnly(key)
	if err != nil {
		t.Fatal(err)
	}
	idx.Close()

	second := NewVectorStore(New(16), dir, 0)
	results, err := second.VSearch(key, []float32{0, 1, 0}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ID != "doc2" {
		t.Fatalf("restart recovery returned %#v", results)
	}
	if err := second.VAdd(key, "doc1", []float32{0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	results, err = second.VSearch(key, []float32{0, 1, 0}, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("replacement created duplicate graph node: %#v", results)
	}
	idx, _, _ = second.getReadOnly(key)
	idx.Close()
}

func TestVectorStoreExactSearchRecall(t *testing.T) {
	store := NewVectorStore(New(16), t.TempDir(), 0)
	ids := []string{"a", "b", "c", "d"}
	vecs := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0.99, 0.01, 0, 0},
		{-1, 0, 0, 0},
	}
	if err := store.VAddBatch("exact", 4, ids, vecs); err != nil {
		t.Fatal(err)
	}
	idx, unlock, err := store.getReadOnly("exact")
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	defer unlock()
	results, err := store.VSearch("exact", []float32{1, 0, 0, 0}, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || results[0].ID != "a" || results[1].ID != "c" {
		t.Fatalf("exact search returned %#v", results)
	}
}

func TestCosineSimilarityRaw(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	c := []float32{0, 1, 0}
	d := []float32{-1, 0, 0}

	// Create dummy mmap slice for SQ8 testing
	mmapSlice := make([]byte, (3+8)*4) // 4 vectors max

	// Write vector a
	qVec, scale, norm := quantizeVector(b)
	writeSQ8(mmapSlice[0*11:0*11+11], qVec, scale, norm)

	if cosineSimilarityRaw(a, mmapSlice, 0, 3) < 0.99 {
		t.Errorf("expected ~1.0 for same vectors")
	}

	qVec, scale, norm = quantizeVector(c)
	writeSQ8(mmapSlice[1*11:1*11+11], qVec, scale, norm)

	if math.Abs(float64(cosineSimilarityRaw(a, mmapSlice, 1, 3))) > 0.05 {
		t.Errorf("expected ~0.0 for orthogonal vectors")
	}

	qVec, scale, norm = quantizeVector(d)
	writeSQ8(mmapSlice[2*11:2*11+11], qVec, scale, norm)

	if cosineSimilarityRaw(a, mmapSlice, 2, 3) > -0.99 {
		t.Errorf("expected ~-1.0 for opposite vectors")
	}
}

