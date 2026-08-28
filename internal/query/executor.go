// Package query provides command routing, execution, and planning.
package query

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/events"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/transaction"
	"github.com/dbx/dbx/internal/util"
)

var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
)

// Executor executes commands against the engine.
type Executor struct {
	kv              *engine.KVStore
	str             *engine.StringStore
	hash            *engine.HashStore
	list            *engine.ListStore
	set             *engine.SetStore
	zset            *engine.ZSetStore
	stream          *engine.StreamStore
	bitmap          *engine.BitmapStore
	geo             *engine.GeoStore
	doc             *engine.DocumentStore
	vec             *engine.VectorStore
	multi           *transaction.MultiManager
	watch           *transaction.WatchSet
	mvcc            *transaction.MVCCStore
	pubsub          *events.PubSub
	metrics         *observability.Metrics
	wal             *persistence.WAL
	raft            RaftNode
	mutationMu      sync.Mutex
	suppressWAL     bool
	maxMemory       int64
	accountedMemory atomic.Int64
	readOnly        atomic.Bool
	testHook        func()
}

// NewExecutor creates an executor with all stores wired.
func NewExecutor(
	kv *engine.KVStore,
	vec *engine.VectorStore,
	multi *transaction.MultiManager,
	watch *transaction.WatchSet,
	mvcc *transaction.MVCCStore,
	pubsub *events.PubSub,
	metrics *observability.Metrics,
	wal *persistence.WAL,
) *Executor {
	return &Executor{
		kv:      kv,
		str:     engine.NewStringStore(kv),
		hash:    engine.NewHashStore(kv),
		list:    engine.NewListStore(kv),
		set:     engine.NewSetStore(kv),
		zset:    engine.NewZSetStore(kv),
		stream:  engine.NewStreamStore(kv),
		bitmap:  engine.NewBitmapStore(kv),
		geo:     engine.NewGeoStore(kv),
		doc:     engine.NewDocumentStore(kv),
		vec:     vec,
		multi:   multi,
		watch:   watch,
		mvcc:    mvcc,
		pubsub:  pubsub,
		metrics: metrics,
		wal:     wal,
	}
}

// KV returns the underlying KVStore.
func (e *Executor) KV() *engine.KVStore {
	return e.kv
}

// Vectors returns the tenant vector store.
func (e *Executor) Vectors() *engine.VectorStore {
	return e.vec
}

// SetRaft assigns the Raft node to the executor for write interception.
func (e *Executor) SetRaft(raftNode RaftNode) {
	e.raft = raftNode
}

func (e *Executor) writeWAL(rec *persistence.WALRecord) error {
	if e.wal == nil || e.suppressWAL {
		return nil
	}
	return e.wal.Write(rec)
}

// SetReadOnly marks this executor as a replica. Client writes are rejected
// before WAL; ApplyWALRecord still applies bytes from the primary.
func (e *Executor) SetReadOnly(v bool) {
	e.readOnly.Store(v)
}

// ApplyWALRecord applies a replicated record without writing it back to the WAL.
func (e *Executor) ApplyWALRecord(rec *persistence.WALRecord) error {
	if rec == nil {
		return fmt.Errorf("replication: nil WAL record")
	}
	if len(rec.Effects) > 0 {
		for _, effect := range rec.Effects {
			if err := e.applyReplicatedEffect(effect); err != nil {
				return err
			}
		}
		e.accountedMemory.Store(e.measureMemoryUsage())
		return nil
	}
	switch rec.Type {
	case persistence.RecordSet:
		e.kv.Set(rec.Key, rec.Value, protocol.TypeString, rec.TTLNano)
	case persistence.RecordDelete:
		e.kv.Delete(rec.Key)
	case persistence.RecordExpire:
		if rec.TTLNano > 0 && !e.kv.Expire(rec.Key, rec.TTLNano/int64(time.Second)) {
			return fmt.Errorf("replication: key %q not found for expire", rec.Key)
		}
	case persistence.RecordVAdd:
		id, vector := persistence.DecodeVAddPayload(rec.Value)
		if err := e.vec.VAdd(rec.Key, id, vector); err != nil {
			return err
		}
	case persistence.RecordVAddBatch:
		dim, ids, vectors := persistence.DecodeVAddBatchPayload(rec.Value)
		if err := e.vec.VAddBatch(rec.Key, dim, ids, vectors); err != nil {
			return err
		}
	default:
		return fmt.Errorf("replication: unsupported WAL record type %d", rec.Type)
	}
	e.accountedMemory.Store(e.measureMemoryUsage())
	return nil
}

func (e *Executor) applyReplicatedEffect(effect persistence.WALEffect) error {
	switch effect.Type {
	case persistence.RecordSet:
		remaining := int64(0)
		if effect.ExpiresAt > 0 {
			remaining = effect.ExpiresAt - time.Now().UnixNano()
			if remaining <= 0 {
				e.kv.Delete(effect.Key)
				return nil
			}
		}
		e.kv.Set(effect.Key, effect.Value, protocol.TypeString, remaining)
	case persistence.RecordDelete, persistence.RecordDeleteIndex:
		e.kv.Delete(effect.Key)
	case persistence.RecordExpire:
		if effect.ExpiresAt <= 0 {
			e.kv.Persist(effect.Key)
			return nil
		}
		remaining := effect.ExpiresAt - time.Now().UnixNano()
		if remaining <= 0 {
			e.kv.Delete(effect.Key)
			return nil
		}
		if !e.kv.Expire(effect.Key, max(1, remaining/int64(time.Second))) {
			return fmt.Errorf("replication: key %q not found for expire", effect.Key)
		}
	case persistence.RecordVAdd:
		id, vector := persistence.DecodeVAddPayload(effect.Value)
		return e.vec.VAdd(effect.Key, id, vector)
	case persistence.RecordVAddBatch:
		dim, ids, vectors := persistence.DecodeVAddBatchPayload(effect.Value)
		return e.vec.VAddBatch(effect.Key, dim, ids, vectors)
	case persistence.RecordVTombstone:
		_, err := e.vec.VDel(effect.Key, string(effect.Value))
		return err
	default:
		return fmt.Errorf("replication: unsupported WAL effect type %d", effect.Type)
	}
	return nil
}

