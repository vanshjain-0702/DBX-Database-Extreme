// Package config handles DBX server configuration.
package config

import "time"

// Config is the root configuration structure for DBX.
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Engine        EngineConfig        `yaml:"engine"`
	Persistence   PersistenceConfig   `yaml:"persistence"`
	Replication   ReplicationConfig   `yaml:"replication"`
	Cluster       ClusterConfig       `yaml:"cluster"`
	Auth          AuthConfig          `yaml:"auth"`
	Security      SecurityConfig      `yaml:"security"`
	Policy        PolicyConfig        `yaml:"policy"`
	Tiering       TieringConfig       `yaml:"tiering"`
	Observability ObservabilityConfig `yaml:"observability"`
	Workflow      WorkflowConfig      `yaml:"workflow"`
	Events        EventsConfig        `yaml:"events"`
}

// ServerConfig holds TCP/HTTP/gRPC listener settings.
type ServerConfig struct {
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	HTTPPort       int           `yaml:"http_port"`
	GRPCPort       int           `yaml:"grpc_port"`
	Socket         string        `yaml:"socket"`
	HTTPSocket     string        `yaml:"http_socket"`
	MaxConnections int           `yaml:"max_connections"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	PeerPIDs       []int         `yaml:"-"`
}

// EngineConfig holds in-memory engine settings.
type EngineConfig struct {
	MaxMemory           string `yaml:"max_memory"`
	EvictionPolicy      string `yaml:"eviction_policy"`
	DefaultTTL          int64  `yaml:"default_ttl"`
	NumShards           int    `yaml:"num_shards"`
	MaxVectorsPerTenant int    `yaml:"max_vectors_per_tenant"`
}

// PersistenceConfig holds WAL/snapshot settings.
type PersistenceConfig struct {
	Enabled            bool          `yaml:"enabled"`
	DataDir            string        `yaml:"data_dir"`
	WALDir             string        `yaml:"wal_dir"`
	SnapshotDir        string        `yaml:"snapshot_dir"`
	BackupDir          string        `yaml:"backup_dir"`
	WALSync            string        `yaml:"wal_sync"` // "always", "everysec", "no"
	SnapshotInterval   time.Duration `yaml:"snapshot_interval"`
	CompactionInterval time.Duration `yaml:"compaction_interval"`
}

// ReplicationConfig holds replication topology settings.
type ReplicationConfig struct {
	Role         string        `yaml:"role"` // "primary" or "replica"
	ListenAddr   string        `yaml:"listen_addr"`
	PrimaryAddr  string        `yaml:"primary_addr"`
	Replicas     []string      `yaml:"replicas"`
	Quorum       int           `yaml:"quorum"`
	SyncTimeout  time.Duration `yaml:"sync_timeout"`
	LagThreshold time.Duration `yaml:"lag_threshold"`

	// Raft configuration
	RaftEnabled   bool   `yaml:"raft_enabled"`
	RaftNodeID    string `yaml:"raft_node_id"`
	RaftBindAddr  string `yaml:"raft_bind_addr"`
	RaftDir       string `yaml:"raft_dir"`
	RaftBootstrap bool   `yaml:"raft_bootstrap"`
	RaftJoinAddr  string `yaml:"raft_join_addr"`
}

// ClusterConfig holds distributed cluster settings.
type ClusterConfig struct {
	Enabled           bool          `yaml:"enabled"`
	NodeID            string        `yaml:"node_id"`
	Nodes             []string      `yaml:"nodes"`
	SlotCount         int           `yaml:"slot_count"`
	RebalanceInterval time.Duration `yaml:"rebalance_interval"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	Enabled         bool   `yaml:"enabled"`
	DefaultUser     string `yaml:"default_user"`
	RequirePassword bool   `yaml:"require_password"`
	ACLFile         string `yaml:"acl_file"`
}

// SecurityConfig holds security policy settings.
type SecurityConfig struct {
	RateLimit  RateLimitConfig  `yaml:"rate_limit"`
	Encryption EncryptionConfig `yaml:"encryption"`
	TLS        TLSConfig        `yaml:"tls"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second"`
	Burst             int  `yaml:"burst"`
}

type EncryptionConfig struct {
	Enabled bool   `yaml:"enabled"`
	Key     string `yaml:"key"`
}

type TLSConfig struct {
	Enabled           bool   `yaml:"enabled"`
	CertFile          string `yaml:"cert_file"`
	KeyFile           string `yaml:"key_file"`
	CAFile            string `yaml:"ca_file"`
	RequireClientCert bool   `yaml:"require_client_cert"`
}

// PolicyConfig holds eviction/retention policies.
type PolicyConfig struct {
	Eviction         string          `yaml:"eviction"`
	MaxMemorySamples int             `yaml:"max_memory_samples"`
	Lazyfree         bool            `yaml:"lazyfree"`
	Retention        RetentionConfig `yaml:"retention"`
}

type RetentionConfig struct {
	Default string `yaml:"default"`
}

// TieringConfig holds hot/cold tiering settings.
type TieringConfig struct {
	Enabled     bool   `yaml:"enabled"`
	NVMEPath    string `yaml:"nvme_path"`
	ThresholdMB int    `yaml:"threshold_mb"`
}

// ObservabilityConfig holds metrics/tracing/logging settings.
type ObservabilityConfig struct {
	Metrics   MetricsConfig   `yaml:"metrics"`
	Tracing   TracingConfig   `yaml:"tracing"`
	Logging   LoggingConfig   `yaml:"logging"`
	Profiling ProfilingConfig `yaml:"profiling"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

type TracingConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type ProfilingConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// WorkflowConfig holds workflow engine settings.
type WorkflowConfig struct {
	Enabled      bool          `yaml:"enabled"`
	MaxRetries   int           `yaml:"max_retries"`
	RetryBackoff time.Duration `yaml:"retry_backoff"`
	DLQMaxSize   int           `yaml:"dlq_max_size"`
}

// EventsConfig holds pub/sub and stream settings.
type EventsConfig struct {
	PubSub  PubSubConfig  `yaml:"pubsub"`
	Streams StreamsConfig `yaml:"streams"`
}

type PubSubConfig struct {
	MaxChannels              int `yaml:"max_channels"`
	MaxSubscribersPerChannel int `yaml:"max_subscribers_per_channel"`
}

type StreamsConfig struct {
	MaxLength int    `yaml:"max_length"`
	Retention string `yaml:"retention"`
}
