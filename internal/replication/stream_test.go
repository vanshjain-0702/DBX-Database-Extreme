package replication

import (
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/persistence"
)

type testReplicaEngine struct{}

func (testReplicaEngine) ApplyWALRecord(*persistence.WALRecord) error { return nil }

type collectingReplicaEngine struct {
	records chan *persistence.WALRecord
}

func (e *collectingReplicaEngine) ApplyWALRecord(record *persistence.WALRecord) error {
	e.records <- record
	return nil
}

func TestPrimaryAndReplicaStreamBootstrapWAL(t *testing.T) {
	wal, err := persistence.OpenWAL(filepath.Join(t.TempDir(), "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	if err := wal.Write(&persistence.WALRecord{Type: persistence.RecordSet, Key: "quantrag:key", Value: []byte("value")}); err != nil {
		t.Fatal(err)
	}

	primary := NewPrimaryStream()
	if err := primary.Start("127.0.0.1:0", wal); err != nil {
		t.Fatal(err)
	}
	defer primary.Stop()

	engine := &collectingReplicaEngine{records: make(chan *persistence.WALRecord, 1)}
	replica := NewReplicaStream(primary.Addr(), engine)
	replica.Start()
	defer replica.Stop()

	select {
	case record := <-engine.records:
		if record.Key != "quantrag:key" || string(record.Value) != "value" {
			t.Fatalf("unexpected replicated record: %#v", record)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WAL bootstrap")
	}

	if primary.Addr() == "" {
		t.Fatal(fmt.Errorf("primary address was not exposed"))
	}
}

func TestReplicaStreamRejectsOversizedFrame(t *testing.T) {
	server, client := net.Pipe()
	rs := NewReplicaStream("", testReplicaEngine{})
	done := make(chan struct{})
	go func() {
		rs.consumeStream(server)
		close(done)
	}()

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], maxReplicationFrameSize+1)
	if _, err := client.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("replica did not reject oversized frame")
	}
	client.Close()
	server.Close()
}

func TestReplicaStreamStopIsIdempotent(t *testing.T) {
	rs := NewReplicaStream("", testReplicaEngine{})
	rs.Stop()
	rs.Stop()
}

func TestPrimaryStreamsLiveEverysecWrites(t *testing.T) {
	wal, err := persistence.OpenWAL(filepath.Join(t.TempDir(), "wal"), "everysec", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()

	primary := NewPrimaryStream()
	if err := primary.Start("127.0.0.1:0", wal); err != nil {
		t.Fatal(err)
	}
	defer primary.Stop()
	wal.Subscribe(primary.BroadcastRecord)

	engine := &collectingReplicaEngine{records: make(chan *persistence.WALRecord, 4)}
	replica := NewReplicaStream(primary.Addr(), engine)
	replica.Start()
	defer replica.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := wal.Write(&persistence.WALRecord{Type: persistence.RecordSet, Key: "live", Value: []byte("ok")}); err != nil {
			t.Fatal(err)
		}
		select {
		case record := <-engine.records:
			if record.Key == "live" && string(record.Value) == "ok" {
				return
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("timed out waiting for live WAL stream")
}