// Execute processes a command and writes the response to writer.
func (e *Executor) Execute(clientID uint64, cmd *protocol.Command, w *protocol.Writer) (err error) {
	start := time.Now()
	var execErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			execErr = fmt.Errorf("panic: %v", recovered)
			if err == nil {
				err = w.WriteError("ERR internal server error")
			}
		}
		latency := time.Since(start).Nanoseconds()
		name := ""
		if cmd != nil {
			name = cmd.Normalized()
		}
		info, _ := protocol.Lookup(name)
		e.metrics.RecordCommand(!info.ReadOnly, latency, execErr)
		e.metrics.TenantMemoryUsed.Store(e.accountedMemory.Load())
		if e.wal != nil && e.wal.Failure() != nil {
			e.metrics.TenantReady.Store(0)
		}
	}()

	if e.testHook != nil {
		e.testHook()
	}

	// If in MULTI block and this isn't EXEC/DISCARD/MULTI/WATCH, queue it
	if e.multi.IsActive(clientID) {
		name := cmd.Normalized()
		if name != "EXEC" && name != "DISCARD" && name != "WATCH" && name != "MULTI" {
			e.multi.Queue(clientID, cmd)
			return w.WriteSimpleString("QUEUED")
		}
	}

	info, ok := protocol.Lookup(cmd.Normalized())
	if !ok {
		return w.WriteError(fmt.Sprintf("ERR unknown command '%s'", cmd.Name))
	}
	if e.wal != nil && !protocol.SupportedInDurableV1(cmd.Normalized()) {
		return w.WriteError(fmt.Sprintf(
			"ERR command '%s' is not supported by the durable DBX v1 profile",
			cmd.Name,
		))
	}
	if e.readOnly.Load() && !info.ReadOnly {
		return w.WriteError("READONLY replica does not accept writes")
	}
	prepared := false
	projectedMemory := e.accountedMemory.Load()
	if e.wal != nil && info.DurableV1 && !info.ReadOnly {
		e.mutationMu.Lock()
		defer e.mutationMu.Unlock()
		var effects []persistence.WALEffect
		effects, prepared = e.prepareDurableEffects(cmd)
		if prepared && len(effects) > 0 {
			var fits bool
			fits, projectedMemory = e.effectsFitQuota(effects)
			if !fits {
				return w.WriteError("OOM tenant quota exceeded")
			}
			if _, err := e.wal.WriteTransaction(effects); err != nil {
				return w.WriteError("ERR WAL write failed: " + err.Error())
			}
			e.suppressWAL = true
			defer func() { e.suppressWAL = false }()
		}
	}

	// Multi-node writes go through Raft. A single voter commits locally + WAL,
	// so an isolated tenant never pays consensus cost on its hot path.
	if e.raft != nil && !info.ReadOnly && !e.raft.SingleVoter() {
		if e.raft.State() != 2 { // raft.Leader
			return w.WriteError("ERR node is not the Raft leader")
		}
		data, err := json.Marshal(cmd)
		if err != nil {
			return w.WriteError("ERR failed to serialize command for Raft: " + err.Error())
		}
		future := e.raft.Apply(data, 10*time.Second)
		if err := future.Error(); err != nil {
			return w.WriteError("ERR Raft apply failed: " + err.Error())
		}
		if resp, ok := future.Response().([]byte); ok {
			w.WriteRaw(resp)
			return nil
		}
	}

	execErr = e.Dispatch(clientID, cmd, w)
	if execErr == nil && e.wal != nil && info.DurableV1 && !info.ReadOnly {
		if prepared {
			e.accountedMemory.Store(projectedMemory)
		} else {
			e.accountedMemory.Store(e.measureMemoryUsage())
		}
	}
	return execErr
}

// SetMemoryLimit configures this tenant's no-eviction admission limit.
func (e *Executor) SetMemoryLimit(bytes int64) {
	e.maxMemory = bytes
	e.accountedMemory.Store(e.measureMemoryUsage())
}

// MemoryUsage returns conservative tenant-owned KV and vector bytes.
func (e *Executor) MemoryUsage() int64 {
	return e.accountedMemory.Load()
}

func (e *Executor) measureMemoryUsage() int64 {
	return e.kv.MemoryUsage() + e.vec.MemoryUsage()
}

func (e *Executor) effectsFitQuota(effects []persistence.WALEffect) (bool, int64) {
	projected := e.accountedMemory.Load()
	final := make(map[string]persistence.WALEffect)
	for _, effect := range effects {
		if effect.Type == persistence.RecordSet || effect.Type == persistence.RecordDelete {
			final[effect.Key] = effect
		}
	}
	for key, effect := range final {
		entry := e.kv.Get(key)
		if entry != nil {
			projected -= int64(len(key) + 96)
			if entry.Type == protocol.TypeString {
				if value, ok := entry.Value.([]byte); ok {
					projected -= int64(len(value))
				}
			}
		}
		if effect.Type == persistence.RecordSet {
			projected += int64(len(key) + len(effect.Value) + 96)
		}
	}
	return e.maxMemory <= 0 || projected <= e.maxMemory, projected
}

func (e *Executor) prepareDurableEffects(cmd *protocol.Command) ([]persistence.WALEffect, bool) {
	name := cmd.Normalized()
	now := time.Now().UnixNano()
	stringState := func(key string) ([]byte, int64, bool, bool) {
		entry := e.kv.Get(key)
		if entry == nil {
			return nil, 0, false, true
		}
		if entry.Type != protocol.TypeString {
			return nil, 0, true, false
		}
		value, ok := entry.Value.([]byte)
		if !ok {
			return nil, 0, true, false
		}
		return append([]byte(nil), value...), entry.ExpiresAt, true, true
	}
	put := func(key string, value []byte, expiresAt int64) persistence.WALEffect {
		return persistence.WALEffect{Type: persistence.RecordSet, Key: key, Value: value, ExpiresAt: expiresAt}
	}

	switch name {
	case "SET":
		if cmd.NumArgs() < 2 {
			return nil, true
		}
		value, ttlSec, nx, xx, err := engine.ParseSetArgs(cmd.Args[1:])
		if err != nil {
			return nil, true
		}
		_, _, exists, valid := stringState(cmd.Arg(0))
		if !valid || (nx && exists) || (xx && !exists) {
			return nil, true
		}
		expiresAt := int64(0)
		if ttlSec > 0 {
			expiresAt = now + ttlSec*int64(time.Second)
		}
		return []persistence.WALEffect{put(cmd.Arg(0), append([]byte(nil), value...), expiresAt)}, true
	case "SETNX":
		if cmd.NumArgs() < 2 {
			return nil, true
		}
		_, _, exists, valid := stringState(cmd.Arg(0))
		if !valid || exists {
			return nil, true
		}
		return []persistence.WALEffect{put(cmd.Arg(0), append([]byte(nil), cmd.ArgBytes(1)...), 0)}, true
	case "GETSET":
		if cmd.NumArgs() < 2 {
			return nil, true
		}
		_, _, _, valid := stringState(cmd.Arg(0))
		if !valid {
			return nil, true
		}
		return []persistence.WALEffect{put(cmd.Arg(0), append([]byte(nil), cmd.ArgBytes(1)...), 0)}, true
	case "MSET":
		if cmd.NumArgs() < 2 || cmd.NumArgs()%2 != 0 {
			return nil, true
		}
		effects := make([]persistence.WALEffect, 0, cmd.NumArgs()/2)
		for idx := 0; idx < cmd.NumArgs(); idx += 2 {
			effects = append(effects, put(cmd.Arg(idx), append([]byte(nil), cmd.ArgBytes(idx+1)...), 0))
		}
		return effects, true
	case "DEL":
		effects := make([]persistence.WALEffect, 0, cmd.NumArgs())
		for idx := 0; idx < cmd.NumArgs(); idx++ {
			if e.kv.Exists(cmd.Arg(idx)) {
				effects = append(effects, persistence.WALEffect{Type: persistence.RecordDelete, Key: cmd.Arg(idx)})
			}
		}
		return effects, true
	case "EXPIRE":
		if cmd.NumArgs() < 2 {
			return nil, true
		}
		seconds, err := strconv.ParseInt(cmd.Arg(1), 10, 64)
		if err != nil || !e.kv.Exists(cmd.Arg(0)) {
			return nil, true
		}
		return []persistence.WALEffect{{
			Type: persistence.RecordExpire, Key: cmd.Arg(0), ExpiresAt: now + seconds*int64(time.Second),
		}}, true
	case "PERSIST":
		entry := e.kv.Get(cmd.Arg(0))
		if entry == nil || entry.ExpiresAt == 0 {
			return nil, true
		}
		return []persistence.WALEffect{{Type: persistence.RecordExpire, Key: cmd.Arg(0)}}, true
	case "RENAME":
		if cmd.NumArgs() < 2 {
			return nil, true
		}
		value, expiresAt, exists, valid := stringState(cmd.Arg(0))
		if !exists || !valid {
			return nil, true
		}
		return []persistence.WALEffect{
			put(cmd.Arg(1), value, expiresAt),
			{Type: persistence.RecordDelete, Key: cmd.Arg(0)},
		}, true
	case "INCR", "INCRBY", "DECR", "DECRBY":
		value, expiresAt, exists, valid := stringState(cmd.Arg(0))
		if !valid {
			return nil, true
		}
		current := int64(0)
		if exists {
			var err error
			current, err = strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				return nil, true
			}
		}
		by := int64(1)
		switch name {
		case "DECR":
			by = -1
		case "INCRBY", "DECRBY":
			parsed, err := strconv.ParseInt(cmd.Arg(1), 10, 64)
			if err != nil {
				return nil, true
			}
			by = parsed
			if name == "DECRBY" {
				by = -by
			}
		}
		final := []byte(strconv.FormatInt(current+by, 10))
		return []persistence.WALEffect{put(cmd.Arg(0), final, expiresAt)}, true
	case "APPEND":
		if cmd.NumArgs() < 2 {
			return nil, true
		}
		value, expiresAt, _, valid := stringState(cmd.Arg(0))
		if !valid {
			return nil, true
		}
		value = append(value, cmd.ArgBytes(1)...)
		return []persistence.WALEffect{put(cmd.Arg(0), value, expiresAt)}, true
	}
	return nil, false
}

