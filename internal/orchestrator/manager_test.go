package orchestrator

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/server"
)

// newTestManager builds a Manager with two tenants whose data directories exist
// on disk, without starting any engines.
func newTestManager(t *testing.T) (*Manager, *Tenant, *Tenant) {
	t.Helper()
	root := t.TempDir()

	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    filepath.Join(root, "state.json"),
		instances:    make(map[string]*server.Instance),
		starting:     make(map[string]bool),
		restarts:     make(map[string]int),
		tenantQuotas: make(map[string]int64),
		nextHTTPPort: 18081,
		nextRESPPort: 16401,
		nextReplPort: 17401,
	}

	mk := func(id string) *Tenant {
		dir := filepath.Join(root, "tenants", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dump.rdb"), []byte(id), 0o644); err != nil {
			t.Fatalf("seed %s: %v", dir, err)
		}
		tn := &Tenant{ID: id, Name: id, DataDir: dir}
		m.tenants[id] = tn
		return tn
	}

	return m, mk("acme"), mk("globex")
}

func TestDeleteTenantPurgesOnlyThatTenant(t *testing.T) {
	m, acme, globex := newTestManager(t)

	if err := m.DeleteTenant(acme.ID, true); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}

	if _, ok := m.GetTenant(acme.ID); ok {
		t.Error("deleted tenant is still in the control plane")
	}
	if _, err := os.Stat(acme.DataDir); !os.IsNotExist(err) {
		t.Errorf("purge left data behind at %s (err=%v)", acme.DataDir, err)
	}

	if _, ok := m.GetTenant(globex.ID); !ok {
		t.Error("unrelated tenant was removed from the control plane")
	}
	if _, err := os.Stat(filepath.Join(globex.DataDir, "dump.rdb")); err != nil {
		t.Errorf("unrelated tenant lost data: %v", err)
	}
}

func TestDeleteTenantWithoutPurgeKeepsData(t *testing.T) {
	m, acme, _ := newTestManager(t)

	if err := m.DeleteTenant(acme.ID, false); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}

	if _, ok := m.GetTenant(acme.ID); ok {
		t.Error("tenant should be gone from the control plane")
	}
	if _, err := os.Stat(filepath.Join(acme.DataDir, "dump.rdb")); err != nil {
		t.Errorf("data should survive a non-purging delete: %v", err)
	}
}

func TestListTenantViewsReportsRunningAndDown(t *testing.T) {
	m, acme, globex := newTestManager(t)
	acme.HTTPPort, acme.RESPPort = 8083, 6403
	globex.HTTPPort, globex.RESPPort = 8084, 6404
	m.instances[acme.ID] = &server.Instance{}

	views := m.ListTenantViews()
	if len(views) != 2 {
		t.Fatalf("got %d views, want 2", len(views))
	}
	byID := map[string]TenantView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if !byID[acme.ID].Healthy || byID[acme.ID].Status != "running" {
		t.Fatalf("running tenant: %+v", byID[acme.ID])
	}
	if byID[globex.ID].Healthy || byID[globex.ID].Status != "down" {
		t.Fatalf("down tenant: %+v", byID[globex.ID])
	}
	if byID[acme.ID].HTTPPort != 8083 || byID[acme.ID].RESPPort != 6403 {
		t.Fatalf("ports missing: %+v", byID[acme.ID])
	}
}

func TestDeleteUnknownTenant(t *testing.T) {
	m, _, _ := newTestManager(t)

	if err := m.DeleteTenant("nobody", true); err == nil {
		t.Error("expected an error when deleting a tenant that does not exist")
	}
}

