package persistence

import (
	"testing"

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
