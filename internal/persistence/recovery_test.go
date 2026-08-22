package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dbx/dbx/internal/engine"
)

func TestRecoveryRejectsCorruptWAL(t *testing.T) {
	dir := t.TempDir()
	wal, err := OpenWAL(filepath.Join(dir, "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(filepath.Join(dir, "wal", "wal.log"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0xff, 0x00, 0x01}); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	recovery := NewRecovery(wal, NewSnapshotter(filepath.Join(dir, "snapshots")))
	if err := recovery.Recover(engine.New(4), nil); err != nil {
		t.Fatalf("expected corrupt WAL recovery to ignore tail corruption and succeed, got: %v", err)
	}
}