func TestTenantKeyLifecycleAndPersistence(t *testing.T) {
	m, acme, _ := newTestManager(t)
	secret, key, err := m.CreateTenantKey(acme.ID, "agent-reader", "reader", []string{"agent:*"})
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || key.Hash != "" {
		t.Fatal("secret was not returned once or hash leaked")
	}
	if _, ok := m.VerifyTenantKey(acme.ID, key.ID, secret); !ok {
		t.Fatal("new tenant key did not authenticate")
	}
	if _, ok := m.VerifyTenantKey(acme.ID, key.ID, secret+"bad"); ok {
		t.Fatal("invalid secret authenticated")
	}

	reloaded, err := NewManager(m.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.StopAll()
	if _, ok := reloaded.VerifyTenantKey(acme.ID, key.ID, secret); !ok {
		t.Fatal("persisted tenant key did not authenticate")
	}
	if err := m.RevokeTenantKey(acme.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.VerifyTenantKey(acme.ID, key.ID, secret); ok {
		t.Fatal("revoked tenant key still authenticated")
	}
}

func TestProvisionCreatesAsyncReplicas(t *testing.T) {
	root := t.TempDir()
	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    filepath.Join(root, "state.json"),
		instances:    make(map[string]*server.Instance),
		starting:     make(map[string]bool),
		restarts:     make(map[string]int),
		tenantQuotas: make(map[string]int64),
		nextHTTPPort: 18081,
		nextRESPPort: 16401,
		nextReplPort: 17401,
	}
	primary, err := m.Provision("acme", "Acme", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	if primary.Role != "primary" || primary.ReplicationPort != 17401 || len(primary.Replicas) != 1 {
		t.Fatalf("primary = %+v", primary)
	}
	replica, ok := m.GetTenant("acme-r1")
	if !ok || replica.Role != "replica" || replica.ReplicaOf != "acme" {
		t.Fatalf("replica = %+v ok=%v", replica, ok)
	}
	if _, err := m.Provision("too-many", "x", 3); err == nil {
		t.Fatal("expected replica cap")
	}
}

func TestPromoteSwapsPrimaryPorts(t *testing.T) {
	m, _, _ := newTestManager(t)
	primary := &Tenant{
		ID: "acme", Name: "Acme", HTTPPort: 8081, RESPPort: 6401,
		ReplicationPort: 7401, Role: "primary", DataDir: t.TempDir(),
		Replicas: []string{"acme-r1"},
	}
	replica := &Tenant{
		ID: "acme-r1", Name: "Acme replica 1", HTTPPort: 8082, RESPPort: 6402,
		ReplicationPort: 7402, Role: "replica", ReplicaOf: "acme", DataDir: t.TempDir(),
	}
	m.tenants[primary.ID] = primary
	m.tenants[replica.ID] = replica
	oldPrimaryDir := primary.DataDir
	oldReplicaDir := replica.DataDir
	if err := m.Promote("acme-r1"); err != nil {
		t.Fatal(err)
	}
	if primary.DataDir != oldReplicaDir || replica.DataDir != oldPrimaryDir {
		t.Fatalf("data dirs were not swapped: primary=%s replica=%s", primary.DataDir, replica.DataDir)
	}
	if primary.RESPPort != 6402 || replica.RESPPort != 6401 {
		t.Fatalf("ports were not swapped: primary=%d replica=%d", primary.RESPPort, replica.RESPPort)
	}
	if err := m.Promote("acme"); err == nil {
		t.Fatal("promoting a primary should fail")
	}
}

func TestDeletePrimaryRemovesReplicas(t *testing.T) {
	root := t.TempDir()
	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    filepath.Join(root, "state.json"),
		instances:    make(map[string]*server.Instance),
		starting:     make(map[string]bool),
		restarts:     make(map[string]int),
		tenantQuotas: make(map[string]int64),
	}
	primaryDir := filepath.Join(root, "primary")
	replicaDir := filepath.Join(root, "replica")
	if err := os.MkdirAll(primaryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(replicaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m.tenants["acme"] = &Tenant{
		ID: "acme", DataDir: primaryDir, Role: "primary", Replicas: []string{"acme-r1"},
	}
	m.tenants["acme-r1"] = &Tenant{
		ID: "acme-r1", DataDir: replicaDir, Role: "replica", ReplicaOf: "acme",
	}
	if err := m.DeleteTenant("acme", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetTenant("acme"); ok {
		t.Fatal("primary still present")
	}
	if _, ok := m.GetTenant("acme-r1"); ok {
		t.Fatal("replica still present")
	}
}

func TestStartTenantWithoutCheckoutYAML(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DBX_DATA_DIR", root)
	t.Setenv("DBX_DEFAULT_PASSWORD", "launch-secret")
	respPort := freeTCPPort(t)
	httpPort := freeTCPPort(t)
	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    filepath.Join(root, "state.json"),
		instances:    make(map[string]*server.Instance),
		starting:     make(map[string]bool),
		restarts:     make(map[string]int),
		tenantQuotas: make(map[string]int64),
		nextHTTPPort: httpPort,
		nextRESPPort: respPort,
		nextReplPort: freeTCPPort(t),
	}
	primary, err := m.Provision("launch", "Launch", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m.TenantRunning(primary.ID) {
			if _, err := os.Stat(filepath.Join(primary.DataDir, "engine.yaml")); err != nil {
				t.Fatalf("engine.yaml: %v", err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tenant did not start without configs/local.yaml")
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
