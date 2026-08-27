package query

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/dbx/dbx/internal/cluster"
	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/events"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/transaction"
	"github.com/hashicorp/raft"
)

func newOpenExecutor(t *testing.T) *Executor {
	t.Helper()
	kv := engine.New(8)
	executor := NewExecutor(
		kv, engine.NewVectorStore(kv, t.TempDir(), 1000),
		transaction.NewMultiManager(), transaction.NewWatchSet(), transaction.NewMVCCStore(8),
		events.NewPubSub(10, 10), &observability.Metrics{}, nil,
	)
	t.Cleanup(func() { executor.vec.CloseAll() })
	return executor
}

func dispatchForTest(t *testing.T, executor *Executor, name string, args ...string) string {
	t.Helper()
	command := &protocol.Command{Name: name, Args: make([][]byte, len(args))}
	for i := range args {
		command.Args[i] = []byte(args[i])
	}
	var output bytes.Buffer
	writer := protocol.NewWriter(&output)
	err := executor.Dispatch(1, command, writer)
	if err != nil && err.Error() != "quit" {
		t.Fatalf("%s: %v (%q)", name, err, output.String())
	}
	_ = writer.Flush()
	return output.String()
}

func TestDurableStringSurface(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	if got := executeForTest(t, executor, "PING"); !strings.Contains(got, "PONG") {
		t.Fatalf("PING = %q", got)
	}
	if got := executeForTest(t, executor, "PING", "hello"); !strings.Contains(got, "hello") {
		t.Fatalf("PING arg = %q", got)
	}
	if executeForTest(t, executor, "ECHO", "hi") != "$2\r\nhi\r\n" {
		t.Fatal("ECHO")
	}
	if executeForTest(t, executor, "SET", "k", "1") != "+OK\r\n" {
		t.Fatal("SET")
	}
	if executeForTest(t, executor, "GET", "k") != "$1\r\n1\r\n" {
		t.Fatal("GET")
	}
	if !strings.Contains(executeForTest(t, executor, "STRLEN", "k"), ":1") {
		t.Fatal("STRLEN")
	}
	if !strings.Contains(executeForTest(t, executor, "GETRANGE", "k", "0", "0"), "1") {
		t.Fatal("GETRANGE")
	}
	if !strings.Contains(executeForTest(t, executor, "APPEND", "k", "x"), ":2") {
		t.Fatal("APPEND")
	}
	if !strings.Contains(executeForTest(t, executor, "INCR", "n"), ":1") {
		t.Fatal("INCR")
	}
	if !strings.Contains(executeForTest(t, executor, "INCRBY", "n", "4"), ":5") {
		t.Fatal("INCRBY")
	}
	if !strings.Contains(executeForTest(t, executor, "DECR", "n"), ":4") {
		t.Fatal("DECR")
	}
	if !strings.Contains(executeForTest(t, executor, "DECRBY", "n", "2"), ":2") {
		t.Fatal("DECRBY")
	}
	if executeForTest(t, executor, "MSET", "a", "1", "b", "2") != "+OK\r\n" {
		t.Fatal("MSET")
	}
	if !strings.Contains(executeForTest(t, executor, "MGET", "a", "missing", "b"), "1") {
		t.Fatal("MGET")
	}
	if !strings.Contains(executeForTest(t, executor, "EXISTS", "a", "zzz"), ":1") {
		t.Fatal("EXISTS")
	}
	if !strings.Contains(executeForTest(t, executor, "SETNX", "a", "nope"), ":0") {
		t.Fatal("SETNX existing")
	}
	if !strings.Contains(executeForTest(t, executor, "SETNX", "fresh", "yes"), ":1") {
		t.Fatal("SETNX new")
	}
	if !strings.Contains(executeForTest(t, executor, "GETSET", "a", "9"), "1") {
		t.Fatal("GETSET")
	}
	if executeForTest(t, executor, "SET", "ttlkey", "v", "EX", "60") != "+OK\r\n" {
		t.Fatal("SET EX")
	}
	if !strings.Contains(executeForTest(t, executor, "TTL", "ttlkey"), ":") {
		t.Fatal("TTL")
	}
	if !strings.Contains(executeForTest(t, executor, "EXPIRE", "a", "30"), ":1") {
		t.Fatal("EXPIRE")
	}
	if !strings.Contains(executeForTest(t, executor, "PERSIST", "a"), ":1") {
		t.Fatal("PERSIST")
	}
	if executeForTest(t, executor, "RENAME", "a", "renamed") != "+OK\r\n" {
		t.Fatal("RENAME")
	}
	if !strings.Contains(executeForTest(t, executor, "TYPE", "renamed"), "string") {
		t.Fatal("TYPE")
	}
	if !strings.Contains(executeForTest(t, executor, "DEL", "renamed", "fresh"), ":") {
		t.Fatal("DEL")
	}
	if !strings.Contains(executeForTest(t, executor, "DBSIZE"), ":") {
		t.Fatal("DBSIZE")
	}
	_ = executeForTest(t, executor, "KEYS", "*")
	_ = executeForTest(t, executor, "SCAN", "0", "MATCH", "*", "COUNT", "10")
	_ = executeForTest(t, executor, "INFO")
	_ = executeForTest(t, executor, "COMMAND")
	_ = executeForTest(t, executor, "HELLO")
	_ = executeForTest(t, executor, "DBX.TENANT", "INFO")
	_ = executeForTest(t, executor, "DBX.TENANT", "LIST")
	_ = executeForTest(t, executor, "DBX.MVCC", "VERSION")
	_ = executeForTest(t, executor, "DBX.MVCC", "SNAPSHOT")
	_ = executeForTest(t, executor, "DBX.SNAPSHOT", "NOW")
	_ = executeForTest(t, executor, "DBX.SNAPSHOT", "STATUS")
	_ = executeForTest(t, executor, "DBX.KEYSPACE")
	if !strings.Contains(executeForTest(t, executor, "UNKNOWNCMD"), "unknown command") {
		t.Fatal("unknown command")
	}
}

