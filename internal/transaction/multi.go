package transaction

import (
	"fmt"
	"sync"

	"github.com/dbx/dbx/internal/protocol"
)

// TxState represents the state of a transaction.
type TxState int

const (
	TxStateNone    TxState = iota
	TxStateOpen              // MULTI received
	TxStateError             // Error queued (from WATCH or command error)
)

// QueuedCommand is a command queued in a MULTI block.
type QueuedCommand struct {
	Cmd *protocol.Command
}

// Transaction represents a MULTI/EXEC transaction for one client.
type Transaction struct {
	mu       sync.Mutex
	ClientID uint64
	State    TxState
	Queue    []*QueuedCommand
	Error    error
}

// MultiManager manages active transactions for all clients.
type MultiManager struct {
	mu  sync.RWMutex
	txs map[uint64]*Transaction
}

// NewMultiManager creates a new transaction manager.
func NewMultiManager() *MultiManager {
	return &MultiManager{txs: make(map[uint64]*Transaction)}
}

// Begin starts a MULTI block for clientID.
func (m *MultiManager) Begin(clientID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tx, ok := m.txs[clientID]; ok && tx.State == TxStateOpen {
		return fmt.Errorf("ERR MULTI calls can not be nested")
	}
	m.txs[clientID] = &Transaction{
		ClientID: clientID,
		State:    TxStateOpen,
	}
	return nil
}

// IsActive returns true if clientID is in a MULTI block.
func (m *MultiManager) IsActive(clientID uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tx, ok := m.txs[clientID]
	return ok && tx.State == TxStateOpen
}

// Queue adds a command to the transaction queue.
func (m *MultiManager) Queue(clientID uint64, cmd *protocol.Command) {
	m.mu.RLock()
	tx := m.txs[clientID]
	m.mu.RUnlock()
	if tx == nil {
		return
	}
	tx.mu.Lock()
	tx.Queue = append(tx.Queue, &QueuedCommand{Cmd: cmd})
	tx.mu.Unlock()
}

// SetError marks the transaction as errored.
func (m *MultiManager) SetError(clientID uint64, err error) {
	m.mu.RLock()
	tx := m.txs[clientID]
	m.mu.RUnlock()
	if tx != nil {
		tx.mu.Lock()
		tx.State = TxStateError
		tx.Error = err
		tx.mu.Unlock()
	}
}

// Exec extracts and removes the queued commands for execution.
// Returns nil if the transaction was aborted.
func (m *MultiManager) Exec(clientID uint64) ([]*QueuedCommand, error) {
	m.mu.Lock()
	tx, ok := m.txs[clientID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("ERR EXEC without MULTI")
	}
	delete(m.txs, clientID)
	m.mu.Unlock()

	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.State == TxStateError {
		return nil, tx.Error
	}
	return tx.Queue, nil
}

// Discard cancels the transaction for clientID.
func (m *MultiManager) Discard(clientID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.txs[clientID]; !ok {
		return fmt.Errorf("ERR DISCARD without MULTI")
	}
	delete(m.txs, clientID)
	return nil
}
