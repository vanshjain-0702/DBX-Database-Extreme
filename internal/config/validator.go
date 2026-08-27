package config

import "fmt"

// Validate checks the config for semantic errors.
func Validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("config: invalid server port %d", cfg.Server.Port)
	}
	if cfg.Auth.Enabled && cfg.Auth.RequirePassword && cfg.Auth.DefaultUser == "" {
		return fmt.Errorf("config: default_user is required when password authentication is enabled")
	}
	switch cfg.Engine.EvictionPolicy {
	case "no-eviction":
		// v1 never discards tenant data to satisfy an admission request.
	case "lru", "lfu", "ttl", "random":
		return fmt.Errorf("config: eviction policy %q is not supported by the production v1 profile; use no-eviction", cfg.Engine.EvictionPolicy)
	default:
		return fmt.Errorf("config: unknown eviction policy %q", cfg.Engine.EvictionPolicy)
	}
	if _, err := ParseBytes(cfg.Engine.MaxMemory); err != nil {
		return fmt.Errorf("config: max_memory: %w", err)
	}
	switch cfg.Persistence.WALSync {
	case "always", "everysec":
		// valid
	case "no":
		return fmt.Errorf("config: wal_sync=no is not supported by the production v1 profile")
	default:
		return fmt.Errorf("config: unknown wal_sync value %q", cfg.Persistence.WALSync)
	}
	switch cfg.Replication.Role {
	case "primary", "replica", "":
		// valid
	default:
		return fmt.Errorf("config: unknown replication role %q", cfg.Replication.Role)
	}
	if cfg.Replication.RaftEnabled {
		return fmt.Errorf("config: data-plane Raft is not supported; replicate WAL bytes asynchronously instead")
	}
	switch cfg.Replication.Role {
	case "":
		if cfg.Replication.ListenAddr != "" || cfg.Replication.PrimaryAddr != "" {
			return fmt.Errorf("config: listen_addr and primary_addr require role primary or replica")
		}
	case "primary":
		if cfg.Replication.ListenAddr == "" {
			return fmt.Errorf("config: listen_addr is required for primary role")
		}
	case "replica":
		if cfg.Replication.PrimaryAddr == "" {
			return fmt.Errorf("config: primary_addr is required for replica role")
		}
	}
	if cfg.Cluster.Enabled {
		return fmt.Errorf("config: cluster mode is not supported by the single-node v1 profile")
	}
	if cfg.Tiering.Enabled {
		return fmt.Errorf("config: tiering is not supported by the single-node v1 profile")
	}
	if cfg.Cluster.SlotCount <= 0 {
		cfg.Cluster.SlotCount = 16384
	}
	return nil
}
