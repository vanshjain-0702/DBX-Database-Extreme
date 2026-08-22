package persistence

import (
	"fmt"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/protocol"
)

// Recovery handles crash recovery by replaying WAL on top of the latest snapshot.
type Recovery struct {
	wal         *WAL
	snapshotter *Snapshotter
}

// NewRecovery creates a recovery handler.
func NewRecovery(wal *WAL, snapshotter *Snapshotter) *Recovery {
	return &Recovery{wal: wal, snapshotter: snapshotter}
}

// Recover loads the latest snapshot then replays all WAL records.
func (r *Recovery) Recover(kv *engine.KVStore, vecStore *engine.VectorStore) error {
	// Step 1: Load latest snapshot
	if snap := r.snapshotter.Latest(); snap != "" {
		if err := r.snapshotter.Load(kv, snap); err != nil {
			// Do not crash if snapshot is corrupt (e.g. unexpected EOF from power loss).
			// Log it and fall back to full WAL replay.
			fmt.Printf("⚠️  Warning: Failed to load snapshot %s: %v. Falling back to full WAL replay.\n", snap, err)
		}
	}
	// Step 2: Replay WAL
	records, err := r.wal.ReadAll()
	if err != nil {
		return fmt.Errorf("recovery: read wal: %w", err)
	}
	for _, rec := range records {
		switch rec.Type {
		case RecordSet:
			kv.Set(rec.Key, rec.Value, protocol.TypeString, rec.TTLNano)
		case RecordDelete:
			kv.Delete(rec.Key)
		case RecordExpire:
			if rec.TTLNano > 0 {
				kv.Expire(rec.Key, rec.TTLNano/int64(1e9))
			}
		case RecordVAdd:
			if vecStore != nil {
				docID, vector := DecodeVAddPayload(rec.Value)
				if docID == "" || len(vector) == 0 {
					return fmt.Errorf("recovery: invalid vector record sequence %d", rec.Sequence)
				}
				if err := vecStore.VAdd(rec.Key, docID, vector); err != nil {
					return fmt.Errorf("recovery: vector add sequence %d: %w", rec.Sequence, err)
				}
			}
		case RecordVAddBatch:
			if vecStore != nil {
				dim, docIDs, vectors := DecodeVAddBatchPayload(rec.Value)
				if dim <= 0 || len(docIDs) == 0 || len(docIDs) != len(vectors) {
					return fmt.Errorf("recovery: invalid vector batch sequence %d", rec.Sequence)
				}
				if err := vecStore.VAddBatch(rec.Key, dim, docIDs, vectors); err != nil {
					return fmt.Errorf("recovery: vector batch sequence %d: %w", rec.Sequence, err)
				}
			}
		}
	}
	return nil
}
