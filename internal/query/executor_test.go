package query

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/events"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/transaction"
)

func newDurableTestExecutor(t *testing.T) (*Executor, *persistence.WAL) {
	t.Helper()
	kv := engine.New(8)
	wal, err := persistence.OpenWAL(filepath.Join(t.TempDir(), "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(
		kv, engine.NewVectorStore(kv, t.TempDir(), 100),
		transaction.NewMultiManager(), transaction.NewWatchSet(), transaction.NewMVCCStore(8),
		events.NewPubSub(10, 10), &observability.Metrics{}, wal,
	)
	executor.SetMemoryLimit(16 << 20)
	t.Cleanup(func() { executor.vec.CloseAll() })
	return executor, wal
}

func executeForTest(t *testing.T, executor *Executor, name string, args ...string) string {
	t.Helper()
	command := &protocol.Command{Name: name, Args: make([][]byte, len(args))}
	for i := range args {
		command.Args[i] = []byte(args[i])
	}
	var output bytes.Buffer
	writer := protocol.NewWriter(&output)
	if err := executor.Execute(1, command, writer); err != nil {
		t.Fatal(err)
	}
	_ = writer.Flush()
	return output.String()
}

func TestDurableProfileRejectsUnsupportedMutation(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	response := executeForTest(t, executor, "HSET", "hash", "field", "value")
	if !strings.Contains(response, "not supported by the durable DBX v1 profile") {
		t.Fatalf("response = %q", response)
	}
	if executor.KV().Exists("hash") {
		t.Fatal("unsupported mutation reached the engine")
	}
}

func TestMSETUsesSingleStateImageTransaction(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	response := executeForTest(t, executor, "MSET", "a", "1", "b", "2")
	if response != "+OK\r\n" {
		t.Fatalf("response = %q", response)
	}
	records, err := wal.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(records[0].Effects) != 2 {
		t.Fatalf("WAL records = %#v", records)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteIsolatesPanic(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	executor.testHook = func() { panic("isolated") }
	command := &protocol.Command{Name: "GET", Args: [][]byte{[]byte("k")}}
	var output bytes.Buffer
	writer := protocol.NewWriter(&output)
	if err := executor.Execute(1, command, writer); err != nil {
		t.Fatal(err)
	}
	_ = writer.Flush()
	if !strings.Contains(output.String(), "internal server error") {
		t.Fatalf("response = %q", output.String())
	}
	executor.testHook = nil
	if executeForTest(t, executor, "SET", "k", "1") != "+OK\r\n" {
		t.Fatal("executor did not continue after isolated panic")
	}
}

func TestReplicaRejectsWritesAndServesReads(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	if executeForTest(t, executor, "SET", "k", "1") != "+OK\r\n" {
		t.Fatal("primary write failed")
	}
	executor.SetReadOnly(true)
	if got := executeForTest(t, executor, "SET", "k", "2"); !strings.Contains(got, "READONLY") {
		t.Fatalf("replica write = %q", got)
	}
	if got := executeForTest(t, executor, "GET", "k"); got != "$1\r\n1\r\n" {
		t.Fatalf("replica read = %q", got)
	}
	if err := executor.ApplyWALRecord(&persistence.WALRecord{
		Effects: []persistence.WALEffect{
			{Type: persistence.RecordSet, Key: "a", Value: []byte("1")},
			{Type: persistence.RecordSet, Key: "b", Value: []byte("2")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := executeForTest(t, executor, "GET", "b"); got != "$1\r\n2\r\n" {
		t.Fatalf("replicated mset = %q", got)
	}
}

func TestTenantQuotaRejectsBeforeWAL(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	executor.SetMemoryLimit(128)
	response := executeForTest(t, executor, "SET", "large", strings.Repeat("x", 256))
	if !strings.Contains(response, "OOM tenant quota exceeded") {
		t.Fatalf("response = %q", response)
	}
	records, err := wal.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 || executor.KV().Exists("large") {
		t.Fatal("rejected write changed WAL or engine")
	}
}

func TestCheckpointFlushesVectorsAndCURRENT(t *testing.T) {
	dir := t.TempDir()
	kv := engine.New(8)
	wal, err := persistence.OpenWAL(filepath.Join(dir, "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	vec := engine.NewVectorStore(kv, dir, 0)
	executor := NewExecutor(
		kv, vec,
		transaction.NewMultiManager(), transaction.NewWatchSet(), transaction.NewMVCCStore(8),
		events.NewPubSub(10, 10), &observability.Metrics{}, wal,
	)
	executor.SetMemoryLimit(16 << 20)
	t.Cleanup(func() { vec.CloseAll() })

	if got := executeForTest(t, executor, "VADD", "mem", "a", "1", "0"); got != ":1\r\n" {
		t.Fatalf("vadd = %q", got)
	}
	if got := executeForTest(t, executor, "VADD", "mem", "b", "0", "1"); got != ":1\r\n" {
		t.Fatalf("vadd b = %q", got)
	}

	snap := persistence.NewSnapshotter(filepath.Join(dir, "snapshots"))
	path, err := executor.Checkpoint(snap)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Latest() != path {
		t.Fatalf("CURRENT = %s, saved = %s", snap.Latest(), path)
	}
	hdr, err := snap.LoadWithHeader(engine.New(8), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdr.VectorSeals) == 0 {
		t.Fatal("checkpoint missing vector seals")
	}

	vec.CloseAll()
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := persistence.OpenWAL(filepath.Join(dir, "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := engine.New(8)
	restoredVec := engine.NewVectorStore(restored, dir, 0)
	defer restoredVec.CloseAll()
	if err := persistence.NewRecovery(reopened, snap).Recover(restored, restoredVec); err != nil {
		t.Fatal(err)
	}
	results, err := restoredVec.VSearch("mem", []float32{1, 0}, 1, nil)
	if err != nil || len(results) == 0 || results[0].ID != "a" {
		t.Fatalf("search after checkpoint recover = %#v, %v", results, err)
	}
}
