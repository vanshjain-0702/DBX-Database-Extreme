package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/security"
	"github.com/dbx/dbx/internal/server"
	"github.com/hashicorp/raft"
	"gopkg.in/yaml.v3"
)

type Tenant struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	HTTPPort        int                   `json:"http_port"`
	RESPPort        int                   `json:"resp_port"`
	GRPCPort        int                   `json:"grpc_port"`
	RaftPort        int                   `json:"raft_port"`
	DataDir         string                `json:"data_dir"`
	Role            string                `json:"role,omitempty"`
	ReplicaOf       string                `json:"replica_of,omitempty"`
	ReplicationPort int                   `json:"replication_port,omitempty"`
	Replicas        []string              `json:"replicas,omitempty"`
	Keys            map[string]*TenantKey `json:"keys,omitempty"`
	Hibernated      bool                  `json:"hibernated,omitempty"`
}

const (
	maxTenantsPerNode      = 100
	maxReplicasPerTenant   = 2
	defaultReplicationPort = 7401
)

type Manager struct {
	mu           sync.RWMutex
	tenants      map[string]*Tenant
	stateFile    string
	nextHTTPPort int
	nextRESPPort int
	instances    map[string]*server.Instance
	workers      map[string]*isolatedWorker
	startMu      sync.Mutex
	starting     map[string]bool
	restarts     map[string]int
	tenantQuotas map[string]int64
	nodeBudget   int64
	nextReplPort int
	RaftNode     *RaftNode
	profile      isolation.Profile
}

func NewManager(stateFile string) (*Manager, error) {
	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    stateFile,
		nextHTTPPort: 8081,
		nextRESPPort: 6401,
		nextReplPort: defaultReplicationPort,
		instances:    make(map[string]*server.Instance),
		workers:      make(map[string]*isolatedWorker),
		starting:     make(map[string]bool),
		restarts:     make(map[string]int),
		tenantQuotas: make(map[string]int64),
		profile:      isolation.FromEnv(),
	}
	if configured := os.Getenv("DBX_NODE_MEMORY_BUDGET"); configured != "" {
		budget, err := config.ParseBytes(configured)
		if err != nil {
			return nil, fmt.Errorf("invalid DBX_NODE_MEMORY_BUDGET: %w", err)
		}
		m.nodeBudget = budget
	}
	if err := m.loadState(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// Resume existing processes. Start primaries before replicas so the WAL
	// listener is bound before replica streams retry.
	var primaries, replicas []*Tenant
	for _, t := range m.tenants {
		if t.HTTPPort >= m.nextHTTPPort {
			m.nextHTTPPort = t.HTTPPort + 1
		}
		if t.RESPPort >= m.nextRESPPort {
			m.nextRESPPort = t.RESPPort + 1
		}
		if t.ReplicationPort >= m.nextReplPort {
			m.nextReplPort = t.ReplicationPort + 1
		}
		// Ensure no port collisions in state.json
		if t.HTTPPort == 0 {
			t.HTTPPort = m.nextHTTPPort
			m.nextHTTPPort++
		}
		if t.RESPPort == 0 {
			t.RESPPort = m.nextRESPPort
			m.nextRESPPort++
		}
		t.GRPCPort = 0
		t.RaftPort = 0
		if t.Role == "replica" {
			replicas = append(replicas, t)
		} else {
			primaries = append(primaries, t)
		}
	}
	start := func(t *Tenant) {
		if t.Hibernated {
			return
		}
		if err := m.StartTenant(t); err != nil {
			fmt.Printf("[Orchestrator] tenant %s failed to start: %v\n", t.ID, err)
		}
	}
	// Start synchronously so primaries bind the WAL listener before replicas
	// connect, and so tests can StopAll before t.TempDir cleanup. Background
	// starts raced with directory removal on macOS GitHub runners.
	for _, t := range primaries {
		start(t)
	}
	for _, t := range replicas {
		start(t)
	}
	return m, nil
}

