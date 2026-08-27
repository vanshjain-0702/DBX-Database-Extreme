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
