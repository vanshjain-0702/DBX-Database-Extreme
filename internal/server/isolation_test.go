package server

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/security"
)

func sealedTestConfig(t *testing.T, dir, role, listen, primaryAddr string) *config.Config {
	t.Helper()
	cfg := config.Defaults()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	cfg.Server.HTTPPort = freePort(t)
	cfg.Server.GRPCPort = 0
	cfg.Server.Socket = isolation.RESPSocket(dir)
	cfg.Server.HTTPSocket = isolation.HTTPSocket(dir)
	cfg.Security.RateLimit.Enabled = false
	cfg.Persistence.Enabled = true
	cfg.Persistence.DataDir = dir
	cfg.Persistence.WALDir = filepath.Join(dir, "wal")
	cfg.Persistence.SnapshotDir = filepath.Join(dir, "snapshots")
	cfg.Persistence.BackupDir = filepath.Join(dir, "backups")
	cfg.Persistence.WALSync = "everysec"
	cfg.Replication.Role = role
	cfg.Replication.ListenAddr = listen
	cfg.Replication.PrimaryAddr = primaryAddr
	cfg.Replication.RaftEnabled = false
	cfg.Observability.Logging.Level = "error"
	if err := config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func testDEK(seed byte) *security.Encryptor {
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed + byte(i)
	}
	enc, _ := security.NewEncryptor(key)
	return enc
}

func waitSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", path, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for socket %s", path)
}

func socketCommand(t *testing.T, path, password string, args ...string) string {
	t.Helper()
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	writer := protocol.NewWriter(conn)
	_ = writer.WriteArray(3)
	_ = writer.WriteBulkString([]byte("AUTH"))
	_ = writer.WriteBulkString([]byte("default"))
	_ = writer.WriteBulkString([]byte(password))
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	_ = writer.WriteArray(len(args))
	for _, arg := range args {
		_ = writer.WriteBulkString([]byte(arg))
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(line, "$") && !strings.HasPrefix(line, "$-1") {
		body, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		return line + body
	}
	return line
}

// The isolated profile puts RESP, HTTP, and the replication side channel on
// Unix sockets and encrypts durable files. Replication has to keep working
// across that change: a primary and replica are separate sandboxed processes
// with different DEKs, so WAL frames must cross the socket as plaintext and be
// re-sealed by whichever engine receives them.
func TestSealedPrimaryReplicaReplicationOverUnixSockets(t *testing.T) {
	t.Setenv("DBX_DEFAULT_PASSWORD", "sealed-replica-secret")
	password := "sealed-replica-secret"

	primaryDir := t.TempDir()
	replicaDir := t.TempDir()
	replSocket := isolation.ReplSocket(primaryDir)

	primaryCfg := sealedTestConfig(t, primaryDir, "primary", replSocket, "")
	replicaCfg := sealedTestConfig(t, replicaDir, "replica", "", replSocket)

	primary, err := NewInstance(primaryCfg)
	if err != nil {
		t.Fatal(err)
	}
	primary.SetAtRest(testDEK(1))
	if err := primary.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer primary.Stop()
	waitSocket(t, primaryCfg.Server.Socket)

	replica, err := NewInstance(replicaCfg)
	if err != nil {
		t.Fatal(err)
	}
	replica.SetAtRest(testDEK(90))
	if err := replica.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer replica.Stop()
	waitSocket(t, replicaCfg.Server.Socket)

	if got := socketCommand(t, primaryCfg.Server.Socket, password, "SET", "k", "v1"); got != "+OK\r\n" {
		t.Fatalf("primary SET over unix socket = %q", got)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		got := socketCommand(t, replicaCfg.Server.Socket, password, "GET", "k")
		if got == "$2\r\nv1\r\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replica never received the write over the unix side channel: GET = %q", got)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The replica sealed the replayed record under its own key, not the primary's.
	replicaWAL, err := os.ReadFile(filepath.Join(replicaDir, "wal", "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(replicaWAL, []byte("v1")) {
		t.Fatal("replica stored the replicated value in plaintext")
	}
}

// Encryption must not weaken the durability contract for strings either: a
// restart has to replay the sealed WAL.
func TestSealedEngineRecoversAfterRestart(t *testing.T) {
	t.Setenv("DBX_DEFAULT_PASSWORD", "sealed-restart-secret")
	password := "sealed-restart-secret"
	dir := t.TempDir()

	cfg := sealedTestConfig(t, dir, "", "", "")
	first, err := NewInstance(cfg)
	if err != nil {
		t.Fatal(err)
	}
	first.SetAtRest(testDEK(3))
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitSocket(t, cfg.Server.Socket)
	if got := socketCommand(t, cfg.Server.Socket, password, "SET", "survives", "yes"); got != "+OK\r\n" {
		t.Fatalf("SET = %q", got)
	}
	first.Stop()

	second := sealedTestConfig(t, dir, "", "", "")
	restarted, err := NewInstance(second)
	if err != nil {
		t.Fatal(err)
	}
	restarted.SetAtRest(testDEK(3))
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop()
	waitSocket(t, second.Server.Socket)
	if got := socketCommand(t, second.Server.Socket, password, "GET", "survives"); got != "$3\r\nyes\r\n" {
		t.Fatalf("sealed value did not survive restart: GET = %q", got)
	}
}

// A wrong key must fail closed rather than silently serve an empty tenant.
func TestSealedEngineRefusesWrongKey(t *testing.T) {
	t.Setenv("DBX_DEFAULT_PASSWORD", "sealed-wrongkey-secret")
	dir := t.TempDir()
	cfg := sealedTestConfig(t, dir, "", "", "")
	first, err := NewInstance(cfg)
	if err != nil {
		t.Fatal(err)
	}
	first.SetAtRest(testDEK(5))
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitSocket(t, cfg.Server.Socket)
	socketCommand(t, cfg.Server.Socket, "sealed-wrongkey-secret", "SET", "secret", "value")
	first.Stop()

	second := sealedTestConfig(t, dir, "", "", "")
	wrong, err := NewInstance(second)
	if err != nil {
		t.Fatal(err)
	}
	wrong.SetAtRest(testDEK(200))
	if err := wrong.Start(context.Background()); err == nil {
		wrong.Stop()
		t.Fatal("engine started with the wrong DEK instead of failing closed")
	}
}
