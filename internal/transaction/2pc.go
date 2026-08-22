package transaction

import (
	"context"
	"fmt"
	"sync"
)

// TwoPCState is the state of a 2PC transaction.
type TwoPCState int

const (
	TwoPCPrepared  TwoPCState = iota
	TwoPCCommitted
	TwoPCAborted
)

// TwoPCTransaction represents a cross-shard two-phase commit transaction.
type TwoPCTransaction struct {
	ID         string
	State      TwoPCState
	Shards     []string
	prepareAck map[string]bool
	mu         sync.Mutex
}

// TwoPCCoordinator coordinates 2PC transactions across shards.
type TwoPCCoordinator struct {
	mu   sync.RWMutex
	txns map[string]*TwoPCTransaction
}

// NewTwoPCCoordinator creates a new coordinator.
func NewTwoPCCoordinator() *TwoPCCoordinator {
	return &TwoPCCoordinator{txns: make(map[string]*TwoPCTransaction)}
}

// Begin starts a new 2PC transaction with the given ID and participating shards.
func (c *TwoPCCoordinator) Begin(txID string, shards []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.txns[txID]; ok {
		return fmt.Errorf("2PC: transaction %s already exists", txID)
	}
	c.txns[txID] = &TwoPCTransaction{
		ID:         txID,
		State:      TwoPCPrepared,
		Shards:     shards,
		prepareAck: make(map[string]bool),
	}
	return nil
}

// Prepare sends PREPARE to all shards and waits for acks.
func (c *TwoPCCoordinator) Prepare(ctx context.Context, txID string, prepareFunc func(shard string) error) error {
	c.mu.RLock()
	tx := c.txns[txID]
	c.mu.RUnlock()
	if tx == nil {
		return fmt.Errorf("2PC: unknown transaction %s", txID)
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(tx.Shards))
	for _, shard := range tx.Shards {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			if err := prepareFunc(s); err != nil {
				errs <- err
			} else {
				tx.mu.Lock()
				tx.prepareAck[s] = true
				tx.mu.Unlock()
			}
		}(shard)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return fmt.Errorf("2PC prepare failed: %w", err)
		}
	}
	return nil
}

// Commit sends COMMIT to all shards.
func (c *TwoPCCoordinator) Commit(ctx context.Context, txID string, commitFunc func(shard string) error) error {
	return c.broadcast(ctx, txID, commitFunc, TwoPCCommitted)
}

// Abort sends ABORT to all shards.
func (c *TwoPCCoordinator) Abort(ctx context.Context, txID string, abortFunc func(shard string) error) error {
	return c.broadcast(ctx, txID, abortFunc, TwoPCAborted)
}

func (c *TwoPCCoordinator) broadcast(ctx context.Context, txID string, fn func(string) error, finalState TwoPCState) error {
	c.mu.Lock()
	tx := c.txns[txID]
	if tx != nil {
		tx.State = finalState
	}
	c.mu.Unlock()
	if tx == nil {
		return fmt.Errorf("2PC: unknown transaction %s", txID)
	}
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for _, shard := range tx.Shards {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			if err := fn(s); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(shard)
	}
	wg.Wait()
	return firstErr
}
