package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

func TestReadKeyHistoryKeepsOnlyThatKeyUpToTheCutoff(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	write := func(effects ...WALEffect) {
		t.Helper()
		if _, err := wal.WriteTransaction(effects); err != nil {
			t.Fatal(err)
		}
	}
	write(WALEffect{Type: RecordSet, Key: "mem", Value: []byte("1")})
	write(WALEffect{Type: RecordSet, Key: "other", Value: []byte("big")})
	write(WALEffect{Type: RecordSet, Key: "other", Value: []byte("x")}, WALEffect{Type: RecordSet, Key: "mem", Value: []byte("2")})
	time.Sleep(2 * time.Millisecond)
	cutoff := time.Now().UnixNano()
	time.Sleep(2 * time.Millisecond)
	write(WALEffect{Type: RecordSet, Key: "mem", Value: []byte("after")})

	history, err := wal.ReadKeyHistory("mem", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history has %d records, want 2: %#v", len(history), history)
	}
	for i, want := range []string{"1", "2"} {
		rec := history[i]
		if len(rec.Effects) != 1 || rec.Effects[0].Key != "mem" || string(rec.Effects[0].Value) != want {
			t.Fatalf("record %d = %#v, want only mem=%s", i, rec.Effects, want)
		}
	}
	if history[0].Sequence >= history[1].Sequence {
		t.Fatal("history is not in sequence order")
	}
}

// ReadKeyHistory stops at the first record past AS_OF, which is only exact if
// concurrent writers can never land in the log out of sequence order.
func TestConcurrentWritesLandInSequenceOrder(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "wal"), "everysec", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if err := wal.Write(&WALRecord{Type: RecordSet, Key: "k", Value: []byte("v")}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err := wal.Sync(); err != nil {
		t.Fatal(err)
	}
	var last uint64
	var lastTS int64
	count := 0
	if err := wal.scan(func(rec *WALRecord) error {
		if rec.Sequence <= last || rec.Timestamp < lastTS {
			return fmt.Errorf("record seq=%d ts=%d follows seq=%d ts=%d", rec.Sequence, rec.Timestamp, last, lastTS)
		}
		last, lastTS = rec.Sequence, rec.Timestamp
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 16*200 {
		t.Fatalf("scanned %d records, want %d", count, 16*200)
	}
}

// Time travel must not read the log past its point in time: a query about the
// past stays cheap however much has been written since.
func TestReadKeyHistoryStopsAtCutoff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wal")
	wal, err := OpenWAL(dir, "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	if err := wal.Write(&WALRecord{Type: RecordSet, Key: "mem", Value: []byte("old")}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	cutoff := time.Now().UnixNano()
	time.Sleep(2 * time.Millisecond)
	if err := wal.Write(&WALRecord{Type: RecordSet, Key: "mem", Value: []byte("new")}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "wal.log"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0xff, 0xff, 0xff, 0xff, 0xde, 0xad}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := wal.ReadAll(); err == nil {
		t.Fatal("full scan should reach the unreadable bytes past the cutoff")
	}

	history, err := wal.ReadKeyHistory("mem", cutoff)
	if err != nil {
		t.Fatalf("ReadKeyHistory read past its cutoff: %v", err)
	}
	if len(history) != 1 || string(history[0].Value) != "old" {
		t.Fatalf("history = %#v, want only mem=old", history)
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
