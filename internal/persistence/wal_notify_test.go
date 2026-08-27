package persistence

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEverysecNotifiesSubscribersWithoutBlockingWrite(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "wal"), "everysec", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()

	got := make(chan *WALRecord, 1)
	wal.Subscribe(func(rec *WALRecord) {
		select {
		case got <- rec:
		default:
		}
	})
	if err := wal.Write(&WALRecord{Type: RecordSet, Key: "live", Value: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	select {
	case rec := <-got:
		if rec.Key != "live" || string(rec.Value) != "1" {
			t.Fatalf("unexpected record %#v", rec)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("everysec write did not notify replica subscriber")
	}
}

func TestReadAllFallsBackWhenDirectoryListingFails(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	if err := wal.Write(&WALRecord{Type: RecordSet, Key: "k", Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatal(err)
	}
	wal.dir = filepath.Join(t.TempDir(), "missing-wal-dir")
	records, err := wal.ReadAll()
	if err != nil {
		t.Fatalf("fallback ReadAll: %v", err)
	}
	if len(records) != 1 || records[0].Key != "k" {
		t.Fatalf("records = %#v", records)
	}
}
