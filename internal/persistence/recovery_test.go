package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/protocol"
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

	wal, err = OpenWAL(filepath.Join(dir, "wal"), "always", 64)
	if err != nil {
		t.Fatalf("expected final partial frame to be repaired, got: %v", err)
	}
	defer wal.Close()
	if err := wal.Write(&WALRecord{Type: RecordSet, Key: "after-tail", Value: []byte("ok")}); err != nil {
		t.Fatal(err)
	}
	recovery := NewRecovery(wal, NewSnapshotter(filepath.Join(dir, "snapshots")))
	if err := recovery.Recover(engine.New(4), nil); err != nil {
		t.Fatalf("expected repaired WAL recovery to succeed, got: %v", err)
	}
}

func TestWALV2TransactionAndCheckpointRecovery(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	snapshotDir := filepath.Join(dir, "snapshots")
	wal, err := OpenWAL(walDir, "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Hour).UnixNano()
	sequence, err := wal.WriteTransaction([]WALEffect{
		{Type: RecordSet, Key: "source", Value: []byte("one")},
		{Type: RecordSet, Key: "expiring", Value: []byte("two"), ExpiresAt: expiresAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sequence != 1 {
		t.Fatalf("sequence = %d, want 1", sequence)
	}

	kv := engine.New(4)
	if err := NewRecovery(wal, NewSnapshotter(snapshotDir)).Recover(kv, nil); err != nil {
		t.Fatal(err)
	}
	if got := string(kv.Get("source").Value.([]byte)); got != "one" {
		t.Fatalf("source = %q", got)
	}
	if _, err := NewSnapshotter(snapshotDir).SaveAt(kv, sequence); err != nil {
		t.Fatal(err)
	}
	if err := wal.Rotate(); err != nil {
		t.Fatal(err)
	}
	if _, err := wal.WriteTransaction([]WALEffect{
		{Type: RecordSet, Key: "source", Value: []byte("new")},
		{Type: RecordDelete, Key: "expiring"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWAL(walDir, "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered := engine.New(4)
	if err := NewRecovery(reopened, NewSnapshotter(snapshotDir)).Recover(recovered, nil); err != nil {
		t.Fatal(err)
	}
	entry := recovered.Get("source")
	if entry == nil || entry.Type != protocol.TypeString || string(entry.Value.([]byte)) != "new" {
		t.Fatalf("unexpected recovered source: %#v", entry)
	}
	if recovered.Exists("expiring") {
		t.Fatal("expiring key should have been deleted by post-checkpoint transaction")
	}
}

func TestWALV2RejectsCRCFailure(t *testing.T) {
	dir := t.TempDir()
	wal, err := OpenWAL(dir, "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := wal.Write(&WALRecord{Type: RecordSet, Key: "key", Value: []byte("value")}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wal.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWAL(dir, "always", 64); err == nil {
		t.Fatal("expected CRC failure to reject WAL")
	}
}
