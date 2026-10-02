package orchestrator

import "fmt"

var errTenantNotFound = fmt.Errorf("tenant not found")

func (m *Manager) HibernateTenant(id string) error {
	m.mu.Lock()
	tenant, ok := m.tenants[id]
	if !ok {
		m.mu.Unlock()
		return errTenantNotFound
	}
	if tenant.Role == "replica" {
		m.mu.Unlock()
		return fmt.Errorf("hibernate the primary, not replica %s", id)
	}
	tenant.Hibernated = true
	inst := m.instances[id]
	worker := m.workers[id]
	delete(m.instances, id)
	delete(m.workers, id)
	if err := m.saveState(); err != nil {
		tenant.Hibernated = false
		if inst != nil {
			m.instances[id] = inst
		}
		if worker != nil {
			m.workers[id] = worker
		}
		m.mu.Unlock()
		return fmt.Errorf("persist hibernation state: %w", err)
	}
	m.mu.Unlock()
	if inst != nil {
		inst.Stop()
	}
	if worker != nil {
		worker.Stop()
	}
	return nil
}

func (m *Manager) WakeTenant(id string) error {
	return m.wakeTenant(id, false)
}

// WakeTenantIfHibernated enforces the support recovery precondition atomically
// with the lifecycle-state update.
func (m *Manager) WakeTenantIfHibernated(id string) error {
	return m.wakeTenant(id, true)
}

func (m *Manager) wakeTenant(id string, requireHibernated bool) error {
	m.mu.Lock()
	tenant, ok := m.tenants[id]
	if !ok {
		m.mu.Unlock()
		return errTenantNotFound
	}
	if requireHibernated && !tenant.Hibernated {
		m.mu.Unlock()
		return fmt.Errorf("tenant is not hibernated")
	}
	tenant.Hibernated = false
	if err := m.saveState(); err != nil {
		tenant.Hibernated = true
		m.mu.Unlock()
		return fmt.Errorf("persist wake state: %w", err)
	}
	m.mu.Unlock()
	return m.StartTenant(tenant)
}
