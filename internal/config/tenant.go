package config

import "path/filepath"

// TenantEngine returns the production v1 profile for one tenant engine.
// It does not read configs/local.yaml, so Docker and Kubernetes can start
// tenants without a writable checkout of the source tree.
func TenantEngine(dataDir string, respPort, httpPort int) *Config {
	cfg := Defaults()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = respPort
	cfg.Server.HTTPPort = httpPort
	cfg.Server.GRPCPort = 0
	cfg.Engine.EvictionPolicy = "no-eviction"
	cfg.Persistence.Enabled = true
	cfg.Persistence.DataDir = dataDir
	cfg.Persistence.WALDir = filepath.Join(dataDir, "wal")
	cfg.Persistence.SnapshotDir = filepath.Join(dataDir, "snapshots")
	cfg.Persistence.BackupDir = filepath.Join(dataDir, "backups")
	cfg.Persistence.WALSync = "everysec"
	cfg.Replication.Role = ""
	cfg.Replication.ListenAddr = ""
	cfg.Replication.PrimaryAddr = ""
	cfg.Replication.RaftEnabled = false
	cfg.Replication.RaftBootstrap = false
	cfg.Cluster.Enabled = false
	cfg.Tiering.Enabled = false
	cfg.Security.RateLimit.Enabled = false
	cfg.Observability.Logging.Level = "info"
	return cfg
}

// ApplyReplication overlays async WAL roles. Raft stays disabled.
func ApplyReplication(cfg *Config, role, listenAddr, primaryAddr string) error {
	cfg.Replication.Role = role
	cfg.Replication.ListenAddr = listenAddr
	cfg.Replication.PrimaryAddr = primaryAddr
	cfg.Replication.RaftEnabled = false
	cfg.Replication.RaftBootstrap = false
	return Validate(cfg)
}
