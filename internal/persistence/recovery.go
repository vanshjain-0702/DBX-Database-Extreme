package persistence

import (
	"fmt"
	"time"

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
	var checkpointSequence uint64
	if snap := r.snapshotter.Latest(); snap != "" {
		hdr, err := r.snapshotter.LoadWithHeader(kv, snap)
		if err != nil {
			return fmt.Errorf("recovery: load checkpoint %s: %w", snap, err)
		}
		checkpointSequence = hdr.Sequence
	}
	// Step 2: Replay WAL
	records, err := r.wal.ReadAll()
	if err != nil {
		return fmt.Errorf("recovery: read wal: %w", err)
	}
	for _, rec := range records {
		if rec.Sequence <= checkpointSequence {
			continue
		}
		effects := rec.Effects
		if len(effects) == 0 {
			effects = []WALEffect{{Type: rec.Type, Key: rec.Key, Value: rec.Value, ExpiresAt: rec.TTLNano}}
		}
		for _, effect := range effects {
			if err := applyRecoveredEffect(kv, vecStore, rec.Sequence, effect); err != nil {
				return err
			}
		}
	}
	if vecStore != nil {
		if err := vecStore.ReopenPersisted(); err != nil {
			return fmt.Errorf("recovery: reopen vector indexes: %w", err)
		}
	}
	return nil
}

func applyRecoveredEffect(kv *engine.KVStore, vecStore *engine.VectorStore, sequence uint64, effect WALEffect) error {
	switch effect.Type {
	case RecordSet:
		remaining := int64(0)
		if effect.ExpiresAt > 0 {
			remaining = effect.ExpiresAt - time.Now().UnixNano()
			if remaining <= 0 {
				kv.Delete(effect.Key)
				return nil
			}
		}
		kv.Set(effect.Key, effect.Value, protocol.TypeString, remaining)
	case RecordDelete:
		kv.Delete(effect.Key)
	case RecordExpire:
		if effect.ExpiresAt <= 0 {
			kv.Persist(effect.Key)
		} else {
			remaining := effect.ExpiresAt - time.Now().UnixNano()
			if remaining <= 0 {
				kv.Delete(effect.Key)
			} else {
				kv.Expire(effect.Key, max(1, remaining/int64(time.Second)))
			}
		}
	case RecordVAdd:
		if vecStore != nil {
			docID, vector := DecodeVAddPayload(effect.Value)
			if docID == "" || len(vector) == 0 {
				return fmt.Errorf("recovery: invalid vector record sequence %d", sequence)
			}
			if err := vecStore.VAdd(effect.Key, docID, vector); err != nil {
				return fmt.Errorf("recovery: vector add sequence %d: %w", sequence, err)
			}
		}
	case RecordVAddBatch:
		if vecStore != nil {
			dim, docIDs, vectors := DecodeVAddBatchPayload(effect.Value)
			if dim <= 0 || len(docIDs) == 0 || len(docIDs) != len(vectors) {
				return fmt.Errorf("recovery: invalid vector batch sequence %d", sequence)
			}
			if err := vecStore.VAddBatch(effect.Key, dim, docIDs, vectors); err != nil {
				return fmt.Errorf("recovery: vector batch sequence %d: %w", sequence, err)
			}
		}
	case RecordVTombstone:
		if vecStore != nil {
			if _, err := vecStore.VDel(effect.Key, string(effect.Value)); err != nil {
				return fmt.Errorf("recovery: vector tombstone sequence %d: %w", sequence, err)
			}
		}
	case RecordDeleteIndex:
		kv.Delete(effect.Key)
	default:
		return fmt.Errorf("recovery: unsupported effect type %d at sequence %d", effect.Type, sequence)
	}
	return nil
}
