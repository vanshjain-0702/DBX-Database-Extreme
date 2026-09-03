package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/security"
)

// Sealed large-index path: mmap rows + encrypted metadata/graph must survive
// close/reopen at a size that would have bricked the old heap-backed .vec.
func TestSealedLargeVectorIndexRoundTrip(t *testing.T) {
	count, dim, queries, k := 20000, 64, 16, 10
	if os.Getenv("DBX_LARGE") == "1" {
		count, dim = 100000, 128
	}
	dir := t.TempDir()
	dek := bytes.Repeat([]byte{0x5a}, 32)
	enc, err := security.NewEncryptor(dek)
	if err != nil {
		t.Fatal(err)
	}

	ids := make([]string, count)
	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		ids[i] = "v" + strconv.Itoa(i)
		vectors[i] = deterministicUnitVector(uint64(i+1)*0x9e3779b97f4a7c15, dim)
	}

	first := NewVectorStore(New(64), dir, count)
	first.SetAtRest(enc)
	ingestStart := time.Now()
	const batch = 1000
	for start := 0; start < count; start += batch {
		end := start + batch
		if end > count {
			end = count
		}
		if err := first.VAddBatch("recall", dim, ids[start:end], vectors[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	ingest := time.Since(ingestStart)
	first.CloseAll()

	rowPath := filepath.Join(dir, vectorIndexFilename("recall"))
	info, err := os.Stat(rowPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < int64(count*(dim+8)) {
		t.Fatalf(".vec is %d bytes, want at least %d mmap rows", info.Size(), count*(dim+8))
	}
	metaRaw, err := os.ReadFile(rowPath + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(metaRaw, []byte("DBXENC1\n")) {
		t.Fatal("vector metadata was not sealed")
	}
	if bytes.Contains(metaRaw, []byte(`"ids"`)) || bytes.Contains(metaRaw, []byte("doc1")) {
		t.Fatal("vector metadata stored json in plaintext under the sealed path")
	}

	second := NewVectorStore(New(64), dir, count)
	second.SetAtRest(enc)
	defer second.CloseAll()

	searchStart := time.Now()
	var recallSum float64
	for i := 0; i < queries; i++ {
		row := (i * 7919) % count
		expected := bruteForceTopK(vectors[row], vectors, k)
		results, err := second.VSearch("recall", vectors[row], k, nil)
		if err != nil {
			t.Fatalf("VSearch after reopen: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("sealed large index returned no neighbors after reopen")
		}
		hits := 0
		for _, result := range results {
			n, convErr := strconv.Atoi(result.ID[1:])
			if convErr == nil && expected[n] {
				hits++
			}
		}
		recallSum += float64(hits) / float64(k)
	}
	mean := recallSum / float64(queries)
	t.Logf("sealed large index vectors=%d dim=%d ingest=%s search=%s recall@%d=%.3f file=%dB",
		count, dim, ingest.Round(time.Millisecond), time.Since(searchStart).Round(time.Millisecond), k, mean, info.Size())
	if mean < 0.85 {
		t.Fatalf("mean recall@%d = %.3f, want >= 0.85", k, mean)
	}
}

func TestSealedLargeIndexRefusesWrongKey(t *testing.T) {
	dir := t.TempDir()
	right, err := security.NewEncryptor(bytes.Repeat([]byte{0x11}, 32))
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := security.NewEncryptor(bytes.Repeat([]byte{0x22}, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := NewVectorStore(New(8), dir, 16)
	store.SetAtRest(right)
	if err := store.VAdd("idx", "doc", []float32{1, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	store.CloseAll()

	reopen := NewVectorStore(New(8), dir, 16)
	reopen.SetAtRest(wrong)
	defer reopen.CloseAll()
	_, err = reopen.VSearch("idx", []float32{1, 0, 0, 0}, 1, nil)
	if err == nil {
		t.Fatal("wrong DEK opened sealed vector metadata")
	}
	if got := fmt.Sprint(err); got == "" {
		t.Fatal("expected a decryption error")
	}
}
