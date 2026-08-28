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
	delete(m.instances, id)
	_ = m.saveState()
	m.mu.Unlock()
	if inst != nil {
		inst.Stop()
	}
	return nil
}

func (m *Manager) WakeTenant(id string) error {
	m.mu.Lock()
	tenant, ok := m.tenants[id]
	if !ok {
		m.mu.Unlock()
		return errTenantNotFound
	}
	tenant.Hibernated = false
	_ = m.saveState()
	m.mu.Unlock()
	return m.StartTenant(tenant)
}
