package orchestrator

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/dbx/dbx/internal/auth"
)

type TenantKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Hash        string    `json:"hash,omitempty"`
	Role        string    `json:"role"`
	KeyPatterns []string  `json:"key_patterns"`
	Generation  uint64    `json:"generation"`
	Revoked     bool      `json:"revoked"`
	CreatedAt   time.Time `json:"created_at"`
}

func rolePermissions(role string) (int, error) {
	switch role {
	case "reader":
		return auth.PermRead, nil
	case "writer":
		return auth.PermRead | auth.PermWrite, nil
	case "tenant-admin":
		return auth.PermRead | auth.PermWrite | auth.PermAdmin, nil
	default:
		return 0, fmt.Errorf("role must be reader, writer, or tenant-admin")
	}
}

func tenantUser(key *TenantKey) *auth.User {
	perms, _ := rolePermissions(key.Role)
	return &auth.User{
		Name: key.ID, PasswordHash: key.Hash, Enabled: !key.Revoked,
		Permissions: perms, AllowedKeys: append([]string(nil), key.KeyPatterns...),
	}
}

func (m *Manager) CreateTenantKey(tenantID, name, role string, patterns []string) (string, *TenantKey, error) {
	if name == "" {
		return "", nil, fmt.Errorf("key name is required")
	}
	if _, err := rolePermissions(role); err != nil {
		return "", nil, err
	}
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	secretBytes := make([]byte, 32)
	idBytes := make([]byte, 8)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", nil, err
	}
	if _, err := rand.Read(idBytes); err != nil {
		return "", nil, err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	key := &TenantKey{
		ID: hex.EncodeToString(idBytes), Name: name, Hash: auth.HashPassword(secret),
		Role: role, KeyPatterns: append([]string(nil), patterns...),
		Generation: 1, CreatedAt: time.Now().UTC(),
	}

	m.mu.Lock()
	tenant, ok := m.tenants[tenantID]
	if !ok {
		m.mu.Unlock()
		return "", nil, fmt.Errorf("tenant not found")
	}
	if tenant.Keys == nil {
		tenant.Keys = make(map[string]*TenantKey)
	}
	tenant.Keys[key.ID] = key
	if err := m.saveState(); err != nil {
		delete(tenant.Keys, key.ID)
		m.mu.Unlock()
		return "", nil, err
	}
	instance := m.instances[tenantID]
	m.mu.Unlock()
	if instance != nil {
		instance.UpsertUser(tenantUser(key))
	}
	return secret, publicTenantKey(key), nil
}

func (m *Manager) ListTenantKeys(tenantID string) ([]*TenantKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tenant, ok := m.tenants[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant not found")
	}
	keys := make([]*TenantKey, 0, len(tenant.Keys))
	for _, key := range tenant.Keys {
		keys = append(keys, publicTenantKey(key))
	}
	return keys, nil
}

func (m *Manager) RevokeTenantKey(tenantID, keyID string) error {
	m.mu.Lock()
	tenant, ok := m.tenants[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant not found")
	}
	key := tenant.Keys[keyID]
	if key == nil {
		m.mu.Unlock()
		return fmt.Errorf("tenant key not found")
	}
	key.Revoked = true
	key.Generation++
	if err := m.saveState(); err != nil {
		key.Revoked = false
		key.Generation--
		m.mu.Unlock()
		return err
	}
	instance := m.instances[tenantID]
	m.mu.Unlock()
	if instance != nil {
		instance.DeleteUser(keyID)
	}
	return nil
}

func (m *Manager) VerifyTenantKey(tenantID, keyID, secret string) (*Tenant, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tenant := m.tenants[tenantID]
	if tenant == nil {
		return nil, false
	}
	key := tenant.Keys[keyID]
	if key == nil || key.Revoked {
		return nil, false
	}
	actual := auth.HashPassword(secret)
	if subtle.ConstantTimeCompare([]byte(actual), []byte(key.Hash)) != 1 {
		return nil, false
	}
	copyTenant := *tenant
	return &copyTenant, true
}

func publicTenantKey(key *TenantKey) *TenantKey {
	copyKey := *key
	copyKey.Hash = ""
	copyKey.KeyPatterns = append([]string(nil), key.KeyPatterns...)
	return &copyKey
}