func (m *Manager) loadState() error {
	b, err := os.ReadFile(m.stateFile)
	if err != nil {
		// A crash between rotating the old state file and installing the new
		// one leaves the previous complete copy at .bak.
		b, err = os.ReadFile(m.stateFile + ".bak")
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(b, &m.tenants)
}

func (m *Manager) saveState() error {
	b, err := json.MarshalIndent(m.tenants, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.stateFile), 0755); err != nil {
		return err
	}
	tmp := m.stateFile + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}

	backup := m.stateFile + ".bak"
	_ = os.Remove(backup)
	if _, statErr := os.Stat(m.stateFile); statErr == nil {
		if err := os.Rename(m.stateFile, backup); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, m.stateFile); err != nil {
		_ = os.Rename(backup, m.stateFile)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func dataRoot() string {
	if v := strings.TrimSpace(os.Getenv("DBX_DATA_DIR")); v != "" {
		return v
	}
	return "data"
}

func replicaTenantID(id string, n int) string {
	return fmt.Sprintf("%s-r%d", id, n)
}

func (m *Manager) Provision(id, name string, replicaCount int) (*Tenant, error) {
	if id == "" {
		return nil, fmt.Errorf("invalid tenant id")
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
			return nil, fmt.Errorf("invalid tenant id: only alphanumeric and dashes allowed")
		}
	}
	if name == "" {
		return nil, fmt.Errorf("tenant name is required")
	}
	if replicaCount < 0 {
		return nil, fmt.Errorf("replica count must be >= 0")
	}
	if replicaCount > maxReplicasPerTenant {
		return nil, fmt.Errorf("at most %d replicas per tenant", maxReplicasPerTenant)
	}
	m.mu.Lock()
	if _, exists := m.tenants[id]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("tenant already exists")
	}
	for n := 1; n <= replicaCount; n++ {
		rid := replicaTenantID(id, n)
		if _, exists := m.tenants[rid]; exists {
			m.mu.Unlock()
			return nil, fmt.Errorf("replica id %s already exists", rid)
		}
	}
	if len(m.tenants)+1+replicaCount > maxTenantsPerNode {
		m.mu.Unlock()
		return nil, fmt.Errorf("v1 tenant limit reached: maximum %d tenants per node", maxTenantsPerNode)
	}

	primary := &Tenant{
		ID:       id,
		Name:     name,
		HTTPPort: m.nextHTTPPort,
		RESPPort: m.nextRESPPort,
		DataDir:  filepath.Join(dataRoot(), "tenants", id),
	}
	m.nextHTTPPort++
	m.nextRESPPort++
	var replicas []*Tenant
	if replicaCount > 0 {
		primary.Role = "primary"
		primary.ReplicationPort = m.nextReplPort
		m.nextReplPort++
		primary.Replicas = make([]string, 0, replicaCount)
		for n := 1; n <= replicaCount; n++ {
			rid := replicaTenantID(id, n)
			rt := &Tenant{
				ID:              rid,
				Name:            fmt.Sprintf("%s replica %d", name, n),
				HTTPPort:        m.nextHTTPPort,
				RESPPort:        m.nextRESPPort,
				DataDir:         filepath.Join(dataRoot(), "tenants", rid),
				Role:            "replica",
				ReplicaOf:       id,
				ReplicationPort: m.nextReplPort,
			}
			m.nextHTTPPort++
			m.nextRESPPort++
			m.nextReplPort++
			primary.Replicas = append(primary.Replicas, rid)
			replicas = append(replicas, rt)
		}
	}
	m.mu.Unlock()

	if m.RaftNode != nil {
		if m.RaftNode.Raft.State() != raft.Leader {
			return nil, fmt.Errorf("provisioning failed: not the leader")
		}
		cmd := fsmUpdateCommand{Action: "provision", Tenant: primary, Members: replicas}
		data, _ := json.Marshal(cmd)
		future := m.RaftNode.Raft.Apply(data, 10*time.Second)
		if err := future.Error(); err != nil {
			return nil, err
		}
		return primary, nil
	}
	m.mu.Lock()
	m.tenants[id] = primary
	for _, r := range replicas {
		m.tenants[r.ID] = r
	}
	err := m.saveState()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	go m.startReplicaSet(primary, replicas)
	return primary, nil
}

