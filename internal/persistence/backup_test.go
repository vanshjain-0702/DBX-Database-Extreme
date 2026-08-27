package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/protocol"
)

func TestBackupArchiveRoundTrip(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "tenant")
	snapshotDir := filepath.Join(dataDir, "snapshots")
	kv := engine.New(4)
	kv.Set("key", []byte("value"), protocol.TypeString, 0)
	snapshotter := NewSnapshotter(snapshotDir)
	snapshot, err := snapshotter.SaveAt(kv, 7)
	if err != nil {
		t.Fatal(err)
	}
	vectorPath := filepath.Join(dataDir, "index.vec")
	if err := os.WriteFile(vectorPath, []byte("vector-data"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.zip")
	manifest, err := CreateBackupArchive("tenant-a", dataDir, snapshot, archive, 7)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CheckpointSequence != 7 {
		t.Fatalf("checkpoint = %d", manifest.CheckpointSequence)
	}
	restoreDir := filepath.Join(t.TempDir(), "restored")
	if _, err := ExtractAndValidateBackup(archive, restoreDir, "tenant-a", 1<<20); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(restoreDir, "index.vec"))
	if err != nil || string(got) != "vector-data" {
		t.Fatalf("restored vector = %q, %v", got, err)
	}
}

func TestBackupRejectsWrongTenant(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "tenant")
	snapshotter := NewSnapshotter(filepath.Join(dataDir, "snapshots"))
	snapshot, err := snapshotter.SaveAt(engine.New(1), 0)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if _, err := CreateBackupArchive("tenant-a", dataDir, snapshot, archive, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractAndValidateBackup(archive, t.TempDir(), "tenant-b", 1<<20); err == nil {
		t.Fatal("expected tenant mismatch to fail")
	}
}
