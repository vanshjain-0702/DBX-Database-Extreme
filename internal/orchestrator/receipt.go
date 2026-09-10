package orchestrator

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dbx/dbx/internal/isolation"
)

// PurgeReceipt is the durable record that a tenant directory was erased.
// It is an operator artifact, not a compliance certification.
type PurgeReceipt struct {
	ReceiptID   string    `json:"receipt_id"`
	TenantID    string    `json:"tenant_id"`
	Purged      bool      `json:"purged"`
	DEKShredded bool      `json:"dek_shredded"`
	Operator    string    `json:"operator,omitempty"`
	Timestamp   time.Time `json:"ts"`
	WrapSHA256  string    `json:"wrap_sha256,omitempty"`
	Path        string    `json:"path,omitempty"`
}

func (m *Manager) receiptsDir() string {
	if m != nil && m.stateFile != "" {
		return filepath.Join(filepath.Dir(m.stateFile), "receipts")
	}
	return filepath.Join(dataRoot(), "receipts")
}

func newReceiptID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:16])
	}
	return hex.EncodeToString(buf[:])
}

func hashWrapFile(dataDir string) string {
	data, err := os.ReadFile(filepath.Join(dataDir, isolation.WrappedDEKName))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) writePurgeReceipt(t *Tenant, operator string, wrapHash string, dekShredded bool) (*PurgeReceipt, error) {
	receipt := &PurgeReceipt{
		ReceiptID:   newReceiptID(),
		TenantID:    t.ID,
		Purged:      true,
		DEKShredded: dekShredded,
		Operator:    operator,
		Timestamp:   time.Now().UTC(),
		WrapSHA256:  wrapHash,
	}
	dir := m.receiptsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return receipt, fmt.Errorf("receipt directory: %w", err)
	}
	path := filepath.Join(dir, receipt.ReceiptID+".json")
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return receipt, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return receipt, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return receipt, err
	}
	receipt.Path = path
	return receipt, nil
}