func (m *Manager) startReplicaSet(primary *Tenant, replicas []*Tenant) {
	if err := m.StartTenant(primary); err != nil {
		fmt.Printf("[Orchestrator] tenant %s failed to start: %v\n", primary.ID, err)
		return
	}
	for _, r := range replicas {
		if err := m.StartTenant(r); err != nil {
			fmt.Printf("[Orchestrator] replica %s failed to start: %v\n", r.ID, err)
		}
	}
}

// DeleteTenant removes a tenant from the control plane and stops its engine.
// When purge is true the tenant's data directory is erased as well, which is
// what customer off-boarding and "delete my data" requests require.
func (m *Manager) DeleteTenant(id string, purge bool) error {
	m.mu.RLock()
	t, ok := m.tenants[id]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("tenant not found")
	}
	var members []*Tenant
	if t.Role != "replica" {
		for _, rid := range t.Replicas {
			if rt, ok := m.tenants[rid]; ok {
				members = append(members, rt)
			}
		}
	}
	replicaOf := t.ReplicaOf
	m.mu.RUnlock()

	if m.RaftNode != nil {
		if m.RaftNode.Raft.State() != raft.Leader {
			return fmt.Errorf("deprovisioning failed: not the leader")
		}
		cmd := fsmUpdateCommand{Action: "deprovision", Tenant: t, Members: members, Purge: purge}
		data, err := json.Marshal(cmd)
		if err != nil {
			return err
		}
		return m.RaftNode.Raft.Apply(data, 10*time.Second).Error()
	}

	for _, member := range members {
		if err := m.removeTenant(member, purge); err != nil {
			return err
		}
	}
	if err := m.removeTenant(t, purge); err != nil {
		return err
	}
	if replicaOf == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	primary, ok := m.tenants[replicaOf]
	if !ok {
		return nil
	}
	kept := make([]string, 0, len(primary.Replicas))
	for _, rid := range primary.Replicas {
		if rid != id {
			kept = append(kept, rid)
		}
	}
	primary.Replicas = kept
	return m.saveState()
}

// removeTenant tears down a single tenant locally: stop the engine, drop the
// control-plane record, then optionally erase its isolated data directory.
func (m *Manager) removeTenant(t *Tenant, purge bool) error {
	m.mu.Lock()
	inst := m.instances[t.ID]
	worker := m.workers[t.ID]
	delete(m.instances, t.ID)
	delete(m.workers, t.ID)
	delete(m.tenantQuotas, t.ID)
	delete(m.tenants, t.ID)
	saveErr := m.saveState()
	m.mu.Unlock()

	if inst != nil {
		inst.Stop()
	}
	if worker != nil {
		worker.Stop()
	}
	if saveErr != nil {
		return saveErr
	}

	_ = os.Remove(fmt.Sprintf("./configs/tenant-%s.yaml", t.ID))

	if purge {
		_ = isolation.ShredDEK(t.DataDir)
		if err := os.RemoveAll(t.DataDir); err != nil {
			return fmt.Errorf("tenant %s removed from control plane but data purge failed: %w", t.ID, err)
		}
	}
	fmt.Printf("[Orchestrator] Deleted tenant %s (purge=%v)\n", t.ID, purge)
	return nil
}

func (m *Manager) GetTenant(id string) (*Tenant, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[id]
	return t, ok
}

// TenantRunning reports whether this process currently has a live engine for id.
func (m *Manager) TenantRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, inst := m.instances[id]
	_, worker := m.workers[id]
	return inst || worker
}

func (m *Manager) ListTenants() []*Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*Tenant
	for _, t := range m.tenants {
		list = append(list, t)
	}
	return list
}

// TenantView is the control-plane list payload: identity, ports, and live health.
// Keys are omitted so the dashboard never receives credential material.
type TenantView struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	HTTPPort        int      `json:"http_port"`
	RESPPort        int      `json:"resp_port"`
	Status          string   `json:"status"`
	Healthy         bool     `json:"healthy"`
	Engine          string   `json:"engine"`
	Role            string   `json:"role,omitempty"`
	ReplicaOf       string   `json:"replica_of,omitempty"`
	ReplicationPort int      `json:"replication_port,omitempty"`
	Replicas        []string `json:"replicas,omitempty"`
	Hibernated      bool     `json:"hibernated,omitempty"`
}

