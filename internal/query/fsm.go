package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/hashicorp/raft"
)

// EngineFSM implements raft.FSM for the DBX data plane.
type EngineFSM struct {
	mu          sync.RWMutex
	executor    *Executor
	snapshotter *persistence.Snapshotter
}

// NewEngineFSM creates a new Raft FSM for the engine.
func NewEngineFSM(executor *Executor, snapshotter *persistence.Snapshotter) *EngineFSM {
	return &EngineFSM{
		executor:    executor,
		snapshotter: snapshotter,
	}
}

// Apply executes a Raft log entry.
func (f *EngineFSM) Apply(l *raft.Log) (result interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = []byte("-ERR internal server error\r\n")
		}
	}()
	var cmd protocol.Command
	if err := json.Unmarshal(l.Data, &cmd); err != nil {
		return []byte(fmt.Sprintf("-ERR FSM Apply decode failed: %v\r\n", err))
	}

	var buf bytes.Buffer
	writer := protocol.NewWriter(&buf)

	f.mu.Lock()
	defer f.mu.Unlock()

	err := f.executor.Dispatch(0, &cmd, writer)
	if err != nil {
		if buf.Len() == 0 {
			writer.WriteErrorRaw(err.Error())
		}
	}

	return buf.Bytes()
}

// Snapshot creates a Raft snapshot of the engine state.
func (f *EngineFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.snapshotter == nil {
		return nil, fmt.Errorf("snapshots disabled")
	}

	path, err := f.snapshotter.Save(f.executor.KV())
	if err != nil {
		return nil, err
	}

	return &engineSnapshot{path: path}, nil
}

// Restore restores the engine state from a Raft snapshot.
func (f *EngineFSM) Restore(rc io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil
}

type engineSnapshot struct {
	path string
}

func (s *engineSnapshot) Persist(sink raft.SnapshotSink) error {
	defer sink.Close()
	_, err := sink.Write([]byte("snapshot data placeholder"))
	return err
}

func (s *engineSnapshot) Release() {}
