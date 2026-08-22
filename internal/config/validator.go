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
	case "lru", "lfu", "ttl", "random", "no-eviction":
		// valid
	default:
		return fmt.Errorf("config: unknown eviction policy %q", cfg.Engine.EvictionPolicy)
	}
	switch cfg.Persistence.WALSync {
	case "always", "everysec", "no":
		// valid
	default:
		return fmt.Errorf("config: unknown wal_sync value %q", cfg.Persistence.WALSync)
	}
	switch cfg.Replication.Role {
	case "primary", "replica", "":
		// valid
	default:
		return fmt.Errorf("config: unknown replication role %q", cfg.Replication.Role)
	}
	if cfg.Replication.Role == "replica" && cfg.Replication.PrimaryAddr == "" {
		return fmt.Errorf("config: primary_addr is required for replica role")
	}
	if cfg.Cluster.SlotCount <= 0 {
		cfg.Cluster.SlotCount = 16384
	}
	return nil
}
