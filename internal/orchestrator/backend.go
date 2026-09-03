package orchestrator

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/isolation"
)

const (
	backendDialTimeout = 5 * time.Second
	backendIdleTTL     = 30 * time.Second
	backendMaxIdle     = 8
)

// tenantRESPAddr prefers the tenant Unix socket. TCP is only a fallback for
// tests and Windows, where a socket was never bound.
func tenantRESPAddr(t *Tenant) (network, addr string) {
	if t == nil {
		return "tcp", "127.0.0.1:0"
	}
	if t.DataDir != "" {
		sock := isolation.RESPSocket(t.DataDir)
		if _, err := os.Stat(sock); err == nil {
			return "unix", sock
		}
	}
	return "tcp", fmt.Sprintf("127.0.0.1:%d", t.RESPPort)
}

func waitTenantSockets(dataDir string, timeout time.Duration) error {
	if dataDir == "" {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, respErr := os.Stat(isolation.RESPSocket(dataDir))
		_, httpErr := os.Stat(isolation.HTTPSocket(dataDir))
		if respErr == nil && httpErr == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("tenant sockets not ready: %s", isolation.RESPSocket(dataDir))
}

type pooledBackend struct {
	conn   net.Conn
	reader *bufio.Reader
	idleAt time.Time
}

// backendPool holds unused worker dials so the next AUTH does not wait on
// DialTimeout. A connection that has already carried a client session is
// never reused: CloseWrite ends the worker's command loop.
type backendPool struct {
	mu   sync.Mutex
	idle map[string][]*pooledBackend
}

func newBackendPool() *backendPool {
	return &backendPool{idle: make(map[string][]*pooledBackend)}
}

func (p *backendPool) Acquire(t *Tenant) (net.Conn, *bufio.Reader, error) {
	if p != nil && t != nil {
		p.mu.Lock()
		list := p.idle[t.ID]
		now := time.Now()
		for len(list) > 0 {
			item := list[len(list)-1]
			list = list[:len(list)-1]
			if now.Sub(item.idleAt) > backendIdleTTL {
				_ = item.conn.Close()
				continue
			}
			p.idle[t.ID] = list
			p.mu.Unlock()
			return item.conn, item.reader, nil
		}
		p.idle[t.ID] = list
		p.mu.Unlock()
	}
	return dialTenantBackend(t)
}

func dialTenantBackend(t *Tenant) (net.Conn, *bufio.Reader, error) {
	network, addr := tenantRESPAddr(t)
	conn, err := net.DialTimeout(network, addr, backendDialTimeout)
	if err != nil {
		return nil, nil, err
	}
	return conn, bufio.NewReaderSize(conn, 64*1024), nil
}

// Prefetch dials one unused worker connection so the next client skips connect.
func (p *backendPool) Prefetch(t *Tenant) {
	if p == nil || t == nil {
		return
	}
	go func() {
		p.mu.Lock()
		if len(p.idle[t.ID]) >= backendMaxIdle {
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		network, addr := tenantRESPAddr(t)
		conn, err := net.DialTimeout(network, addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		reader := bufio.NewReaderSize(conn, 64*1024)
		p.mu.Lock()
		if len(p.idle[t.ID]) >= backendMaxIdle {
			p.mu.Unlock()
			_ = conn.Close()
			return
		}
		p.idle[t.ID] = append(p.idle[t.ID], &pooledBackend{conn: conn, reader: reader, idleAt: time.Now()})
		p.mu.Unlock()
	}()
}

func (p *backendPool) DropTenant(tenantID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	list := p.idle[tenantID]
	delete(p.idle, tenantID)
	p.mu.Unlock()
	for _, item := range list {
		_ = item.conn.Close()
	}
}