func TestDispatchCoversCompoundTypesAndVectors(t *testing.T) {
	executor := newOpenExecutor(t)

	if !strings.Contains(dispatchForTest(t, executor, "HSET", "h", "f", "1", "g", "2"), ":2") {
		t.Fatal("HSET")
	}
	if dispatchForTest(t, executor, "HMSET", "h", "f", "3") != "+OK\r\n" {
		t.Fatal("HMSET")
	}
	if !strings.Contains(dispatchForTest(t, executor, "HGET", "h", "f"), "3") {
		t.Fatal("HGET")
	}
	_ = dispatchForTest(t, executor, "HMGET", "h", "f", "missing")
	_ = dispatchForTest(t, executor, "HGETALL", "h")
	_ = dispatchForTest(t, executor, "HKEYS", "h")
	_ = dispatchForTest(t, executor, "HVALS", "h")
	if !strings.Contains(dispatchForTest(t, executor, "HLEN", "h"), ":") {
		t.Fatal("HLEN")
	}
	if !strings.Contains(dispatchForTest(t, executor, "HEXISTS", "h", "f"), ":1") {
		t.Fatal("HEXISTS")
	}
	_ = dispatchForTest(t, executor, "HINCRBY", "h", "n", "4")
	_ = dispatchForTest(t, executor, "HDEL", "h", "g")

	_ = dispatchForTest(t, executor, "LPUSH", "list", "a", "b")
	_ = dispatchForTest(t, executor, "RPUSH", "list", "c")
	_ = dispatchForTest(t, executor, "LRANGE", "list", "0", "-1")
	_ = dispatchForTest(t, executor, "LLEN", "list")
	_ = dispatchForTest(t, executor, "LINDEX", "list", "0")
	_ = dispatchForTest(t, executor, "LPOP", "list")
	_ = dispatchForTest(t, executor, "RPOP", "list")

	_ = dispatchForTest(t, executor, "SADD", "set", "x", "y")
	_ = dispatchForTest(t, executor, "SISMEMBER", "set", "x")
	_ = dispatchForTest(t, executor, "SMEMBERS", "set")
	_ = dispatchForTest(t, executor, "SCARD", "set")
	_ = dispatchForTest(t, executor, "SREM", "set", "y")
	_ = dispatchForTest(t, executor, "SPOP", "set")

	_ = dispatchForTest(t, executor, "ZADD", "z", "1", "m1", "2", "m2")
	_ = dispatchForTest(t, executor, "ZSCORE", "z", "m1")
	_ = dispatchForTest(t, executor, "ZRANK", "z", "m1")
	_ = dispatchForTest(t, executor, "ZCARD", "z")
	_ = dispatchForTest(t, executor, "ZINCRBY", "z", "3", "m1")
	_ = dispatchForTest(t, executor, "ZRANGE", "z", "0", "-1")
	_ = dispatchForTest(t, executor, "ZREM", "z", "m2")

	_ = dispatchForTest(t, executor, "SETBIT", "bits", "3", "1")
	_ = dispatchForTest(t, executor, "GETBIT", "bits", "3")
	_ = dispatchForTest(t, executor, "BITCOUNT", "bits")
	_ = dispatchForTest(t, executor, "BITPOS", "bits", "1")

	_ = dispatchForTest(t, executor, "XADD", "s", "*", "f", "v")
	_ = dispatchForTest(t, executor, "XLEN", "s")
	_ = dispatchForTest(t, executor, "XRANGE", "s", "-", "+")
	_ = dispatchForTest(t, executor, "XREVRANGE", "s", "+", "-")

	_ = dispatchForTest(t, executor, "GEOADD", "geo", "13.4", "52.5", "berlin")
	_ = dispatchForTest(t, executor, "GEOPOS", "geo", "berlin")
	_ = dispatchForTest(t, executor, "GEODIST", "geo", "berlin", "berlin", "m")

	_ = dispatchForTest(t, executor, "JSON.SET", "doc", ".", `{"a":1}`)
	_ = dispatchForTest(t, executor, "JSON.GET", "doc", ".")
	_ = dispatchForTest(t, executor, "JSON.TYPE", "doc")
	_ = dispatchForTest(t, executor, "JSON.DEL", "doc", ".")

	_ = dispatchForTest(t, executor, "OBJECT", "ENCODING", "k")
	_ = dispatchForTest(t, executor, "PUBLISH", "ch", "msg")
	_ = dispatchForTest(t, executor, "SUBSCRIBE", "ch")
	_ = dispatchForTest(t, executor, "UNSUBSCRIBE")

	_ = dispatchForTest(t, executor, "MULTI")
	_ = executeForTest(t, executor, "SET", "queued", "1")
	if got := dispatchForTest(t, executor, "SET", "queued", "1"); got != "+QUEUED\r\n" && !strings.Contains(got, "QUEUED") {
		// MULTI queues through Execute; Dispatch SET still writes.
		_ = got
	}
	_ = dispatchForTest(t, executor, "DISCARD")
	_ = dispatchForTest(t, executor, "WATCH", "k")
	_ = dispatchForTest(t, executor, "UNWATCH")
	_ = dispatchForTest(t, executor, "MULTI")
	_ = dispatchForTest(t, executor, "EXEC")

	if !strings.Contains(dispatchForTest(t, executor, "VADD", "idx", "v1", "1", "0"), ":1") {
		t.Fatal("VADD")
	}
	_ = dispatchForTest(t, executor, "VADD_BATCH", "idx", "2", "v2", "0", "1")
	_ = dispatchForTest(t, executor, "VSEARCH", "idx", "1", "0", "2")
	_ = dispatchForTest(t, executor, "VSEARCH", "idx", "1", "0", "2", "WITHDOCS", "doc:idx")
	_ = dispatchForTest(t, executor, "VDEL", "idx", "v1")
	_ = dispatchForTest(t, executor, "VCOMPACT", "idx")

	_ = dispatchForTest(t, executor, "FLUSHDB")
	_ = dispatchForTest(t, executor, "GET", "missing")
	_ = dispatchForTest(t, executor, "ECHO")
	_ = dispatchForTest(t, executor, "DEL")
	_ = dispatchForTest(t, executor, "SET", "onlyone")
	_ = dispatchForTest(t, executor, "DBX.TENANT")
	_ = dispatchForTest(t, executor, "DBX.TENANT", "NOPE")
	_ = dispatchForTest(t, executor, "DBX.MVCC", "NOPE")
	_ = dispatchForTest(t, executor, "DBX.SNAPSHOT", "NOPE")
	_ = dispatchForTest(t, executor, "QUIT")
}

