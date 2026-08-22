package config

import "time"

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host:           "127.0.0.1",
			Port:           6399,
			HTTPPort:       8080,
			GRPCPort:       9090,
			MaxConnections: 10000,
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
		},
		Engine: EngineConfig{
			MaxMemory:      "256mb",
			EvictionPolicy: "lru",
			DefaultTTL:     0,
			NumShards:      256,
		},
		Persistence: PersistenceConfig{
			Enabled:            true,
			DataDir:            "./data",
			WALDir:             "./data/wal",
			SnapshotDir:        "./data/snapshots",
			BackupDir:          "./data/backups",
			WALSync:            "everysec",
			SnapshotInterval:   1 * time.Hour,
			CompactionInterval: 24 * time.Hour,
		},
		Replication: ReplicationConfig{
			Role:         "primary",
			Quorum:       1,
			SyncTimeout:  5 * time.Second,
			LagThreshold: 10 * time.Second,
		},
		Cluster: ClusterConfig{
			Enabled:           false,
			NodeID:            "node-01",
			SlotCount:         16384,
			RebalanceInterval: 5 * time.Minute,
		},
		Auth: AuthConfig{
			Enabled:         true,
			DefaultUser:     "default",
			RequirePassword: true,
		},
		Security: SecurityConfig{
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerSecond: 10000,
				Burst:             1000,
			},
			TLS: TLSConfig{
				Enabled:           false,
				CertFile:          "./certs/server.crt",
				KeyFile:           "./certs/server.key",
				CAFile:            "./certs/ca.crt",
				RequireClientCert: true,
			},
		},
		Policy: PolicyConfig{
			Eviction:         "lru",
			MaxMemorySamples: 5,
			Lazyfree:         true,
			Retention:        RetentionConfig{Default: "7d"},
		},
		Tiering: TieringConfig{
			Enabled:     false,
			NVMEPath:    "./data/nvme",
			ThresholdMB: 256,
		},
		Observability: ObservabilityConfig{
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    2112,
				Path:    "/metrics",
			},
			Logging: LoggingConfig{
				Level:  "info",
				Format: "json",
			},
		},
		Workflow: WorkflowConfig{
			Enabled:      true,
			MaxRetries:   3,
			RetryBackoff: time.Second,
			DLQMaxSize:   10000,
		},
		Events: EventsConfig{
			PubSub: PubSubConfig{
				MaxChannels:              1000,
				MaxSubscribersPerChannel: 1000,
			},
			Streams: StreamsConfig{
				MaxLength: 100000,
				Retention: "7d",
			},
		},
	}
}
