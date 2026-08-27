package persistence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChecksumRoundTrip(t *testing.T) {
	sum := ComputeChecksum([]byte("dbx"))
	if err := VerifyChecksum([]byte("dbx"), sum); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChecksum([]byte("dbx"), sum+1); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestCompactorRemovesCoveredSegments(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wal-10.log"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wal-50.log"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wal.log"), []byte("live"), 0600); err != nil {
		t.Fatal(err)
	}
	removed, err := NewCompactor(dir).CompactThrough(10)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "wal-50.log")); err != nil {
		t.Fatal("newer segment should remain")
	}
	if _, err := NewCompactor(dir).Compact(); err != nil {
		t.Fatal(err)
	}
}

func TestVAddPayloadCodec(t *testing.T) {
	payload := EncodeVAddPayload("id", []float32{1, 0, -1})
	id, vec := DecodeVAddPayload(payload)
	if id != "id" || len(vec) != 3 || vec[0] != 1 {
		t.Fatalf("%s %#v", id, vec)
	}
	batch := EncodeVAddBatchPayload(2, []string{"a", "b"}, [][]float32{{1, 0}, {0, 1}})
	dim, ids, vecs := DecodeVAddBatchPayload(batch)
	if dim != 2 || len(ids) != 2 || len(vecs[1]) != 2 {
		t.Fatalf("%d %#v %#v", dim, ids, vecs)
	}
}
