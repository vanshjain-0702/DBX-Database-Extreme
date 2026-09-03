package engine

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/dbx/dbx/internal/security"
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

// A crash never gives the index a chance to write its .hnsw file. The rows are
// still in the mmap and the ids are still in the metadata, so the store must
// rebuild the graph on open — otherwise every vector is silently unsearchable
// while VSEARCH keeps returning success.
func TestVectorStoreRebuildsGraphAfterCrash(t *testing.T) {
	dir := t.TempDir()
	key := "tenant/memories"

	first := NewVectorStore(New(16), dir, 0)
	ids := []string{"doc1", "doc2", "doc3"}
	vecs := [][]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for i, id := range ids {
		if err := first.VAdd(key, id, vecs[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Release the mmap handles, then delete the graph file to reproduce what a
	// SIGKILL leaves behind: populated .vec and .meta, no .hnsw.
	first.CloseAll()
	graphPath := filepath.Join(dir, vectorIndexFilename(key)+".hnsw")
	if err := os.Remove(graphPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clearing graph file: %v", err)
	}

	second := NewVectorStore(New(16), dir, 0)
	defer second.CloseAll()
	results, err := second.VSearch(key, []float32{0, 1, 0}, 3, nil)
	if err != nil {
		t.Fatalf("VSearch after crash: %v", err)
	}
	if len(results) != len(ids) {
		t.Fatalf("graph was not rebuilt: got %d results, want %d: %#v", len(results), len(ids), results)
	}
	if results[0].ID != "doc2" {
		t.Fatalf("rebuilt graph ranked wrong vector first: %#v", results)
	}
}

// Encryption must not change the durability contract. Rows stay mmap'd, so a
// SIGKILL leaves populated .vec and .meta with no .hnsw, exactly as it does
// without a key, and the graph rebuilds on open. Buffering rows on the heap and
// writing them only on Close left a zero-length .vec beside metadata that still
// listed every id, which bricked the index on reopen.
func TestEncryptedVectorStoreSurvivesCrash(t *testing.T) {
	dir := t.TempDir()
	key := "tenant/memories"
	dek := make([]byte, 32)
	for i := range dek {
		dek[i] = byte(i + 7)
	}
	enc, err := security.NewEncryptor(dek)
	if err != nil {
		t.Fatal(err)
	}

	first := NewVectorStore(New(16), dir, 0)
	first.SetAtRest(enc)
	ids := []string{"doc1", "doc2", "doc3"}
	vecs := [][]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for i, id := range ids {
		if err := first.VAdd(key, id, vecs[i]); err != nil {
			t.Fatal(err)
		}
	}

	rowPath := filepath.Join(dir, vectorIndexFilename(key))
	info, err := os.Stat(rowPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("vector rows were never written to disk before shutdown")
	}

	first.CloseAll()
	if err := os.Remove(rowPath + ".hnsw"); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clearing graph file: %v", err)
	}

	// Metadata is the encrypted surface and must still open under the same DEK.
	metaRaw, err := os.ReadFile(rowPath + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(metaRaw, []byte("doc2")) {
		t.Fatal("vector metadata stored ids in plaintext")
	}

	second := NewVectorStore(New(16), dir, 0)
	second.SetAtRest(enc)
	defer second.CloseAll()
	results, err := second.VSearch(key, []float32{0, 1, 0}, 3, nil)
	if err != nil {
		t.Fatalf("VSearch after crash: %v", err)
	}
	if len(results) != len(ids) {
		t.Fatalf("encrypted index lost rows: got %d results, want %d: %#v", len(results), len(ids), results)
	}
	if results[0].ID != "doc2" {
		t.Fatalf("rebuilt graph ranked wrong vector first: %#v", results)
	}
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

func TestVectorTombstoneAndCompaction(t *testing.T) {
	store := NewVectorStore(New(8), t.TempDir(), 0)
	defer store.CloseAll()
	if err := store.VAddBatch("idx", 3,
		[]string{"a", "b", "c"},
		[][]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}},
	); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.VDel("idx", "a")
	if err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	results, err := store.VSearch("idx", []float32{1, 0, 0}, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.ID == "a" {
			t.Fatal("tombstoned ID was returned")
		}
	}
	if removed, err := store.VCompact("idx"); err != nil || removed != 1 {
		t.Fatalf("compact = %d, %v", removed, err)
	}
	if err := store.VAdd("idx", "a", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	results, err = store.VSearch("idx", []float32{1, 0, 0}, 1, nil)
	if err != nil || len(results) != 1 || results[0].ID != "a" {
		t.Fatalf("reinserted ID unavailable: %#v, %v", results, err)
	}
}

func TestVectorSearchRecallMatchesBruteForce(t *testing.T) {
	const (
		count   = 800
		dim     = 32
		queries = 12
		k       = 10
	)
	store := NewVectorStore(New(32), t.TempDir(), count)
	defer store.CloseAll()
	vectors := make([][]float32, count)
	ids := make([]string, count)
	src := uint64(42)
	for i := 0; i < count; i++ {
		vectors[i] = deterministicUnitVector(src+uint64(i)*0x9e3779b97f4a7c15, dim)
		ids[i] = "v" + strconv.Itoa(i)
	}
	const batch = 500
	for start := 0; start < count; start += batch {
		end := start + batch
		if end > count {
			end = count
		}
		if err := store.VAddBatch("recall", dim, ids[start:end], vectors[start:end]); err != nil {
			t.Fatal(err)
		}
	}

	var sum float64
	recalls := make([]float64, 0, queries)
	for i := 0; i < queries; i++ {
		queryRow := (i * 7919) % count
		query := vectors[queryRow]
		expected := bruteForceTopK(query, vectors, k)
		results, err := store.VSearch("recall", query, k, nil)
		if err != nil {
			t.Fatal(err)
		}
		hits := 0
		for _, result := range results {
			row, convErr := strconv.Atoi(result.ID[1:])
			if convErr == nil && expected[row] {
				hits++
			}
		}
		recall := float64(hits) / float64(k)
		recalls = append(recalls, recall)
		sum += recall
	}
	sort.Float64s(recalls)
	mean := sum / float64(len(recalls))
	p05 := recalls[int(math.Floor(float64(len(recalls)-1)*0.05))]
	if mean < 0.90 {
		t.Fatalf("mean recall@10 = %.3f, want >= 0.90", mean)
	}
	if p05 < 0.70 {
		t.Fatalf("p05 recall@10 = %.3f, want >= 0.70", p05)
	}
}

func deterministicUnitVector(seed uint64, dim int) []float32 {
	vec := make([]float32, dim)
	var norm float64
	x := seed
	for i := range vec {
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
		vec[i] = float32(int64(x%2001)-1000) / 1000
		norm += float64(vec[i] * vec[i])
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec
}

func bruteForceTopK(query []float32, vectors [][]float32, k int) map[int]bool {
	type scored struct {
		row   int
		score float32
	}
	top := make([]scored, 0, k)
	for row, vector := range vectors {
		var score float32
		for i := range query {
			score += query[i] * vector[i]
		}
		if len(top) < k {
			top = append(top, scored{row: row, score: score})
			sort.Slice(top, func(i, j int) bool { return top[i].score > top[j].score })
		} else if score > top[len(top)-1].score {
			top[len(top)-1] = scored{row: row, score: score}
			sort.Slice(top, func(i, j int) bool { return top[i].score > top[j].score })
		}
	}
	result := make(map[int]bool, len(top))
	for _, item := range top {
		result[item.row] = true
	}
	return result
}

func TestVectorStoreRebuildsCorruptOrMismatchedGraph(t *testing.T) {
	dir := t.TempDir()
	key := "idx"
	first := NewVectorStore(New(8), dir, 0)
	if err := first.VAddBatch(key, 2, []string{"a", "b"}, [][]float32{{1, 0}, {0, 1}}); err != nil {
		t.Fatal(err)
	}
	first.CloseAll()
	graphPath := filepath.Join(dir, vectorIndexFilename(key)+".hnsw")
	if err := os.WriteFile(graphPath, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	second := NewVectorStore(New(8), dir, 0)
	defer second.CloseAll()
	results, err := second.VSearch(key, []float32{1, 0}, 2, nil)
	if err != nil || len(results) != 2 || results[0].ID != "a" {
		t.Fatalf("corrupt graph was not rebuilt: %#v, %v", results, err)
	}
}

func TestReadSQ8RoundTrip(t *testing.T) {
	src := []float32{0.2, -0.5, 0.8}
	qVec, scale, reconNorm := quantizeVector(src)
	if reconNorm <= 0 {
		t.Fatalf("expected reconstructed norm, got %v", reconNorm)
	}
	buf := make([]byte, len(src)+8)
	writeSQ8(buf, qVec, scale, reconNorm)
	got := readSQ8(buf, len(src))
	if len(got) != len(src) {
		t.Fatalf("readSQ8 dim=%d want %d", len(got), len(src))
	}
	for i := range src {
		recon := float32(qVec[i]) * scale
		if math.Abs(float64(got[i]-recon)) > 1e-6 {
			t.Fatalf("axis %d: got %v want %v", i, got[i], recon)
		}
	}
}
