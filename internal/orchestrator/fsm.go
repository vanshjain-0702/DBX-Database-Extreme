package orchestrator

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

// OrchestratorFSM implements the raft.FSM interface to replicate the Control Plane state.
// The state consists of the multi-tenant mapping (tenants.json equivalent).
type OrchestratorFSM struct {
	mu      sync.RWMutex
	manager *Manager
}

type fsmUpdateCommand struct {
	Action    string    `json:"action"` // "provision" | "deprovision" | "promote"
	Tenant    *Tenant   `json:"tenant"`
	Members   []*Tenant `json:"members,omitempty"`
	Purge     bool      `json:"purge,omitempty"`
	ReplicaID string    `json:"replica_id,omitempty"`
}

func NewOrchestratorFSM(manager *Manager) *OrchestratorFSM {
	return &OrchestratorFSM{
		manager: manager,
	}
}

// Apply is called by Raft when a log entry is committed.
func (f *OrchestratorFSM) Apply(l *raft.Log) (result interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = fmt.Errorf("internal error: %v", recovered)
		}
	}()
	var cmd fsmUpdateCommand
	if err := json.Unmarshal(l.Data, &cmd); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch cmd.Action {
	case "provision":
		f.manager.mu.Lock()
		f.manager.tenants[cmd.Tenant.ID] = cmd.Tenant
		for _, member := range cmd.Members {
			if member != nil {
				f.manager.tenants[member.ID] = member
			}
		}
		_, alreadyRunning := f.manager.instances[cmd.Tenant.ID]
		f.manager.mu.Unlock()

		if !alreadyRunning {
			members := append([]*Tenant(nil), cmd.Members...)
			go f.manager.startReplicaSet(cmd.Tenant, members)
		}
	case "deprovision":
		if cmd.Tenant == nil {
			return nil
		}
		for _, member := range cmd.Members {
			if member == nil {
				continue
			}
			if err := f.manager.removeTenant(member, cmd.Purge); err != nil {
				return err
			}
		}
		if err := f.manager.removeTenant(cmd.Tenant, cmd.Purge); err != nil {
			return err
		}
	case "promote":
		if err := f.manager.promoteLocal(cmd.ReplicaID); err != nil {
			return err
		}
	}
	return nil
}

// Snapshot returns an FSMSnapshot representing the current state.
func (f *OrchestratorFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.manager.mu.RLock()
	defer f.manager.mu.RUnlock()

	data, err := json.Marshal(f.manager.tenants)
	if err != nil {
		return nil, err
	}
	return &fsmSnapshot{state: data}, nil
}

// Restore applies a snapshot to the FSM.
func (f *OrchestratorFSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.manager.mu.Lock()

	var state map[string]*Tenant
	if err := json.Unmarshal(data, &state); err != nil {
		f.manager.mu.Unlock()
		return err
	}
	f.manager.tenants = state

	// Start any restored tenants that aren't running
	for _, t := range f.manager.tenants {
		if _, alreadyRunning := f.manager.instances[t.ID]; !alreadyRunning {
			go f.manager.StartTenant(t)
		}
	}
	f.manager.mu.Unlock()
	return nil
}

type fsmSnapshot struct {
	state []byte
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	_, err := sink.Write(s.state)
	if err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}
