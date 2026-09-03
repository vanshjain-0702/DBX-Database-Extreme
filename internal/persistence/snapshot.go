package persistence

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/security"
)

func init() {
	// gob.Register(engine.VectorIndex{}) removed
}

// SnapshotHeader contains snapshot metadata.
type SnapshotHeader struct {
	Version   int
	Timestamp time.Time
	KeyCount  int
	Checksum  uint32
	Sequence  uint64
}

// Snapshotter creates and restores RDB-style snapshots.
type Snapshotter struct {
	dir string
	enc *security.Encryptor
}

// NewSnapshotter creates a snapshotter writing to dir.
func NewSnapshotter(dir string) *Snapshotter {
	os.MkdirAll(dir, 0755)
	return &Snapshotter{dir: dir}
}

// SetEncryptor encrypts checkpoint files at rest. Search paths are unchanged.
func (s *Snapshotter) SetEncryptor(enc *security.Encryptor) {
	s.enc = enc
}

// Save writes a snapshot of the KV store to disk atomically.
func (s *Snapshotter) Save(kv *engine.KVStore) (string, error) {
	return s.SaveAt(kv, 0)
}

// SaveAt writes a checkpoint covering all WAL transactions through sequence.
func (s *Snapshotter) SaveAt(kv *engine.KVStore, sequence uint64) (string, error) {
	snap := kv.Snapshot()
	filename := fmt.Sprintf("snapshot-%d.rdb", time.Now().UnixNano())
	finalPath := filepath.Join(s.dir, filename)

	wire := make(map[string]*WireEntry, len(snap))
	for k, e := range snap {
		wire[k] = toWireEntry(e)
	}
	hdr := SnapshotHeader{
		Version:   2,
		Timestamp: time.Now(),
		KeyCount:  len(wire),
		Sequence:  sequence,
	}
	var payload bytes.Buffer
	enc := gob.NewEncoder(&payload)
	if err := enc.Encode(hdr); err != nil {
		return "", fmt.Errorf("snapshot header: %w", err)
	}
	if err := enc.Encode(wire); err != nil {
		return "", fmt.Errorf("snapshot data: %w", err)
	}
	if err := isolation.WriteSealedFile(finalPath, payload.Bytes(), s.enc); err != nil {
		return "", fmt.Errorf("snapshot write: %w", err)
	}
	return finalPath, nil
}

// Load restores a snapshot into the KV store.
func (s *Snapshotter) Load(kv *engine.KVStore, path string) error {
	_, err := s.LoadWithHeader(kv, path)
	return err
}

// LoadWithHeader restores a checkpoint and returns its replay boundary.
func (s *Snapshotter) LoadWithHeader(kv *engine.KVStore, path string) (SnapshotHeader, error) {
	data, err := isolation.ReadSealedFile(path, s.enc)
	if err != nil {
		return SnapshotHeader{}, fmt.Errorf("snapshot: open %s: %w", path, err)
	}
	dec := gob.NewDecoder(bytes.NewReader(data))
	var hdr SnapshotHeader
	if err := dec.Decode(&hdr); err != nil {
		return SnapshotHeader{}, fmt.Errorf("snapshot header: %w", err)
	}
	if hdr.Version != 2 {
		return SnapshotHeader{}, fmt.Errorf("snapshot: unsupported version %d; use the offline v1 migration/reset tool", hdr.Version)
	}
	var wire map[string]*WireEntry
	if err := dec.Decode(&wire); err != nil {
		return SnapshotHeader{}, fmt.Errorf("snapshot data: %w", err)
	}
	for k, we := range wire {
		remaining := int64(0)
		if we.ExpiresAt > 0 {
			remaining = we.ExpiresAt - time.Now().UnixNano()
			if remaining <= 0 {
				continue
			}
		}
		kv.Set(k, we.Value, protocol.DataType(we.Type), remaining)
	}
	return hdr, nil
}

// Latest returns the path of the most recent snapshot.
func (s *Snapshotter) Latest() string {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestTime int64
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".rdb" {
			info, _ := e.Info()
			if info.ModTime().UnixNano() > latestTime {
				latestTime = info.ModTime().UnixNano()
				latest = filepath.Join(s.dir, e.Name())
			}
		}
	}
	return latest
}

// WireEntry is a serializable version of engine.Entry.
type WireEntry struct {
	Value     interface{}
	Type      string
	ExpiresAt int64
}

func toWireEntry(e *engine.Entry) *WireEntry {
	we := &WireEntry{
		Type:      string(e.Type),
		ExpiresAt: e.ExpiresAt,
	}
	if e.Type == protocol.TypeVector {
		we.Value = nil // Vector indexes are backed by .vec files, do not serialize
	} else {
		we.Value = e.Value
	}
	return we
}
