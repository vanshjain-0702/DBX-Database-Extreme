package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/protocol"
)

func TestPrimaryReplicaLiveReplication(t *testing.T) {
	t.Setenv("DBX_DEFAULT_PASSWORD", "replica-test-secret")
	primaryRESP := freePort(t)
	primaryHTTP := freePort(t)
	replicaRESP := freePort(t)
	replicaHTTP := freePort(t)

	primaryCfg := replicaTestConfig(t, primaryRESP, primaryHTTP, "primary", "127.0.0.1:0", "")
	primary, err := NewInstance(primaryCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := primary.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer primary.Stop()

	listen := primary.ReplicationAddr()
	if listen == "" {
		t.Fatal("primary replication listener was not bound")
	}

	replicaCfg := replicaTestConfig(t, replicaRESP, replicaHTTP, "replica", "", listen)
	replica, err := NewInstance(replicaCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer replica.Stop()

	waitTCP(t, fmt.Sprintf("127.0.0.1:%d", primaryRESP))
	waitTCP(t, fmt.Sprintf("127.0.0.1:%d", replicaRESP))
	waitReplicaConnected(t, primary)

	password := "replica-test-secret"
	if got := respCommand(t, primaryRESP, password, "SET", "k", "v1"); got != "+OK\r\n" {
		t.Fatalf("primary SET = %q", got)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := respCommand(t, replicaRESP, password, "GET", "k")
		if got == "$2\r\nv1\r\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replica GET = %q", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := respCommand(t, replicaRESP, password, "SET", "k", "nope"); !strings.Contains(got, "READONLY") {
		t.Fatalf("replica SET = %q", got)
	}
}

func replicaTestConfig(t *testing.T, respPort, httpPort int, role, listen, primaryAddr string) *config.Config {
	t.Helper()
	cfg := config.Defaults()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = respPort
	cfg.Server.HTTPPort = httpPort
	cfg.Server.GRPCPort = 0
	cfg.Security.RateLimit.Enabled = false
	cfg.Persistence.Enabled = true
	dir := t.TempDir()
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

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func waitTCP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", addr)
}

func waitReplicaConnected(t *testing.T, primary *Instance) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if primary.ReplicaCount() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for replica to register with primary")
}

func TestSkipBuiltinUserRejectsDefaultAUTH(t *testing.T) {
	t.Setenv("DBX_DEFAULT_PASSWORD", "should-not-work-on-orchestrated-tenants")
	respPort := freePort(t)
	httpPort := freePort(t)
	cfg := replicaTestConfig(t, respPort, httpPort, "", "", "")
	inst, err := NewInstance(cfg)
	if err != nil {
		t.Fatal(err)
	}
	inst.SkipBuiltinUser()
	if err := inst.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer inst.Stop()
	waitTCP(t, fmt.Sprintf("127.0.0.1:%d", respPort))

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", respPort), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	writer := protocol.NewWriter(conn)
	_ = writer.WriteArray(3)
	_ = writer.WriteBulkString([]byte("AUTH"))
	_ = writer.WriteBulkString([]byte("default"))
	_ = writer.WriteBulkString([]byte("should-not-work-on-orchestrated-tenants"))
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(line, "+OK") {
		t.Fatalf("default superuser authenticated on orchestrated instance: %q", line)
	}
}

func respCommand(t *testing.T, port int, password string, args ...string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
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
	authLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(authLine, "+OK") {
		t.Fatalf("AUTH = %q", authLine)
	}
	if err := writer.WriteArray(len(args)); err != nil {
		t.Fatal(err)
	}
	for _, arg := range args {
		_ = writer.WriteBulkString([]byte(arg))
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	return readRESP(t, reader)
}

func readRESP(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	prefix, err := reader.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	out := string(prefix) + line
	if prefix != '$' {
		return out
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	if n < 0 {
		return out
	}
	payload := make([]byte, n+2)
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatal(err)
	}
	return out + string(payload)
}
