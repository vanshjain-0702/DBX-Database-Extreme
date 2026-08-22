package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/server"
	"github.com/hashicorp/raft"
	"gopkg.in/yaml.v3"
)

type Tenant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	HTTPPort int    `json:"http_port"`
	RESPPort int    `json:"resp_port"`
	GRPCPort int    `json:"grpc_port"`
	RaftPort int    `json:"raft_port"`
	DataDir  string `json:"data_dir"`
}

type Manager struct {
	mu           sync.RWMutex
	tenants      map[string]*Tenant
	stateFile    string
	nextHTTPPort int
	nextRESPPort int
	nextGRPCPort int
	nextRaftPort int
	instances    map[string]*server.Instance
	startMu      sync.Mutex
	starting     map[string]bool
	RaftNode     *RaftNode
}

func NewManager(stateFile string) (*Manager, error) {
	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    stateFile,
		nextHTTPPort: 8081,
		nextRESPPort: 6401,
		nextGRPCPort: 9091,
		nextRaftPort: 10001,
		instances:    make(map[string]*server.Instance),
		starting:     make(map[string]bool),
	}
	if err := m.loadState(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// Resume existing processes
	for _, t := range m.tenants {
		if t.HTTPPort >= m.nextHTTPPort {
			m.nextHTTPPort = t.HTTPPort + 1
		}
		if t.RESPPort >= m.nextRESPPort {
			m.nextRESPPort = t.RESPPort + 1
		}
		if t.GRPCPort >= m.nextGRPCPort {
			m.nextGRPCPort = t.GRPCPort + 1
		}
		if t.RaftPort == 0 {
			t.RaftPort = m.nextRaftPort
		}
		if t.RaftPort >= m.nextRaftPort {
			m.nextRaftPort = t.RaftPort + 1
		}
		
		// Ensure no port collisions in state.json
		if t.HTTPPort == 0 { t.HTTPPort = m.nextHTTPPort; m.nextHTTPPort++ }
		if t.RESPPort == 0 { t.RESPPort = m.nextRESPPort; m.nextRESPPort++ }
		if t.GRPCPort == 0 { t.GRPCPort = m.nextGRPCPort; m.nextGRPCPort++ }

		go m.StartTenant(t)
	}
	return m, nil
}

func (m *Manager) loadState() error {
	b, err := os.ReadFile(m.stateFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &m.tenants)
}

func (m *Manager) saveState() error {
	b, err := json.MarshalIndent(m.tenants, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.stateFile, b, 0644)
}

func (m *Manager) Provision(id, name string) (*Tenant, error) {
	if id == "" {
		return nil, fmt.Errorf("invalid tenant id")
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return nil, fmt.Errorf("invalid tenant id: only alphanumeric and dashes allowed")
		}
	}
	if name == "" {
		return nil, fmt.Errorf("tenant name is required")
	}
	m.mu.Lock()
	if _, exists := m.tenants[id]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("tenant already exists")
	}

	t := &Tenant{
		ID:       id,
		Name:     name,
		HTTPPort: m.nextHTTPPort,
		RESPPort: m.nextRESPPort,
		GRPCPort: m.nextGRPCPort,
		RaftPort: m.nextRaftPort,
		DataDir:  fmt.Sprintf("./data/tenants/%s", id),
	}
	m.nextHTTPPort++
	m.nextRESPPort++
	m.nextGRPCPort++
	m.nextRaftPort++
	m.mu.Unlock()

	if m.RaftNode != nil {
		if m.RaftNode.Raft.State() != raft.Leader {
			return nil, fmt.Errorf("provisioning failed: not the leader")
		}
		cmd := fsmUpdateCommand{Action: "provision", Tenant: t}
		data, _ := json.Marshal(cmd)
		future := m.RaftNode.Raft.Apply(data, 10*time.Second)
		if err := future.Error(); err != nil {
			return nil, err
		}
		return t, nil
	} else {
		// Fallback to local mutation if Raft is not configured
		m.mu.Lock()
		m.tenants[id] = t
		err := m.saveState() // Call inside the lock to prevent race condition
		m.mu.Unlock()
		if err != nil {
			return nil, err
		}
		go m.StartTenant(t)
	}
	return t, nil
}

func (m *Manager) GetTenant(id string) (*Tenant, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[id]
	return t, ok
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

func (m *Manager) StartTenant(t *Tenant) error {
	m.startMu.Lock()
	if m.starting[t.ID] {
		m.startMu.Unlock()
		return nil
	}
	m.mu.RLock()
	_, alreadyRunning := m.instances[t.ID]
	m.mu.RUnlock()
	if alreadyRunning {
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
	if err := os.MkdirAll(t.DataDir, 0755); err != nil {
		return err
	}

	// Generate config
	cfgPath := fmt.Sprintf("./configs/tenant-%s.yaml", t.ID)

	// Read template (we'll just use the default configs/local.yaml)
	b, err := os.ReadFile("./configs/local.yaml")
	if err != nil {
		return err
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return err
	}

	// Override ports and paths
	if server, ok := cfg["server"].(map[string]interface{}); ok {
		server["port"] = t.RESPPort
		server["http_port"] = t.HTTPPort
		server["grpc_port"] = t.GRPCPort
	}
	if persistence, ok := cfg["persistence"].(map[string]interface{}); ok {
		persistence["data_dir"] = t.DataDir
		persistence["wal_dir"] = filepath.Join(t.DataDir, "wal")
		persistence["snapshot_dir"] = filepath.Join(t.DataDir, "snapshots")
		persistence["backup_dir"] = filepath.Join(t.DataDir, "backups")
	}
	if replication, ok := cfg["replication"].(map[string]interface{}); ok {
		replication["raft_enabled"] = true
		replication["raft_node_id"] = t.ID
		replication["raft_bind_addr"] = fmt.Sprintf("127.0.0.1:%d", t.RaftPort)
		replication["raft_dir"] = filepath.Join(t.DataDir, "raft")
		replication["raft_bootstrap"] = true
	}

	// Write new config
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		return err
	}

	// Load config struct
	cfgObj, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	inst, err := server.NewInstance(cfgObj)
	if err != nil {
		return err
	}

	fmt.Printf("[Orchestrator] Starting tenant %s on HTTP:%d RESP:%d\n", t.ID, t.HTTPPort, t.RESPPort)
	if err := inst.Start(context.Background()); err != nil {
		return err
	}
	m.mu.Lock()
	m.instances[t.ID] = inst
	m.mu.Unlock()
	return nil
}