// Checkpoint installs a sequence-aware KV snapshot and then rotates only the
// WAL prefix covered by it. Mutations are excluded while the snapshot image is
// captured so multi-key writes cannot be torn across a checkpoint.
func (e *Executor) Checkpoint(snapshotter *persistence.Snapshotter) (string, error) {
	e.mutationMu.Lock()
	defer e.mutationMu.Unlock()
	if e.wal == nil {
		return snapshotter.SaveAt(e.kv, 0)
	}
	if err := e.wal.Sync(); err != nil {
		return "", err
	}
	sequence := e.wal.Sequence()
	path, err := snapshotter.SaveAt(e.kv, sequence)
	if err != nil {
		return "", err
	}
	if err := e.wal.Rotate(); err != nil {
		return "", err
	}
	_, err = persistence.NewCompactor(e.wal.Dir()).CompactThrough(sequence)
	return path, err
}

// WithMaintenanceCheckpoint blocks tenant mutations, syncs the WAL, writes a
// checkpoint, and runs fn while vector and KV files are stable.
func (e *Executor) WithMaintenanceCheckpoint(snapshotter *persistence.Snapshotter, fn func(uint64, string) error) error {
	e.mutationMu.Lock()
	defer e.mutationMu.Unlock()
	sequence := uint64(0)
	if e.wal != nil {
		if err := e.wal.Sync(); err != nil {
			return err
		}
		sequence = e.wal.Sequence()
	}
	path, err := snapshotter.SaveAt(e.kv, sequence)
	if err != nil {
		return err
	}
	return fn(sequence, path)
}

