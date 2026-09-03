package orchestrator

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/isolation"
	"github.com/hashicorp/raft"
)

// restartRecord tracks restart attempts for a single tenant.
type restartRecord struct {
	attempts    int
	windowStart time.Time
}

// Sentinel monitors the health of Data Plane tenants and coordinates failover.
type Sentinel struct {
	manager  *Manager
	raftNode *RaftNode
	done     chan struct{}

	restartMu sync.Mutex
	restarts  map[string]*restartRecord // tenantID -> restart tracking
}

const (
	maxRestartsPerWindow = 3
	restartWindow        = 5 * time.Minute
	healthCheckInterval  = 3 * time.Second
	healthDialTimeout    = 1 * time.Second
)

func NewSentinel(manager *Manager, raftNode *RaftNode) *Sentinel {
	return &Sentinel{
		manager:  manager,
		raftNode: raftNode,
		done:     make(chan struct{}),
		restarts: make(map[string]*restartRecord),
	}
}

func (s *Sentinel) Start() {
	go func() {
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				s.checkHealth()
			}
		}
	}()
}

func (s *Sentinel) Stop() {
	close(s.done)
}

// canRestart checks whether a tenant is allowed to restart (rate-limited).
func (s *Sentinel) canRestart(tenantID string) bool {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()

	rec, exists := s.restarts[tenantID]
	now := time.Now()

	if !exists {
		s.restarts[tenantID] = &restartRecord{attempts: 1, windowStart: now}
		return true
	}

	// Reset window if expired
	if now.Sub(rec.windowStart) > restartWindow {
		rec.attempts = 1
		rec.windowStart = now
		return true
	}

	if rec.attempts >= maxRestartsPerWindow {
		return false
	}

	rec.attempts++
	return true
}

func (s *Sentinel) checkHealth() {
	// Only the Raft Leader should monitor and initiate failovers.
	if s.raftNode != nil && s.raftNode.Raft.State() != raft.Leader {
		return
	}

	tenants := s.manager.ListTenants()
	for _, t := range tenants {
		if t.Hibernated {
			continue
		}
		addr := fmt.Sprintf("127.0.0.1:%d", t.RESPPort)
		network := "tcp"
		if t.DataDir != "" {
			sock := isolation.RESPSocket(t.DataDir)
			if _, err := os.Stat(sock); err == nil {
				network, addr = "unix", sock
			}
		}
		conn, err := net.DialTimeout(network, addr, healthDialTimeout)
		if err != nil {
			fmt.Printf("[Sentinel] WARN: Tenant %s (%s) is unreachable.\n", t.ID, addr)

			if s.canRestart(t.ID) {
				fmt.Printf("[Sentinel] AUTO-RESTART: Attempting to restart tenant %s...\n", t.ID)
				go func(tenant *Tenant) {
					if restartErr := s.manager.StartTenant(tenant); restartErr != nil {
						fmt.Printf("[Sentinel] ERROR: Failed to restart tenant %s: %v\n", tenant.ID, restartErr)
					} else {
						fmt.Printf("[Sentinel] OK: Tenant %s restarted successfully.\n", tenant.ID)
					}
				}(t)
			} else {
				fmt.Printf("[Sentinel] CIRCUIT-BREAK: Tenant %s exceeded max restarts (%d/%dm). Manual intervention required.\n",
					t.ID, maxRestartsPerWindow, int(restartWindow.Minutes()))
			}
		} else {
			conn.Close()
			// Reset restart counter on successful health check
			s.restartMu.Lock()
			delete(s.restarts, t.ID)
			s.restartMu.Unlock()
		}
	}
}
