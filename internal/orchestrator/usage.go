package orchestrator

import (
	"os"
	"path/filepath"
)

// TenantUsage is what an operator's billing loop should call.
// Isolation without a meter is a demo.
type TenantUsage struct {
	TenantID         string `json:"tenant_id"`
	Status           string `json:"status"`
	Hibernated       bool   `json:"hibernated"`
	Keys             int64  `json:"keys"`
	Vectors          int64  `json:"vectors"`
	MemoryUsedBytes  int64  `json:"memory_used_bytes"`
	MemoryLimitBytes int64  `json:"memory_limit_bytes"`
	DiskBytes        int64  `json:"disk_bytes"`
	Commands         int64  `json:"commands"`
	Errors           int64  `json:"errors"`
	AvgLatencyNs     int64  `json:"avg_latency_ns"`
}

func (m *Manager) TenantUsage(id string) (TenantUsage, error) {
	m.mu.RLock()
	tenant, ok := m.tenants[id]
	inst := m.instances[id]
	worker := m.workers[id]
	m.mu.RUnlock()
	if !ok {
		return TenantUsage{}, errTenantNotFound
	}
	usage := TenantUsage{TenantID: id, Hibernated: tenant.Hibernated}
	if tenant.Hibernated {
		usage.Status = "hibernated"
	} else if inst != nil {
		usage.Status = "running"
		snap := inst.UsageSnapshot()
		usage.Keys = snap.Keys
		usage.Vectors = snap.Vectors
		usage.MemoryUsedBytes = snap.MemoryUsedBytes
		usage.MemoryLimitBytes = snap.MemoryLimitBytes
		usage.Commands = snap.Commands
		usage.Errors = snap.Errors
		usage.AvgLatencyNs = snap.AvgLatencyNs
	} else if worker != nil {
		usage.Status = "running"
		snap := worker.UsageSnapshot()
		usage.Keys = snap.Keys
		usage.Vectors = snap.Vectors
		usage.MemoryUsedBytes = snap.MemoryUsedBytes
		usage.MemoryLimitBytes = snap.MemoryLimitBytes
		usage.Commands = snap.Commands
		usage.Errors = snap.Errors
		usage.AvgLatencyNs = snap.AvgLatencyNs
	} else {
		usage.Status = "down"
	}
	usage.DiskBytes = dirSize(tenant.DataDir)
	return usage, nil
}

func (m *Manager) ListUsage() []TenantUsage {
	m.mu.RLock()
	ids := make([]string, 0, len(m.tenants))
	for id := range m.tenants {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	out := make([]TenantUsage, 0, len(ids))
	for _, id := range ids {
		u, err := m.TenantUsage(id)
		if err != nil {
			continue
		}
		out = append(out, u)
	}
	return out
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