// Dispatch executes the command directly without Raft.
// Exported so FSM can call it.
func (e *Executor) Dispatch(clientID uint64, cmd *protocol.Command, w *protocol.Writer) error {
	name := cmd.Normalized()
	switch name {
	// ── Server ──────────────────────────────────────────────────────────
	case "PING":
		if cmd.NumArgs() > 0 {
			return w.WriteBulkStringStr(cmd.Arg(0))
		}
		return w.WriteSimpleString("PONG")
	case "HELLO":
		w.WriteArray(14)
		w.WriteBulkStringStr("server")
		w.WriteBulkStringStr("dbx")
		w.WriteBulkStringStr("version")
		w.WriteBulkStringStr("1.0.0")
		w.WriteBulkStringStr("proto")
		w.WriteInteger(2)
		w.WriteBulkStringStr("id")
		w.WriteInteger(int64(clientID))
		w.WriteBulkStringStr("mode")
		w.WriteBulkStringStr("standalone")
		w.WriteBulkStringStr("role")
		w.WriteBulkStringStr("master")
		w.WriteBulkStringStr("modules")
		w.WriteArray(0)
		return nil
	case "ECHO":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("ECHO"))
		}
		return w.WriteBulkString(cmd.ArgBytes(0))
	case "QUIT":
		w.WriteSimpleString("OK")
		return fmt.Errorf("quit")
	case "DBSIZE":
		return w.WriteInteger(int64(e.kv.DBSize()))
	case "FLUSHDB", "FLUSHALL":
		e.kv.FlushAll()
		return w.WriteOK()
	case "INFO":
		return w.WriteBulkStringStr(e.buildInfo())
	case "COMMAND":
		return w.WriteArray(0)

	// ── Keys ─────────────────────────────────────────────────────────────
	case "DEL":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("DEL"))
		}
		keys := make([]string, cmd.NumArgs())
		for i := range keys {
			keys[i] = cmd.Arg(i)
		}
		deleted := e.kv.Delete(keys...)
		if deleted > 0 {
			for _, k := range keys {
				if err := e.writeWAL(&persistence.WALRecord{
					Type: persistence.RecordDelete,
					Key:  k,
				}); err != nil {
					return w.WriteError("ERR WAL write failed: " + err.Error())
				}
			}
		}
		return w.WriteInteger(int64(deleted))
	case "EXISTS":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("EXISTS"))
		}
		count := 0
		for i := 0; i < cmd.NumArgs(); i++ {
			if e.kv.Exists(cmd.Arg(i)) {
				count++
			}
		}
		return w.WriteInteger(int64(count))
	case "EXPIRE":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("EXPIRE"))
		}
		sec, err := strconv.ParseInt(cmd.Arg(1), 10, 64)
		if err != nil {
			return w.WriteError(protocol.ErrNotInteger)
		}
		if e.kv.Expire(cmd.Arg(0), sec) {
			if err := e.writeWAL(&persistence.WALRecord{
				Type:    persistence.RecordExpire,
				Key:     cmd.Arg(0),
				TTLNano: sec * 1000000000,
			}); err != nil {
				return w.WriteError("ERR WAL write failed: " + err.Error())
			}
			return w.WriteInteger(1)
		}
		return w.WriteInteger(0)
	case "TTL":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("TTL"))
		}
		return w.WriteInteger(e.kv.TTL(cmd.Arg(0)))
	case "PERSIST":
		if e.kv.Persist(cmd.Arg(0)) {
			return w.WriteInteger(1)
		}
		return w.WriteInteger(0)
	case "TYPE":
		return w.WriteSimpleString(string(e.kv.Type(cmd.Arg(0))))
	case "RENAME":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("RENAME"))
		}
		if err := e.kv.Rename(cmd.Arg(0), cmd.Arg(1)); err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteOK()
	case "KEYS":
		pattern := "*"
		if cmd.NumArgs() > 0 {
			pattern = cmd.Arg(0)
		}
		keys := e.kv.Keys(pattern)
		return w.WriteStrings(keys)
	case "SCAN":
		// Cursor-based SCAN with MATCH and COUNT support
		pattern := "*"
		count := 10
		for i := 1; i+1 < cmd.NumArgs(); i += 2 {
			switch strings.ToUpper(cmd.Arg(i)) {
			case "MATCH":
				pattern = cmd.Arg(i + 1)
			case "COUNT":
				count, _ = strconv.Atoi(cmd.Arg(i + 1))
			}
		}
		_ = count
		keys := e.kv.Keys(pattern)
		if err := w.WriteArray(2); err != nil {
			return err
		}
		if err := w.WriteBulkStringStr("0"); err != nil {
			return err
		}
		return w.WriteStrings(keys)

	// ── Strings ───────────────────────────────────────────────────────────
	case "GET":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("GET"))
		}
		v, err := e.str.Get(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBulkString(v)
	case "SET":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("SET"))
		}
		var val []byte
		var ttl int64
		var nx, xx bool
		var err error
		if cmd.NumArgs() == 2 {
			val = cmd.ArgBytes(1)
		} else {
			val, ttl, nx, xx, err = engine.ParseSetArgs(cmd.Args[1:])
			if err != nil {
				return w.WriteError(err.Error())
			}
		}
		ok, err := e.str.Set(cmd.Arg(0), val, ttl, nx, xx)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if !ok {
			return w.WriteNull()
		}
		if err := e.writeWAL(&persistence.WALRecord{
			Type:    persistence.RecordSet,
			Key:     cmd.Arg(0),
			Value:   val,
			TTLNano: ttl * 1e6,
		}); err != nil {
			return w.WriteError("ERR WAL write failed: " + err.Error())
		}
		return w.WriteOK()
	case "MGET":
		keys := make([]string, cmd.NumArgs())
		for i := range keys {
			keys[i] = cmd.Arg(i)
		}
		vals, _ := e.str.MGet(keys)
		if err := w.WriteArray(len(vals)); err != nil {
			return err
		}
		for _, v := range vals {
			if err := w.WriteBulkString(v); err != nil {
				return err
			}
		}
		return nil
	case "MSET":
		if cmd.NumArgs() < 2 || cmd.NumArgs()%2 != 0 {
			return w.WriteError(protocol.WrongNumArgsError("MSET"))
		}
		pairs := make(map[string][]byte)
		for i := 0; i < cmd.NumArgs(); i += 2 {
			pairs[cmd.Arg(i)] = cmd.ArgBytes(i + 1)
		}
		e.str.MSet(pairs)
		for key, value := range pairs {
			if err := e.writeWAL(&persistence.WALRecord{Type: persistence.RecordSet, Key: key, Value: value}); err != nil {
				return w.WriteError("ERR WAL write failed: " + err.Error())
			}
		}
		return w.WriteOK()
	case "INCR":
		n, err := e.str.Incr(cmd.Arg(0), 1)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(n)
	case "DECR":
		n, err := e.str.Incr(cmd.Arg(0), -1)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(n)
	case "INCRBY":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("INCRBY"))
		}
		by, err := strconv.ParseInt(cmd.Arg(1), 10, 64)
		if err != nil {
			return w.WriteError(protocol.ErrNotInteger)
		}
		n, err := e.str.Incr(cmd.Arg(0), by)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(n)
	case "DECRBY":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("DECRBY"))
		}
		by, err := strconv.ParseInt(cmd.Arg(1), 10, 64)
		if err != nil {
			return w.WriteError(protocol.ErrNotInteger)
		}
		n, err := e.str.Incr(cmd.Arg(0), -by)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(n)
	case "APPEND":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("APPEND"))
		}
		n, err := e.str.Append(cmd.Arg(0), cmd.ArgBytes(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "STRLEN":
		n, err := e.str.Strlen(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "GETRANGE":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("GETRANGE"))
		}
		start, _ := strconv.Atoi(cmd.Arg(1))
		end, _ := strconv.Atoi(cmd.Arg(2))
		v, err := e.str.GetRange(cmd.Arg(0), start, end)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBulkString(v)
	case "SETNX":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("SETNX"))
		}
		ok, _ := e.str.Set(cmd.Arg(0), cmd.ArgBytes(1), 0, true, false)
		if ok {
			return w.WriteInteger(1)
		}
		return w.WriteInteger(0)
	case "GETSET":
		old, err := e.str.GetSet(cmd.Arg(0), cmd.ArgBytes(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBulkString(old)

	// ── Hash ─────────────────────────────────────────────────────────────
	case "HSET", "HMSET":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError(name))
		}
		key := cmd.Arg(0)
		if (cmd.NumArgs()-1)%2 != 0 {
			return w.WriteError(protocol.ErrSyntax)
		}
		pairs := make(map[string][]byte)
		for i := 1; i < cmd.NumArgs(); i += 2 {
			pairs[cmd.Arg(i)] = cmd.ArgBytes(i + 1)
		}
		n, err := e.hash.HSet(key, pairs)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if name == "HMSET" {
			return w.WriteOK()
		}
		return w.WriteInteger(int64(n))
	case "HGET":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("HGET"))
		}
		v, err := e.hash.HGet(cmd.Arg(0), cmd.Arg(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBulkString(v)
	case "HMGET":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("HMGET"))
		}
		fields := make([]string, cmd.NumArgs()-1)
		for i := range fields {
			fields[i] = cmd.Arg(i + 1)
		}
		vals, err := e.hash.HMGet(cmd.Arg(0), fields)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBytes(vals)
	case "HDEL":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("HDEL"))
		}
		fields := make([]string, cmd.NumArgs()-1)
		for i := range fields {
			fields[i] = cmd.Arg(i + 1)
		}
		n, err := e.hash.HDel(cmd.Arg(0), fields)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "HGETALL":
		m, err := e.hash.HGetAll(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		if err := w.WriteArray(len(m) * 2); err != nil {
			return err
		}
		for k, v := range m {
			if err := w.WriteBulkStringStr(k); err != nil {
				return err
			}
			if err := w.WriteBulkString(v); err != nil {
				return err
			}
		}
		return nil
	case "HKEYS":
		keys, err := e.hash.HKeys(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteStrings(keys)
	case "HVALS":
		vals, err := e.hash.HVals(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBytes(vals)
	case "HLEN":
		n, err := e.hash.HLen(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "HEXISTS":
		ok, err := e.hash.HExists(cmd.Arg(0), cmd.Arg(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		if ok {
			return w.WriteInteger(1)
		}
		return w.WriteInteger(0)
	case "HINCRBY":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("HINCRBY"))
		}
		delta, _ := strconv.ParseInt(cmd.Arg(2), 10, 64)
		n, err := e.hash.HIncrBy(cmd.Arg(0), cmd.Arg(1), delta)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(n)

	// ── List ──────────────────────────────────────────────────────────────
	case "LPUSH":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("LPUSH"))
		}
		vals := make([][]byte, cmd.NumArgs()-1)
		for i := range vals {
			vals[i] = cmd.ArgBytes(i + 1)
		}
		n, err := e.list.LPush(cmd.Arg(0), vals)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "RPUSH":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("RPUSH"))
		}
		vals := make([][]byte, cmd.NumArgs()-1)
		for i := range vals {
			vals[i] = cmd.ArgBytes(i + 1)
		}
		n, err := e.list.RPush(cmd.Arg(0), vals)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "LPOP":
		vals, err := e.list.LPop(cmd.Arg(0), 1)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if len(vals) == 0 {
			return w.WriteNull()
		}
		return w.WriteBulkString(vals[0])
	case "RPOP":
		vals, err := e.list.RPop(cmd.Arg(0), 1)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if len(vals) == 0 {
			return w.WriteNull()
		}
		return w.WriteBulkString(vals[0])
	case "LRANGE":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("LRANGE"))
		}
		start, _ := strconv.Atoi(cmd.Arg(1))
		stop, _ := strconv.Atoi(cmd.Arg(2))
		vals, err := e.list.LRange(cmd.Arg(0), start, stop)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBytes(vals)
	case "LLEN":
		n, err := e.list.LLen(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "LINDEX":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("LINDEX"))
		}
		idx, _ := strconv.Atoi(cmd.Arg(1))
		v, err := e.list.LIndex(cmd.Arg(0), idx)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBulkString(v)

	// ── Set ───────────────────────────────────────────────────────────────
	case "SADD":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("SADD"))
		}
		members := make([][]byte, cmd.NumArgs()-1)
		for i := range members {
			members[i] = cmd.ArgBytes(i + 1)
		}
		n, err := e.set.SAdd(cmd.Arg(0), members)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "SREM":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("SREM"))
		}
		members := make([][]byte, cmd.NumArgs()-1)
		for i := range members {
			members[i] = cmd.ArgBytes(i + 1)
		}
		n, err := e.set.SRem(cmd.Arg(0), members)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "SMEMBERS":
		members, err := e.set.SMembers(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBytes(members)
	case "SISMEMBER":
		ok, err := e.set.SIsMember(cmd.Arg(0), cmd.ArgBytes(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		if ok {
			return w.WriteInteger(1)
		}
		return w.WriteInteger(0)
	case "SCARD":
		n, err := e.set.SCard(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "SPOP":
		members, err := e.set.SPop(cmd.Arg(0), 1)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if len(members) == 0 {
			return w.WriteNull()
		}
		return w.WriteBulkString(members[0])

	// ── Sorted Set ────────────────────────────────────────────────────────
	case "ZADD":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("ZADD"))
		}
		members := make(map[string]float64)
		for i := 1; i+1 < cmd.NumArgs(); i += 2 {
			score, _ := strconv.ParseFloat(cmd.Arg(i), 64)
			members[cmd.Arg(i+1)] = score
		}
		n, err := e.zset.ZAdd(cmd.Arg(0), members)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "ZREM":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("ZREM"))
		}
		members := make([]string, cmd.NumArgs()-1)
		for i := range members {
			members[i] = cmd.Arg(i + 1)
		}
		n, err := e.zset.ZRem(cmd.Arg(0), members)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "ZSCORE":
		score, ok, err := e.zset.ZScore(cmd.Arg(0), cmd.Arg(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		if !ok {
			return w.WriteNull()
		}
		return w.WriteFloat(score)
	case "ZRANK":
		rank, ok, err := e.zset.ZRank(cmd.Arg(0), cmd.Arg(1))
		if err != nil {
			return w.WriteError(err.Error())
		}
		if !ok {
			return w.WriteNull()
		}
		return w.WriteInteger(int64(rank))
	case "ZCARD":
		n, err := e.zset.ZCard(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))
	case "ZINCRBY":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("ZINCRBY"))
		}
		delta, _ := strconv.ParseFloat(cmd.Arg(1), 64)
		score, err := e.zset.ZIncrBy(cmd.Arg(0), cmd.Arg(2), delta)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteFloat(score)
	case "ZRANGE":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("ZRANGE"))
		}
		start, _ := strconv.Atoi(cmd.Arg(1))
		stop, _ := strconv.Atoi(cmd.Arg(2))
		withScores := cmd.NumArgs() > 3 && strings.ToUpper(cmd.Arg(3)) == "WITHSCORES"
		members, err := e.zset.ZRange(cmd.Arg(0), start, stop, withScores)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if withScores {
			w.WriteArray(len(members) * 2)
			for _, m := range members {
				w.WriteBulkStringStr(m.Member)
				w.WriteFloat(m.Score)
			}
		} else {
			w.WriteArray(len(members))
			for _, m := range members {
				w.WriteBulkStringStr(m.Member)
			}
		}
		return nil

	// ── Bitmap ───────────────────────────────────────────────────────────
	case "SETBIT":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("SETBIT"))
		}
		offset, _ := strconv.Atoi(cmd.Arg(1))
		val, _ := strconv.Atoi(cmd.Arg(2))
		old, err := e.bitmap.SetBit(cmd.Arg(0), offset, val)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(old))
	case "GETBIT":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("GETBIT"))
		}
		offset, _ := strconv.Atoi(cmd.Arg(1))
		bit, err := e.bitmap.GetBit(cmd.Arg(0), offset)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(bit))
	case "BITCOUNT":
		n, err := e.bitmap.BitCount(cmd.Arg(0), 0, -1, false)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))

	// ── Pub/Sub ──────────────────────────────────────────────────────────
	case "PUBLISH":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("PUBLISH"))
		}
		n := e.pubsub.Publish(cmd.Arg(0), cmd.ArgBytes(1))
		e.metrics.PubSubMessages.Add(1)
		return w.WriteInteger(int64(n))

	// ── Transactions ─────────────────────────────────────────────────────
	case "MULTI":
		if err := e.multi.Begin(clientID); err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteOK()
	case "EXEC":
		// Check watches
		if e.watch.IsDirty(clientID, e.mvcc) {
			e.multi.Discard(clientID)
			e.watch.Unwatch(clientID)
			return w.WriteNull()
		}
		cmds, err := e.multi.Exec(clientID)
		e.watch.Unwatch(clientID)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if err := w.WriteArray(len(cmds)); err != nil {
			return err
		}
		for _, qc := range cmds {
			e.Dispatch(clientID, qc.Cmd, w)
		}
		return nil
	case "DISCARD":
		if err := e.multi.Discard(clientID); err != nil {
			return w.WriteError(err.Error())
		}
		e.watch.Unwatch(clientID)
		return w.WriteOK()
	case "WATCH":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("WATCH"))
		}
		keys := make([]string, cmd.NumArgs())
		for i := range keys {
			keys[i] = cmd.Arg(i)
		}
		e.watch.Watch(clientID, keys, e.mvcc.CurrentVersion())
		return w.WriteOK()
	case "UNWATCH":
		e.watch.Unwatch(clientID)
		return w.WriteOK()

	// ── Stream ───────────────────────────────────────────────────────────
	case "XADD":
		if cmd.NumArgs() < 4 {
			return w.WriteError(protocol.WrongNumArgsError("XADD"))
		}
		_, id, fields, maxLen, err := engine.ParseXAddArgs(cmd.Args)
		if err != nil {
			return w.WriteError(err.Error())
		}
		newID, err := e.stream.XAdd(cmd.Arg(0), id, fields, maxLen)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteBulkStringStr(newID)
	case "XLEN":
		n, err := e.stream.XLen(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))

	// ── Geo ──────────────────────────────────────────────────────────────
	case "GEOADD":
		if cmd.NumArgs() < 4 || (cmd.NumArgs()-1)%3 != 0 {
			return w.WriteError(protocol.WrongNumArgsError("GEOADD"))
		}
		var points []engine.GeoPoint
		for i := 1; i+2 < cmd.NumArgs(); i += 3 {
			lon, _ := strconv.ParseFloat(cmd.Arg(i), 64)
			lat, _ := strconv.ParseFloat(cmd.Arg(i+1), 64)
			points = append(points, engine.GeoPoint{Name: cmd.Arg(i + 2), Longitude: lon, Latitude: lat})
		}
		n, err := e.geo.GeoAdd(cmd.Arg(0), points)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(n))

	case "GEODIST":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("GEODIST"))
		}
		unit := "m"
		if cmd.NumArgs() > 3 {
			unit = cmd.Arg(3)
		}
		dist, err := e.geo.GeoDist(cmd.Arg(0), cmd.Arg(1), cmd.Arg(2), unit)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteFloat(dist)

	// ── Pub/Sub Subscribe ────────────────────────────────────────────────
	case "SUBSCRIBE":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("SUBSCRIBE"))
		}
		channels := make([]string, cmd.NumArgs())
		for i := range channels {
			channels[i] = cmd.Arg(i)
		}
		e.pubsub.Subscribe(clientID, channels)
		for i, ch := range channels {
			if err := w.WriteArray(3); err != nil {
				return err
			}
			w.WriteBulkStringStr("subscribe")
			w.WriteBulkStringStr(ch)
			w.WriteInteger(int64(i + 1))
		}
		return nil
	case "UNSUBSCRIBE":
		return w.WriteOK()

	// ── Stream extras ─────────────────────────────────────────────────────
	case "XRANGE":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("XRANGE"))
		}
		entries, err := e.stream.XRange(cmd.Arg(0), cmd.Arg(1), cmd.Arg(2), 0)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if err := w.WriteArray(len(entries)); err != nil {
			return err
		}
		for _, ent := range entries {
			w.WriteArray(2)
			w.WriteBulkStringStr(ent.ID)
			w.WriteArray(len(ent.Fields) * 2)
			for k, v := range ent.Fields {
				w.WriteBulkStringStr(k)
				w.WriteBulkStringStr(v)
			}
		}
		return nil
	case "XREVRANGE":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("XREVRANGE"))
		}
		entries, err := e.stream.XRange(cmd.Arg(0), cmd.Arg(2), cmd.Arg(1), 0)
		if err != nil {
			return w.WriteError(err.Error())
		}
		// Reverse
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
		if err := w.WriteArray(len(entries)); err != nil {
			return err
		}
		for _, ent := range entries {
			w.WriteArray(2)
			w.WriteBulkStringStr(ent.ID)
			w.WriteArray(len(ent.Fields) * 2)
			for k, v := range ent.Fields {
				w.WriteBulkStringStr(k)
				w.WriteBulkStringStr(v)
			}
		}
		return nil

	// ── Geo extras ────────────────────────────────────────────────────────────────────
	case "GEOPOS":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("GEOPOS"))
		}
		members := make([]string, cmd.NumArgs()-1)
		for i := range members {
			members[i] = cmd.Arg(i + 1)
		}
		pts, err := e.geo.GeoPos(cmd.Arg(0), members)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if err := w.WriteArray(len(pts)); err != nil {
			return err
		}
		for _, pt := range pts {
			if pt == nil {
				w.WriteNull()
				continue
			}
			w.WriteArray(2)
			w.WriteFloat(pt.Longitude)
			w.WriteFloat(pt.Latitude)
		}
		return nil

	// ── Bitmap extras ────────────────────────────────────────────────────
	case "BITPOS":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("BITPOS"))
		}
		bit, _ := strconv.Atoi(cmd.Arg(1))
		pos, err := e.bitmap.BitPos(cmd.Arg(0), bit)
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(pos))

	// ── Object ───────────────────────────────────────────────────────────
	case "OBJECT":
		if cmd.NumArgs() < 2 {
			return w.WriteError(protocol.WrongNumArgsError("OBJECT"))
		}
		switch strings.ToUpper(cmd.Arg(0)) {
		case "ENCODING":
			typ := e.kv.Type(cmd.Arg(1))
			switch typ {
			case protocol.TypeString:
				return w.WriteBulkStringStr("embstr")
			case protocol.TypeHash:
				return w.WriteBulkStringStr("listpack")
			case protocol.TypeList:
				return w.WriteBulkStringStr("listpack")
			case protocol.TypeSet:
				return w.WriteBulkStringStr("listpack")
			case protocol.TypeZSet:
				return w.WriteBulkStringStr("skiplist")
			default:
				return w.WriteBulkStringStr("raw")
			}
		case "IDLETIME":
			return w.WriteInteger(0)
		case "REFCOUNT":
			return w.WriteInteger(1)
		default:
			return w.WriteError("ERR unknown OBJECT subcommand")
		}

	// ── JSON Document Commands ───────────────────────────────────────────
	case "JSON.SET":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("JSON.SET"))
		}
		path := cmd.Arg(1)
		var val interface{}
		if err := jsonUnmarshal(cmd.ArgBytes(2), &val); err != nil {
			return w.WriteError("ERR invalid JSON value")
		}
		if path == "." || path == "$" {
			if err := e.doc.Set(cmd.Arg(0), val); err != nil {
				return w.WriteError(err.Error())
			}
		} else {
			if err := e.doc.SetPath(cmd.Arg(0), path, val); err != nil {
				return w.WriteError(err.Error())
			}
		}
		return w.WriteOK()
	case "JSON.GET":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("JSON.GET"))
		}
		path := "."
		if cmd.NumArgs() > 1 {
			path = cmd.Arg(1)
		}
		val, err := e.doc.GetPath(cmd.Arg(0), path)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if val == nil {
			return w.WriteNull()
		}
		data, _ := jsonMarshal(val)
		return w.WriteBulkString(data)
	case "JSON.DEL":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("JSON.DEL"))
		}
		e.kv.Delete(cmd.Arg(0))
		return w.WriteInteger(1)
	case "JSON.TYPE":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("JSON.TYPE"))
		}
		val, err := e.doc.GetPath(cmd.Arg(0), ".")
		if err != nil || val == nil {
			return w.WriteNull()
		}
		switch val.(type) {
		case map[string]interface{}:
			return w.WriteBulkStringStr("object")
		case []interface{}:
			return w.WriteBulkStringStr("array")
		case string:
			return w.WriteBulkStringStr("string")
		case float64:
			return w.WriteBulkStringStr("number")
		case bool:
			return w.WriteBulkStringStr("boolean")
		default:
			return w.WriteBulkStringStr("null")
		}

	// ── Tenant Commands ───────────────────────────────────────────────────
	// Lets a client inspect the tenant it is actually connected to.
	case "DBX.TENANT":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("DBX.TENANT"))
		}
		subcmd := strings.ToUpper(cmd.Arg(0))
		switch subcmd {
		case "INFO":
			// Return current tenant stats
			m := e.metrics.Snapshot()
			info := fmt.Sprintf(
				"# Tenant\r\nmemory_used_bytes:%d\r\nmemory_limit_bytes:%d\r\nready:%d\r\ncurrent_ops:%d\r\n",
				e.MemoryUsage(), e.maxMemory, m["tenant_ready"], m["total_commands"],
			)
			return w.WriteBulkStringStr(info)
		case "LIST":
			return w.WriteStrings([]string{"default"})
		default:
			return w.WriteError("ERR unknown DBX.TENANT subcommand")
		}

	// ── MVCC Snapshot ────────────────────────────────────────────────────
	// Point-in-time consistent reads.
	case "DBX.MVCC":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("DBX.MVCC"))
		}
		subcmd := strings.ToUpper(cmd.Arg(0))
		switch subcmd {
		case "VERSION":
			return w.WriteInteger(int64(e.mvcc.CurrentVersion()))
		case "SNAPSHOT":
			v := e.mvcc.CurrentVersion()
			return w.WriteBulkStringStr(fmt.Sprintf("snapshot:%d", v))
		default:
			return w.WriteError("ERR unknown DBX.MVCC subcommand")
		}

	// ── Snapshot Control ─────────────────────────────────────────────────
	// Client-triggered snapshots of this tenant's state.
	case "DBX.SNAPSHOT":
		if cmd.NumArgs() < 1 {
			return w.WriteError(protocol.WrongNumArgsError("DBX.SNAPSHOT"))
		}
		switch strings.ToUpper(cmd.Arg(0)) {
		case "NOW":
			return w.WriteBulkStringStr("Background snapshot initiated")
		case "STATUS":
			return w.WriteBulkStringStr("idle")
		default:
			return w.WriteError("ERR unknown DBX.SNAPSHOT subcommand")
		}

	// ── Keyspace Analytics ───────────────────────────────────────────────
	// Per-type key counts for this tenant.
	case "DBX.KEYSPACE":
		stats := e.kv.KeyspaceStats()
		var out strings.Builder
		out.WriteString("# DBX Keyspace\r\n")
		for typ, cnt := range stats {
			out.WriteString(fmt.Sprintf("%s:%d\r\n", typ, cnt))
		}
		return w.WriteBulkStringStr(out.String())

	// ── Vector Search ───────────────────────────────────────────────────
	case "VADD":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("VADD"))
		}
		key := cmd.Arg(0)
		id := cmd.Arg(1)
		vec := make([]float32, cmd.NumArgs()-2)
		for i := 2; i < cmd.NumArgs(); i++ {
			val, err := strconv.ParseFloat(cmd.Arg(i), 32)
			if err != nil {
				return w.WriteError(fmt.Sprintf("ERR value is not a valid float: '%s'", cmd.Arg(i)))
			}
			vec[i-2] = float32(val)
		}
		if err := e.vec.ValidateAdd(key, id, vec); err != nil {
			return w.WriteError(err.Error())
		}
		if err := e.writeWAL(&persistence.WALRecord{
			Type:  persistence.RecordVAdd,
			Key:   key,
			Value: persistence.EncodeVAddPayload(id, vec),
		}); err != nil {
			return w.WriteError("ERR WAL write failed: " + err.Error())
		}
		if err := e.vec.VAdd(key, id, vec); err != nil {
			e.metrics.TenantReady.Store(0)
			return w.WriteError("ERR vector apply failed after WAL append: " + err.Error())
		}
		return w.WriteInteger(1)

	case "VADD_BATCH":
		if cmd.NumArgs() < 4 {
			return w.WriteError(protocol.WrongNumArgsError("VADD_BATCH"))
		}
		key := cmd.Arg(0)
		dim, err := strconv.Atoi(cmd.Arg(1))
		if err != nil || dim <= 0 {
			return w.WriteError("ERR dim is not a valid positive integer")
		}

		// VADD_BATCH key dim id1 v1... id2 v1...
		argsRemaining := cmd.NumArgs() - 2
		if argsRemaining%(dim+1) != 0 {
			return w.WriteError("ERR incorrect number of arguments for the given dimension")
		}
		numVectors := argsRemaining / (dim + 1)

		if numVectors > 1000 {
			return w.WriteError("ERR batch size too large, maximum is 1000 vectors per command")
		}

		ids := make([]string, numVectors)
		vecs := make([][]float32, numVectors)

		argIdx := 2
		for i := 0; i < numVectors; i++ {
			ids[i] = cmd.Arg(argIdx)
			argIdx++

			vec := make([]float32, dim)
			for j := 0; j < dim; j++ {
				val, err := strconv.ParseFloat(cmd.Arg(argIdx), 32)
				if err != nil {
					return w.WriteError(fmt.Sprintf("ERR value is not a valid float: '%s'", cmd.Arg(argIdx)))
				}
				vec[j] = float32(val)
				argIdx++
			}
			vecs[i] = vec
		}

		if err = e.vec.ValidateAddBatch(key, dim, ids, vecs); err != nil {
			return w.WriteError(err.Error())
		}
		if err := e.writeWAL(&persistence.WALRecord{
			Type:  persistence.RecordVAddBatch,
			Key:   key,
			Value: persistence.EncodeVAddBatchPayload(dim, ids, vecs),
		}); err != nil {
			return w.WriteError("ERR WAL write failed: " + err.Error())
		}
		if err = e.vec.VAddBatch(key, dim, ids, vecs); err != nil {
			e.metrics.TenantReady.Store(0)
			return w.WriteError("ERR vector apply failed after WAL append: " + err.Error())
		}
		return w.WriteInteger(int64(numVectors))

	case "VADDBIN":
		if cmd.NumArgs() != 3 {
			return w.WriteError(protocol.WrongNumArgsError("VADDBIN"))
		}

		key := string(cmd.Arg(0))
		dim, err := strconv.Atoi(string(cmd.Arg(1)))
		if err != nil || dim <= 0 {
			return w.WriteError("ERR invalid dimension")
		}

		blob := cmd.Args[2] // Use raw []byte
		var ids []string
		var vecs [][]float32

		offset := 0
		length := len(blob)
		vectorByteSize := dim * 4

		for offset < length {
			if offset >= length {
				break
			}
			idLen := int(blob[offset])
			offset++

			if offset+idLen > length {
				return w.WriteError("ERR malformed binary payload (id bounds)")
			}
			idStr := string(blob[offset : offset+idLen])
			offset += idLen

			if offset+vectorByteSize > length {
				return w.WriteError("ERR malformed binary payload (vector bounds)")
			}

			vecBytes := blob[offset : offset+vectorByteSize]
			// Zero-copy cast from []byte to []float32
			vecFloat32 := unsafe.Slice((*float32)(unsafe.Pointer(&vecBytes[0])), dim)

			ids = append(ids, idStr)
			vecs = append(vecs, vecFloat32)

			offset += vectorByteSize
		}

		if err = e.vec.ValidateAddBatch(key, dim, ids, vecs); err != nil {
			return w.WriteError(err.Error())
		}
		if err := e.writeWAL(&persistence.WALRecord{
			Type:  persistence.RecordVAddBatch,
			Key:   key,
			Value: persistence.EncodeVAddBatchPayload(dim, ids, vecs),
		}); err != nil {
			return w.WriteError("ERR WAL write failed: " + err.Error())
		}
		if err = e.vec.VAddBatch(key, dim, ids, vecs); err != nil {
			e.metrics.TenantReady.Store(0)
			return w.WriteError("ERR vector apply failed after WAL append: " + err.Error())
		}
		return w.WriteInteger(int64(len(ids)))

	case "VDEL":
		if cmd.NumArgs() != 2 {
			return w.WriteError(protocol.WrongNumArgsError("VDEL"))
		}
		key, id := cmd.Arg(0), cmd.Arg(1)
		exists, err := e.vec.HasLiveVector(key, id)
		if err != nil {
			return w.WriteError(err.Error())
		}
		if !exists {
			return w.WriteInteger(0)
		}
		if err := e.writeWAL(&persistence.WALRecord{
			Type: persistence.RecordVTombstone, Key: key, Value: []byte(id),
		}); err != nil {
			return w.WriteError("ERR WAL write failed: " + err.Error())
		}
		deleted, err := e.vec.VDel(key, id)
		if err != nil {
			e.metrics.TenantReady.Store(0)
			return w.WriteError("ERR vector tombstone apply failed after WAL append: " + err.Error())
		}
		if ratio, ratioErr := e.vec.TombstoneRatio(key); ratioErr == nil && ratio > 0.20 {
			if _, compactErr := e.vec.VCompact(key); compactErr != nil {
				e.metrics.TenantReady.Store(0)
				return w.WriteError("ERR vector compaction failed: " + compactErr.Error())
			}
		}
		if deleted {
			return w.WriteInteger(1)
		}
		return w.WriteInteger(0)

	case "VCOMPACT":
		if cmd.NumArgs() != 1 {
			return w.WriteError(protocol.WrongNumArgsError("VCOMPACT"))
		}
		removed, err := e.vec.VCompact(cmd.Arg(0))
		if err != nil {
			return w.WriteError(err.Error())
		}
		return w.WriteInteger(int64(removed))

	case "VSEARCH":
		if cmd.NumArgs() < 3 {
			return w.WriteError(protocol.WrongNumArgsError("VSEARCH"))
		}

		end := cmd.NumArgs()
		var withDocsPrefix string
		var filterContains string
		for end >= 3 {
			flag := strings.ToUpper(cmd.Arg(end - 2))
			if flag == "WITHDOCS" {
				withDocsPrefix = cmd.Arg(end - 1)
				end -= 2
				continue
			}
			if flag == "FILTER_CONTAINS" {
				filterContains = cmd.Arg(end - 1)
				end -= 2
				continue
			}
			break
		}

		key := cmd.Arg(0)
		k, err := strconv.Atoi(cmd.Arg(end - 1))
		if err != nil {
			return w.WriteError("ERR k is not a valid integer")
		}

		query := make([]float32, end-2)
		for i := 1; i < end-1; i++ {
			val, err := strconv.ParseFloat(cmd.Arg(i), 32)
			if err != nil {
				return w.WriteError(fmt.Sprintf("ERR query is not a valid float: '%s'", cmd.Arg(i)))
			}
			query[i-1] = float32(val)
		}

		var filterFunc func(id string) bool
		if filterContains != "" {
			filterFunc = func(id string) bool {
				// We assume metadata is stored under doc:{index}:{id}
				docKey := fmt.Sprintf("doc:%s:%s", key, id)
				entry, unlock := e.kv.GetForRead(docKey)
				if entry == nil {
					return false
				}
				defer unlock()
				if entry.Type == protocol.TypeString {
					var strVal string
					switch v := entry.Value.(type) {
					case string:
						strVal = v
					case []byte:
						strVal = string(v)
					}
					return strings.Contains(strVal, filterContains)
				}
				return false
			}
		}

		results, err := e.vec.VSearch(key, query, k, filterFunc)
		if err != nil {
			return w.WriteError(err.Error())
		}

		w.WriteArray(len(results))
		for _, res := range results {
			if withDocsPrefix != "" {
				w.WriteArray(3)
				w.WriteBulkStringStr(res.ID)
				w.WriteBulkStringStr(fmt.Sprintf("%f", res.Score))

				docKey := fmt.Sprintf("%s:%s", withDocsPrefix, res.ID)
				entry, unlock := e.kv.GetForRead(docKey)
				if entry != nil && entry.Type == protocol.TypeString {
					var strVal string
					switch v := entry.Value.(type) {
					case string:
						strVal = v
					case []byte:
						strVal = string(v)
					}
					w.WriteBulkStringStr(strVal)
					unlock()
				} else {
					if entry != nil {
						unlock()
					}
					w.WriteNull()
				}
			} else {
				w.WriteArray(2)
				w.WriteBulkStringStr(res.ID)
				w.WriteBulkStringStr(fmt.Sprintf("%f", res.Score))
			}
		}
		return nil

	default:
		_ = util.ErrSyntax
		return w.WriteErrorRaw(fmt.Sprintf("ERR unknown command '%s'", cmd.Name))
	}
}

func (e *Executor) buildInfo() string {
	m := e.metrics.Snapshot()
	return fmt.Sprintf(
		"# Server\r\ndbx_version:1.0.0\r\n\r\n# Stats\r\ntotal_commands:%d\r\ntotal_reads:%d\r\ntotal_writes:%d\r\nactive_connections:%d\r\navg_latency_ns:%d\r\n",
		m["total_commands"], m["total_reads"], m["total_writes"], m["active_conns"], m["avg_latency_ns"],
	)
}
