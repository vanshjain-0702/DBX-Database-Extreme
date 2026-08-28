package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTenantUsageReportsDiskForStoppedEngine(t *testing.T) {
	m, acme, _ := newTestManager(t)
	usage, err := m.TenantUsage(acme.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Status != "down" {
		t.Fatalf("status=%s", usage.Status)
	}
	if usage.DiskBytes == 0 {
		t.Fatal("expected tenant directory bytes")
	}
	if usage.TenantID != acme.ID {
		t.Fatalf("id=%s", usage.TenantID)
	}
}

func TestHibernatePersistsAndSkipsAutostart(t *testing.T) {
	m, acme, globex := newTestManager(t)
	acme.HTTPPort = freeTCPPort(t)
	acme.RESPPort = freeTCPPort(t)
	globex.HTTPPort = freeTCPPort(t)
	globex.RESPPort = freeTCPPort(t)
	if err := m.HibernateTenant(acme.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.StartTenant(acme); err == nil {
		t.Fatal("hibernated tenant should not start")
	}
	reloaded, err := NewManager(m.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.StopAll()
	if reloaded.TenantRunning(acme.ID) {
		t.Fatal("hibernated tenant started on reload")
	}
	if !reloaded.TenantRunning(globex.ID) {
		t.Fatal("awake tenant should start on reload")
	}
	views := reloaded.ListTenantViews()
	byID := map[string]TenantView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if byID[acme.ID].Status != "hibernated" {
		t.Fatalf("view=%+v", byID[acme.ID])
	}
}

func TestBackupRestoreRoundTripPreservesTenantFile(t *testing.T) {
	m, acme, _ := newTestManager(t)
	acme.HTTPPort = freeTCPPort(t)
	acme.RESPPort = freeTCPPort(t)
	if err := m.StartTenant(acme); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer m.StopAll()
	path, manifest, err := m.BackupTenant(acme.ID)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if len(manifest.Files) == 0 {
		t.Fatal("backup archive listed no files")
	}
	entries, err := os.ReadDir(acme.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(acme.DataDir, entry.Name()))
	}
	if err := m.RestoreTenant(acme.ID, path); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !m.TenantRunning(acme.ID) {
		t.Fatal("tenant should be running after restore")
	}
}
