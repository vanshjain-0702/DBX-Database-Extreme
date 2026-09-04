package persistence

import (
	"os"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/protocol"
)

func TestSnapshotSaveRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kv := engine.New(8)
	kv.Set("k", []byte("v"), protocol.TypeString, 0)
	snap := NewSnapshotter(dir)
	path, err := snap.Save(kv)
	if err != nil {
		t.Fatal(err)
	}
	restored := engine.New(8)
	if err := snap.Load(restored, path); err != nil {
		t.Fatal(err)
	}
	entry := restored.Get("k")
	if entry == nil {
		t.Fatal("missing restored key")
	}
	got, ok := entry.Value.([]byte)
	if !ok || string(got) != "v" {
		t.Fatalf("%#v", entry.Value)
	}
	if snap.Latest() == "" {
		t.Fatal("latest snapshot")
	}
}

func TestSnapshotCurrentPointerSurvivesNewerMtime(t *testing.T) {
	dir := t.TempDir()
	kv := engine.New(8)
	kv.Set("k", []byte("v1"), protocol.TypeString, 0)
	snap := NewSnapshotter(dir)
	first, err := snap.SaveAt(kv, 1)
	if err != nil {
		t.Fatal(err)
	}
	kv.Set("k", []byte("v2"), protocol.TypeString, 0)
	second, err := snap.SaveAt(kv, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Latest(); got != second {
		t.Fatalf("latest = %s, want %s", got, second)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(first, later, later); err != nil {
		t.Fatal(err)
	}
	if got := snap.Latest(); got != second {
		t.Fatalf("mtime stole CURRENT: latest = %s, want %s", got, second)
	}
	hdr, err := snap.LoadWithHeader(engine.New(8), second)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Sequence != 2 {
		t.Fatalf("sequence = %d", hdr.Sequence)
	}
}