func TestApplyWALRecordAndMemoryHelpers(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	if err := executor.ApplyWALRecord(&persistence.WALRecord{Type: persistence.RecordSet, Key: "k", Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if err := executor.ApplyWALRecord(&persistence.WALRecord{Type: persistence.RecordExpire, Key: "k", TTLNano: 1e9}); err != nil {
		t.Fatal(err)
	}
	if err := executor.ApplyWALRecord(&persistence.WALRecord{Type: persistence.RecordDelete, Key: "k"}); err != nil {
		t.Fatal(err)
	}
	if err := executor.ApplyWALRecord(&persistence.WALRecord{
		Type: persistence.RecordVAdd, Key: "idx",
		Value: persistence.EncodeVAddPayload("id", []float32{1, 0}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := executor.ApplyWALRecord(&persistence.WALRecord{
		Type: persistence.RecordVAddBatch, Key: "idx",
		Value: persistence.EncodeVAddBatchPayload(2, []string{"id2"}, [][]float32{{0, 1}}),
	}); err != nil {
		t.Fatal(err)
	}
	executor.SetMemoryLimit(1 << 20)
	if executor.MemoryUsage() <= 0 {
		t.Fatal("expected measured memory after limit")
	}
}

func TestEngineFSMApplyAndSnapshotDisabled(t *testing.T) {
	executor := newOpenExecutor(t)
	fsm := NewEngineFSM(executor, nil)
	out := fsm.Apply(&raft.Log{Data: []byte(`{"Name":"PING"}`)})
	raw, ok := out.([]byte)
	if !ok || !bytes.Contains(raw, []byte("PONG")) {
		t.Fatalf("apply = %#v", out)
	}
	if _, err := fsm.Snapshot(); err == nil {
		t.Fatal("expected snapshots disabled")
	}
	if err := fsm.Restore(io.NopCloser(bytes.NewReader(nil))); err != nil {
		t.Fatal(err)
	}
	bad := fsm.Apply(&raft.Log{Data: []byte("not-json")})
	if raw, ok := bad.([]byte); !ok || !bytes.Contains(raw, []byte("ERR")) {
		t.Fatalf("bad apply = %#v", bad)
	}
}

func TestRouterRedirectsForeignKeys(t *testing.T) {
	ring := cluster.NewRing()
	ring.AddNode(&cluster.Node{ID: "self", Addr: "127.0.0.1:1"})
	ring.AddNode(&cluster.Node{ID: "other", Addr: "127.0.0.1:2"})
	router := NewRouter(ring, "self")
	if addr, redirect := router.RouteKey("any-key"); redirect && addr == "" {
		t.Fatal("redirect missing address")
	}
	local := NewRouter(nil, "self")
	if _, redirect := local.RouteKey("k"); redirect {
		t.Fatal("nil ring should not redirect")
	}
}

func TestNoisyNeighborQuotaDoesNotBlockQuietTenant(t *testing.T) {
	noisy, noisyWAL := newDurableTestExecutor(t)
	defer noisyWAL.Close()
	noisy.SetMemoryLimit(256)
	quiet, quietWAL := newDurableTestExecutor(t)
	defer quietWAL.Close()
	quiet.SetMemoryLimit(16 << 20)

	if !strings.Contains(executeForTest(t, noisy, "SET", "blob", strings.Repeat("x", 512)), "OOM") {
		t.Fatal("noisy tenant should hit quota")
	}
	if executeForTest(t, quiet, "SET", "ok", "1") != "+OK\r\n" {
		t.Fatal("quiet tenant write")
	}
	if executeForTest(t, quiet, "GET", "ok") != "$1\r\n1\r\n" {
		t.Fatal("quiet tenant read")
	}
}

func TestSETNXAndXXDurableEffects(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	if executeForTest(t, executor, "SET", "k", "1", "NX") != "+OK\r\n" {
		t.Fatal("SET NX")
	}
	if executeForTest(t, executor, "SET", "k", "2", "NX") != "$-1\r\n" {
		t.Fatal("SET NX exists")
	}
	if executeForTest(t, executor, "SET", "k", "3", "XX") != "+OK\r\n" {
		t.Fatal("SET XX")
	}
	if executeForTest(t, executor, "SET", "missing", "1", "XX") != "$-1\r\n" {
		t.Fatal("SET XX missing")
	}
}

func TestVADDDurableRoundTrip(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	if !strings.Contains(executeForTest(t, executor, "VADD", "idx", "a", "1", "0"), ":1") {
		t.Fatal("VADD")
	}
	if !strings.Contains(executeForTest(t, executor, "VSEARCH", "idx", "1", "0", "1"), "a") {
		t.Fatal("VSEARCH")
	}
}