// ListTenantViews returns tenants with running/starting/down derived from in-process engines.
func (m *Manager) ListTenantViews() []TenantView {
	m.mu.RLock()
	views := make([]TenantView, 0, len(m.tenants))
	running := make(map[string]bool, len(m.instances)+len(m.workers))
	for id := range m.instances {
		running[id] = true
	}
	for id := range m.workers {
		running[id] = true
	}
	for _, t := range m.tenants {
		views = append(views, TenantView{
			ID:              t.ID,
			Name:            t.Name,
			HTTPPort:        t.HTTPPort,
			RESPPort:        t.RESPPort,
			Engine:          "DBX",
			Role:            t.Role,
			ReplicaOf:       t.ReplicaOf,
			ReplicationPort: t.ReplicationPort,
			Replicas:        append([]string(nil), t.Replicas...),
			Hibernated:      t.Hibernated,
		})
	}
	m.mu.RUnlock()

	m.startMu.Lock()
	starting := make(map[string]bool, len(m.starting))
	for id, v := range m.starting {
		starting[id] = v
	}
	m.startMu.Unlock()

	for i := range views {
		id := views[i].ID
		switch {
		case views[i].Hibernated:
			views[i].Status = "hibernated"
			views[i].Healthy = false
		case running[id]:
			views[i].Status = "running"
			views[i].Healthy = true
		case starting[id]:
			views[i].Status = "starting"
			views[i].Healthy = false
		default:
			views[i].Status = "down"
			views[i].Healthy = false
		}
	}
	return views
}

