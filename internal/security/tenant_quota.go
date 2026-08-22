package security

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// TenantQuota tracks resource usage per tenant.
type TenantQuota struct {
	TenantID    string
	MaxMemBytes int64
	MaxOpsPerSec int64
	MaxKeys     int64
}

// TenantUsage tracks current usage for a tenant.
type TenantUsage struct {
	MemBytes int64
	OpsCount int64
	KeyCount int64
}

// TenantQuotaManager enforces per-tenant quotas.
type TenantQuotaManager struct {
	mu     sync.RWMutex
	quotas map[string]*TenantQuota
	usage  map[string]*tenantUsageInternal
}

type tenantUsageInternal struct {
	memBytes int64
	opsCount int64
	keyCount int64
}

// NewTenantQuotaManager creates a new quota manager.
func NewTenantQuotaManager() *TenantQuotaManager {
	return &TenantQuotaManager{
		quotas: make(map[string]*TenantQuota),
		usage:  make(map[string]*tenantUsageInternal),
	}
}

// SetQuota configures quotas for a tenant.
func (m *TenantQuotaManager) SetQuota(q *TenantQuota) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotas[q.TenantID] = q
	if _, ok := m.usage[q.TenantID]; !ok {
		m.usage[q.TenantID] = &tenantUsageInternal{}
	}
}

// CheckMemory returns error if tenant would exceed memory quota.
func (m *TenantQuotaManager) CheckMemory(tenantID string, bytes int64) error {
	m.mu.RLock()
	q := m.quotas[tenantID]
	u := m.usage[tenantID]
	m.mu.RUnlock()
	if q == nil || u == nil {
		return nil
	}
	if q.MaxMemBytes > 0 && atomic.LoadInt64(&u.memBytes)+bytes > q.MaxMemBytes {
		return fmt.Errorf("OOM quota exceeded for tenant %s", tenantID)
	}
	return nil
}

// TrackWrite updates memory and key counts for a tenant write.
func (m *TenantQuotaManager) TrackWrite(tenantID string, bytes int64, newKey bool) {
	m.mu.RLock()
	u := m.usage[tenantID]
	m.mu.RUnlock()
	if u == nil {
		return
	}
	atomic.AddInt64(&u.memBytes, bytes)
	atomic.AddInt64(&u.opsCount, 1)
	if newKey {
		atomic.AddInt64(&u.keyCount, 1)
	}
}

// Usage returns current usage for a tenant.
func (m *TenantQuotaManager) Usage(tenantID string) *TenantUsage {
	m.mu.RLock()
	u := m.usage[tenantID]
	m.mu.RUnlock()
	if u == nil {
		return &TenantUsage{}
	}
	return &TenantUsage{
		MemBytes: atomic.LoadInt64(&u.memBytes),
		OpsCount: atomic.LoadInt64(&u.opsCount),
		KeyCount: atomic.LoadInt64(&u.keyCount),
	}
}
