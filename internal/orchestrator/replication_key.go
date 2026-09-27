package orchestrator

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const replicationKeyFile = "replication.key"

// replicationToken returns the shared handshake key for one replication
// group: the public tenant and its replicas. It is derived from a node secret
// that lives beside state.json, outside every tenant directory, so a tenant
// worker confined by Landlock cannot read it and cannot pull a neighbour's WAL
// stream. Promote keeps the public id, so the key survives failover.
func (m *Manager) replicationToken(groupID string) (string, error) {
	key, err := m.replicationKey()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("dbx-repl-v1:" + groupID))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func replicationGroup(t *Tenant) string {
	if t.Role == "replica" && t.ReplicaOf != "" {
		return t.ReplicaOf
	}
	return t.ID
}

func (m *Manager) replicationKey() ([]byte, error) {
	m.replKeyMu.Lock()
	defer m.replKeyMu.Unlock()
	if m.replKey != nil {
		return m.replKey, nil
	}
	if m.stateFile == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		m.replKey = key
		return key, nil
	}
	path := filepath.Join(filepath.Dir(m.stateFile), replicationKeyFile)
	key, err := loadReplicationKey(path)
	if errors.Is(err, os.ErrNotExist) {
		key, err = createReplicationKey(path)
		if errors.Is(err, os.ErrExist) {
			key, err = loadReplicationKey(path)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("replication key %s: %w", path, err)
	}
	m.replKey = key
	return key, nil
}

func loadReplicationKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("corrupt replication key; expected 64 hex characters")
	}
	return key, nil
}

func createReplicationKey(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(hex.EncodeToString(key)); err != nil {
		f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return key, nil
}