func (m *Manager) StartTenant(t *Tenant) error {
	if t != nil && t.Hibernated {
		return fmt.Errorf("tenant %s is hibernated", t.ID)
	}
	m.startMu.Lock()
	if m.starting[t.ID] {
		m.startMu.Unlock()
		return nil
	}
	m.mu.RLock()
	_, alreadyRunning := m.instances[t.ID]
	_, alreadyWorker := m.workers[t.ID]
	m.mu.RUnlock()
	if alreadyRunning || alreadyWorker {
		m.startMu.Unlock()
		return nil
	}
	m.starting[t.ID] = true
	m.startMu.Unlock()
	defer func() {
		m.startMu.Lock()
		delete(m.starting, t.ID)
		m.startMu.Unlock()
	}()

	// Create data dir
	if err := os.MkdirAll(t.DataDir, 0700); err != nil {
		return err
	}

	profile := m.profile
	if profile.Mode == "" {
		profile = isolation.FromEnv()
	}

	var atRest *security.Encryptor
	var dek []byte
	if profile.Encryption {
		kek, err := isolation.LoadKEK()
		if err != nil {
			return err
		}
		dek, err = isolation.EnsureDEK(t.DataDir, kek)
		if err != nil {
			return err
		}
		atRest, err = isolation.NewEncryptor(dek)
		if err != nil {
			return err
		}
	}

	cfgObj := config.TenantEngine(t.DataDir, t.RESPPort, t.HTTPPort)
	cfgObj.Auth.ACLFile = isolation.ACLFile(t.DataDir)
	if profile.UnixIPC {
		cfgObj.Server.Socket = isolation.RESPSocket(t.DataDir)
		cfgObj.Server.HTTPSocket = isolation.HTTPSocket(t.DataDir)
		cfgObj.Server.PeerPIDs = []int{os.Getpid()}
	}
	role, listenAddr, primaryAddr := "", "", ""
	switch t.Role {
	case "primary":
		role = "primary"
		listenAddr = fmt.Sprintf("127.0.0.1:%d", t.ReplicationPort)
		if profile.UnixIPC {
			listenAddr = isolation.ReplSocket(t.DataDir)
		}
	case "replica":
		role = "replica"
		m.mu.RLock()
		if primary, ok := m.tenants[t.ReplicaOf]; ok {
			if profile.UnixIPC {
				primaryAddr = isolation.ReplSocket(primary.DataDir)
			} else if primary.ReplicationPort > 0 {
				primaryAddr = fmt.Sprintf("127.0.0.1:%d", primary.ReplicationPort)
			}
		}
		m.mu.RUnlock()
	}
	if err := config.ApplyReplication(cfgObj, role, listenAddr, primaryAddr); err != nil {
		return err
	}
	enginePath := filepath.Join(t.DataDir, "engine.yaml")
	if out, err := yaml.Marshal(cfgObj); err == nil {
		_ = os.WriteFile(enginePath, out, 0600)
	}
	m.mu.RLock()
	users := collectTenantUsers(m.tenants, t)
	m.mu.RUnlock()
	if err := writeUsersACL(t.DataDir, users); err != nil {
		return err
	}
	quota, err := config.ParseBytes(cfgObj.Engine.MaxMemory)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if _, reserved := m.tenantQuotas[t.ID]; !reserved {
		var allocated int64
		for _, value := range m.tenantQuotas {
			allocated += value
		}
		if m.nodeBudget > 0 && allocated+quota > m.nodeBudget*70/100 {
			m.mu.Unlock()
			return fmt.Errorf("node admission denied: tenant quotas would exceed 70%% of %d bytes", m.nodeBudget)
		}
		m.tenantQuotas[t.ID] = quota
	}
	m.mu.Unlock()

	if profile.Process {
		fmt.Printf("[Orchestrator] Starting isolated tenant %s (%s)\n", t.ID, profile)
		if err := m.startIsolatedWorker(t, enginePath, dek, quota); err != nil {
			return err
		}
		m.mu.RLock()
		worker := m.workers[t.ID]
		m.mu.RUnlock()
		go m.superviseWorker(t, worker)
		return nil
	}

	inst, err := server.NewInstance(cfgObj)
	if err != nil {
		return err
	}
	inst.SkipBuiltinUser()
	inst.SetTenantID(t.ID)
	if atRest != nil {
		inst.SetAtRest(atRest)
	}
	inst.SetInitialUsers(users)

	fmt.Printf("[Orchestrator] Starting tenant %s on HTTP:%d RESP:%d isolation=%s\n", t.ID, t.HTTPPort, t.RESPPort, profile.Mode)
	if err := inst.Start(context.Background()); err != nil {
		return err
	}
	m.mu.Lock()
	m.instances[t.ID] = inst
	users = users[:0]
	addLiveKeys := func(keys map[string]*TenantKey) {
		for _, key := range keys {
			if key != nil && !key.Revoked {
				users = append(users, tenantUser(key))
			}
		}
	}
	addLiveKeys(t.Keys)
	if t.Role == "replica" {
		if primary, ok := m.tenants[t.ReplicaOf]; ok {
			addLiveKeys(primary.Keys)
		}
	}
	m.mu.Unlock()
	for _, user := range users {
		inst.UpsertUser(user)
	}
	go m.superviseTenant(t, inst)
	return nil
}

func (m *Manager) superviseWorker(t *Tenant, worker *isolatedWorker) {
	if worker == nil {
		return
	}
	err := <-worker.ErrorChannel()
	if err == nil {
		return
	}
	m.mu.Lock()
	if t.Hibernated || m.workers[t.ID] != worker {
		m.mu.Unlock()
		return
	}
	delete(m.workers, t.ID)
	m.restarts[t.ID]++
	attempt := m.restarts[t.ID]
	m.mu.Unlock()
	worker.Stop()
	if attempt > 3 {
		fmt.Printf("[Orchestrator] tenant %s worker unhealthy after %d restart attempts: %v\n", t.ID, attempt-1, err)
		return
	}
	time.Sleep(time.Duration(attempt) * time.Second)
	if startErr := m.StartTenant(t); startErr != nil {
		fmt.Printf("[Orchestrator] tenant %s worker restart %d failed: %v\n", t.ID, attempt, startErr)
	}
}

