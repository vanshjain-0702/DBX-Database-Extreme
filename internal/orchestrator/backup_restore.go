package orchestrator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/persistence"
)

func (m *Manager) BackupTenant(id string) (string, persistence.BackupManifest, error) {
	m.mu.RLock()
	tenant := m.tenants[id]
	instance := m.instances[id]
	worker := m.workers[id]
	m.mu.RUnlock()
	if tenant == nil {
		return "", persistence.BackupManifest{}, fmt.Errorf("tenant not found")
	}
	if instance == nil && worker == nil {
		return "", persistence.BackupManifest{}, fmt.Errorf("tenant is not running")
	}
	name := fmt.Sprintf("backup_%s_%s.dbx.zip", id, time.Now().UTC().Format("20060102_150405"))
	final := filepath.Join(dataRoot(), "backups", name)
	var manifest persistence.BackupManifest
	var err error
	if instance != nil {
		manifest, err = instance.CreateBackup(id, final)
		return final, manifest, err
	}
	// A sandboxed worker can only write inside its own tenant directory, so it
	// stages the archive there and the orchestrator moves it into the shared
	// backup directory afterwards.
	staged := filepath.Join(tenant.DataDir, "backups", name)
	manifest, err = worker.CreateBackup(id, staged)
	if err != nil {
		return "", manifest, err
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return "", manifest, err
	}
	if err := moveFile(staged, final); err != nil {
		return "", manifest, fmt.Errorf("relocating tenant archive: %w", err)
	}
	return final, manifest, nil
}

// moveFile renames across directories, falling back to copy when the staging
// directory and the backup directory are on different filesystems.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Remove(from)
}

func (m *Manager) RestoreTenant(id, archivePath string) error {
	m.mu.Lock()
	tenant := m.tenants[id]
	instance := m.instances[id]
	worker := m.workers[id]
	if tenant == nil {
		m.mu.Unlock()
		return fmt.Errorf("tenant not found")
	}
	delete(m.instances, id)
	delete(m.workers, id)
	m.mu.Unlock()
	if instance != nil {
		instance.Stop()
	}
	if worker != nil {
		worker.Stop()
	}

	parent := filepath.Dir(tenant.DataDir)
	staging := filepath.Join(parent, "."+filepath.Base(tenant.DataDir)+".restore-"+fmt.Sprint(time.Now().UnixNano()))
	rollback := tenant.DataDir + ".rollback"
	_ = os.RemoveAll(staging)
	_ = os.RemoveAll(rollback)
	if err := os.MkdirAll(staging, 0700); err != nil {
		return err
	}
	quota := int64(0)
	if cfg, err := config.Load(fmt.Sprintf("./configs/tenant-%s.yaml", id)); err == nil {
		quota, _ = config.ParseBytes(cfg.Engine.MaxMemory)
	}
	if _, err := persistence.ExtractAndValidateBackup(archivePath, staging, id, quota*2+(64<<20)); err != nil {
		_ = os.RemoveAll(staging)
		_ = m.StartTenant(tenant)
		return err
	}
	if err := os.Rename(tenant.DataDir, rollback); err != nil && !os.IsNotExist(err) {
		_ = os.RemoveAll(staging)
		_ = m.StartTenant(tenant)
		return err
	}
	if err := os.Rename(staging, tenant.DataDir); err != nil {
		_ = os.Rename(rollback, tenant.DataDir)
		_ = m.StartTenant(tenant)
		return err
	}
	if err := m.StartTenant(tenant); err != nil {
		_ = os.RemoveAll(tenant.DataDir)
		_ = os.Rename(rollback, tenant.DataDir)
		_ = m.StartTenant(tenant)
		return fmt.Errorf("restored tenant failed validation/startup: %w", err)
	}
	_ = os.RemoveAll(rollback)
	return nil
}
