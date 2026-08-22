package security

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditEvent represents an immutable audit log entry.
type AuditEvent struct {
	Timestamp time.Time `json:"ts"`
	ClientID  uint64    `json:"client_id"`
	UserName  string    `json:"user"`
	TenantID  string    `json:"tenant,omitempty"`
	Command   string    `json:"cmd"`
	Key       string    `json:"key,omitempty"`
	Result    string    `json:"result"` // "ok" or "denied"
	Reason    string    `json:"reason,omitempty"`
	RemoteAddr string   `json:"remote_addr,omitempty"`
}

// AuditGuard writes immutable audit log entries.
type AuditGuard struct {
	mu      sync.Mutex
	file    *os.File
	enabled bool
	buf     []AuditEvent
	bufSize int
}

// NewAuditGuard creates an audit guard writing to path.
func NewAuditGuard(path string, enabled bool) (*AuditGuard, error) {
	if !enabled {
		return &AuditGuard{enabled: false}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return &AuditGuard{file: f, enabled: true, bufSize: 100}, nil
}

// Log records an audit event.
func (g *AuditGuard) Log(event AuditEvent) {
	if !g.enabled {
		return
	}
	event.Timestamp = time.Now()
	g.mu.Lock()
	g.buf = append(g.buf, event)
	if len(g.buf) >= g.bufSize {
		g.flush()
	}
	g.mu.Unlock()
}

// Flush writes buffered events to disk.
func (g *AuditGuard) Flush() {
	if !g.enabled {
		return
	}
	g.mu.Lock()
	g.flush()
	g.mu.Unlock()
}

func (g *AuditGuard) flush() {
	for _, e := range g.buf {
		data, _ := json.Marshal(e)
		data = append(data, '\n')
		g.file.Write(data)
	}
	g.buf = g.buf[:0]
}

// Close flushes and closes the audit log.
func (g *AuditGuard) Close() {
	if g.file != nil {
		g.Flush()
		g.file.Close()
	}
}