func (m *Manager) superviseTenant(t *Tenant, inst *server.Instance) {
	err := <-inst.ErrorChannel()
	if err == nil {
		return
	}
	m.mu.Lock()
	if t.Hibernated || m.instances[t.ID] != inst {
		m.mu.Unlock()
		return
	}
	delete(m.instances, t.ID)
	m.restarts[t.ID]++
	attempt := m.restarts[t.ID]
	m.mu.Unlock()
	inst.Stop()
	if attempt > 3 {
		fmt.Printf("[Orchestrator] tenant %s unhealthy after %d restart attempts: %v\n", t.ID, attempt-1, err)
		return
	}
	time.Sleep(time.Duration(attempt) * time.Second)
	if startErr := m.StartTenant(t); startErr != nil {
		fmt.Printf("[Orchestrator] tenant %s restart %d failed: %v\n", t.ID, attempt, startErr)
	}
}

// Promote fails the public tenant over to replicaID. Ingress keeps AUTH'ing the
// original tenant id; only the data directory and loopback ports swap.
func (m *Manager) Promote(replicaID string) error {
	if m.RaftNode != nil {
		if m.RaftNode.Raft.State() != raft.Leader {
			return fmt.Errorf("promotion failed: not the leader")
		}
		cmd := fsmUpdateCommand{Action: "promote", ReplicaID: replicaID}
		data, err := json.Marshal(cmd)
		if err != nil {
			return err
		}
		return m.RaftNode.Raft.Apply(data, 10*time.Second).Error()
	}
	return m.promoteLocal(replicaID)
}

func (m *Manager) promoteLocal(replicaID string) error {
	m.mu.Lock()
	replica, ok := m.tenants[replicaID]
	if !ok || replica.Role != "replica" || replica.ReplicaOf == "" {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s is not a replica", replicaID)
	}
	primary, ok := m.tenants[replica.ReplicaOf]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("primary %s not found", replica.ReplicaOf)
	}
	stopIDs := append([]string{primary.ID}, primary.Replicas...)
	instances := make([]*server.Instance, 0, len(stopIDs))
	workers := make([]*isolatedWorker, 0, len(stopIDs))
	for _, id := range stopIDs {
		if inst := m.instances[id]; inst != nil {
			instances = append(instances, inst)
			delete(m.instances, id)
		}
		if worker := m.workers[id]; worker != nil {
			workers = append(workers, worker)
			delete(m.workers, id)
		}
	}
	primary.DataDir, replica.DataDir = replica.DataDir, primary.DataDir
	primary.HTTPPort, replica.HTTPPort = replica.HTTPPort, primary.HTTPPort
	primary.RESPPort, replica.RESPPort = replica.RESPPort, primary.RESPPort
	primary.ReplicationPort, replica.ReplicationPort = replica.ReplicationPort, primary.ReplicationPort
	replicas := make([]*Tenant, 0, len(primary.Replicas))
	for _, rid := range primary.Replicas {
		if rt, ok := m.tenants[rid]; ok {
			replicas = append(replicas, rt)
		}
	}
	saveErr := m.saveState()
	primaryCopy := primary
	hadEngines := len(instances) > 0 || len(workers) > 0
	m.mu.Unlock()

	for _, inst := range instances {
		inst.Stop()
	}
	for _, worker := range workers {
		worker.Stop()
	}
	if saveErr != nil {
		return saveErr
	}
	if !hadEngines {
		return nil
	}
	if err := m.StartTenant(primaryCopy); err != nil {
		return err
	}
	for _, r := range replicas {
		if err := m.StartTenant(r); err != nil {
			fmt.Printf("[Orchestrator] replica %s failed to start after promote: %v\n", r.ID, err)
		}
	}
	return nil
}

// StopAll performs one ordered process-wide tenant shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	instances := make([]*server.Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	workers := make([]*isolatedWorker, 0, len(m.workers))
	for _, worker := range m.workers {
		workers = append(workers, worker)
	}
	m.instances = make(map[string]*server.Instance)
	m.workers = make(map[string]*isolatedWorker)
	m.mu.Unlock()
	for _, inst := range instances {
		inst.Stop()
	}
	for _, worker := range workers {
		worker.Stop()
	}
}
