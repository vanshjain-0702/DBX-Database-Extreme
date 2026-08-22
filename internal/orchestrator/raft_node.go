package orchestrator

import (
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

// RaftNode encapsulates the Raft consensus for the Orchestrator.
type RaftNode struct {
	Raft     *raft.Raft
	FSM      *OrchestratorFSM
	HasState bool
}

// NewRaftNode initializes a Raft node for the given manager.
func NewRaftNode(nodeID, bindAddr, raftDir string, manager *Manager) (*RaftNode, error) {
	if err := os.MkdirAll(raftDir, 0700); err != nil {
		return nil, err
	}

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	// Network transport
	addr, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		return nil, err
	}
	transport, err := raft.NewTCPTransport(bindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Snapshots
	snapshots, err := raft.NewFileSnapshotStore(raftDir, 2, os.Stderr)
	if err != nil {
		return nil, err
	}

	// BoltDB log store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft-log.bolt"))
	if err != nil {
		return nil, err
	}

	// BoltDB stable store
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft-stable.bolt"))
	if err != nil {
		return nil, err
	}

	fsm := NewOrchestratorFSM(manager)

	hasState, err := raft.HasExistingState(logStore, stableStore, snapshots)
	if err != nil {
		return nil, err
	}

	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return nil, err
	}

	return &RaftNode{
		Raft:     r,
		FSM:      fsm,
		HasState: hasState,
	}, nil
}

// Bootstrap starts a single-node cluster if needed.
func (rn *RaftNode) Bootstrap(nodeID, bindAddr string) error {
	if rn.HasState {
		return nil // skip bootstrap, state already exists
	}
	configuration := raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      raft.ServerID(nodeID),
				Address: raft.ServerAddress(bindAddr),
			},
		},
	}
	future := rn.Raft.BootstrapCluster(configuration)
	return future.Error()
}

// Join adds a new node to the cluster.
func (rn *RaftNode) Join(nodeID, bindAddr string) error {
	future := rn.Raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(bindAddr), 0, 0)
	return future.Error()
}
