package orchestrator

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/isolation"
)

func TestBackendPoolReusesWarmDial(t *testing.T) {
	if !isolation.UnixAvailable() {
		t.Skip("unix sockets are not available on this OS")
	}
	dir := t.TempDir()
	sock := isolation.RESPSocket(dir)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}(conn)
		}
	}()

	tenant := &Tenant{ID: "acme", DataDir: dir}
	pool := newBackendPool()
	conn, _, err := pool.Acquire(tenant)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	pool.Prefetch(tenant)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pool.mu.Lock()
		n := len(pool.idle[tenant.ID])
		pool.mu.Unlock()
		if n > 0 {
			warm, _, err := pool.Acquire(tenant)
			if err != nil {
				t.Fatal(err)
			}
			_ = warm.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("prefetch did not warm a worker dial")
}

func TestTenantRESPAddrFallsBackToTCP(t *testing.T) {
	tenant := &Tenant{ID: "x", RESPPort: 6401}
	network, addr := tenantRESPAddr(tenant)
	if network != "tcp" || addr != "127.0.0.1:6401" {
		t.Fatalf("%s %s", network, addr)
	}
}
