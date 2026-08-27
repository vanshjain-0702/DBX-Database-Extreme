package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/api"
	"github.com/dbx/dbx/internal/auth"
	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/events"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/query"
	"github.com/dbx/dbx/internal/replication"
	"github.com/dbx/dbx/internal/security"
	"github.com/dbx/dbx/internal/transaction"
	"github.com/hashicorp/raft"
)

type Instance struct {
	cfg           *config.Config
	logger        *observability.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	respServer    *api.RESPServer
	httpServer    *api.HTTPServer
	wal           *persistence.WAL
	snapshotter   *persistence.Snapshotter
	kv            *engine.KVStore
	vecStore      *engine.VectorStore
	executor      *query.Executor
	auditGuard    *security.AuditGuard
	primaryStream *replication.PrimaryStream
	replicaStream *replication.ReplicaStream
	serverErr     chan error
	metrics       *observability.Metrics
	stopOnce      sync.Once
	aclStore      *auth.ACLStore
	initialUsers  []*auth.User
}

func NewInstance(cfg *config.Config) (*Instance, error) {
	logger := observability.NewLogger(cfg.Observability.Logging.Level, cfg.Observability.Logging.Format)
	return &Instance{
		cfg:    cfg,
		logger: logger,
	}, nil
}

func (i *Instance) Start(ctx context.Context) error {
	i.ctx, i.cancel = context.WithCancel(ctx)
	cfg := i.cfg
	logger := i.logger
	metrics := &observability.Metrics{}
	i.metrics = metrics
	i.serverErr = make(chan error, 1)

	numShards := cfg.Engine.NumShards
	if numShards <= 0 {
		numShards = 256
	}
	kv := engine.New(numShards)
	i.kv = kv
	vecStore := engine.NewVectorStore(kv, cfg.Persistence.DataDir, cfg.Engine.MaxVectorsPerTenant)
	i.vecStore = vecStore
	logger.Info("Engine initialized with %d shards", numShards)

	var wal *persistence.WAL
	var snapshotter *persistence.Snapshotter
	if cfg.Persistence.Enabled {
		var err error
		wal, err = persistence.OpenWAL(cfg.Persistence.WALDir, cfg.Persistence.WALSync, 64)
		if err != nil {
			return fmt.Errorf("WAL init failed: %v", err)
		}
		i.wal = wal
		snapshotter = persistence.NewSnapshotter(cfg.Persistence.SnapshotDir)
		i.snapshotter = snapshotter
		recovery := persistence.NewRecovery(wal, snapshotter)
		if err := recovery.Recover(kv, vecStore); err != nil {
			_ = wal.Close()
			i.wal = nil
			i.snapshotter = nil
			return fmt.Errorf("recovery failed: %w", err)
		} else {
			logger.Info("Recovery complete, %d keys loaded", kv.DBSize())
		}
	}

	mvcc := transaction.NewMVCCStore(64)
	watch := transaction.NewWatchSet()
	multi := transaction.NewMultiManager()

	aclStore := auth.NewACLStore()
	aclStore.DisableDefault()
	if cfg.Auth.Enabled && cfg.Auth.RequirePassword {
		password := os.Getenv("DBX_DEFAULT_PASSWORD")
		if password != "" {
			aclStore.SetDefaultPassword(cfg.Auth.DefaultUser, password)
		}
	}
	for _, user := range i.initialUsers {
		aclStore.AddUser(user)
	}
	i.aclStore = aclStore
	enforcer := security.NewACLEnforcer(aclStore)
	rateLimit := security.NewRateLimiter(
		cfg.Security.RateLimit.RequestsPerSecond,
		cfg.Security.RateLimit.Burst,
		cfg.Security.RateLimit.Enabled,
	)
	auditPath := cfg.Persistence.DataDir + "/audit.log"
	auditGuard, err := security.NewAuditGuard(auditPath, true)
	if err != nil {
		logger.Warn("Audit log init failed: %v", err)
		auditGuard, _ = security.NewAuditGuard("", false)
	}
	i.auditGuard = auditGuard

	pubsub := events.NewPubSub(
		cfg.Events.PubSub.MaxChannels,
		cfg.Events.PubSub.MaxSubscribersPerChannel,
	)

	executor := query.NewExecutor(kv, vecStore, multi, watch, mvcc, pubsub, metrics, wal)
	i.executor = executor
	if cfg.Replication.Role == "replica" {
		executor.SetReadOnly(true)
	}
	maxMemory, err := config.ParseBytes(cfg.Engine.MaxMemory)
	if err != nil {
		return err
	}
	executor.SetMemoryLimit(maxMemory)
	vecStore.SetMemoryLimit(maxMemory)
	metrics.TenantMemoryLimit.Store(maxMemory)
	metrics.TenantReady.Store(1)

	// Raft Integration for Data Plane
	if cfg.Replication.RaftEnabled {
		logger.Info("Initializing Raft consensus for data plane on %s", cfg.Replication.RaftBindAddr)

		if err := os.MkdirAll(cfg.Replication.RaftDir, 0700); err != nil {
			return fmt.Errorf("raft dir error: %w", err)
		}

		raftCfg := raft.DefaultConfig()
		raftCfg.LocalID = raft.ServerID(cfg.Replication.RaftNodeID)

		addr, err := net.ResolveTCPAddr("tcp", cfg.Replication.RaftBindAddr)
		if err != nil {
			return fmt.Errorf("raft resolve addr error: %w", err)
		}
		transport, err := raft.NewTCPTransport(cfg.Replication.RaftBindAddr, addr, 3, 10*time.Second, os.Stderr)
		if err != nil {
			return fmt.Errorf("raft transport error: %w", err)
		}

		snapshots, err := raft.NewFileSnapshotStore(cfg.Replication.RaftDir, 2, os.Stderr)
		if err != nil {
			return fmt.Errorf("raft snapshot error: %w", err)
		}

		// In-memory store for log and stable store (since we persist via WAL independently,
		// but Raft requires its own log. In a real system we use boltDB)
		// For MVP, we'll just use the memory store to avoid adding more boltDB dependencies.
		logStore := raft.NewInmemStore()
		stableStore := raft.NewInmemStore()

		fsm := query.NewEngineFSM(executor, snapshotter)

		r, err := raft.NewRaft(raftCfg, fsm, logStore, stableStore, snapshots, transport)
		if err != nil {
			return fmt.Errorf("raft init error: %w", err)
		}

		if cfg.Replication.RaftBootstrap {
			r.BootstrapCluster(raft.Configuration{
				Servers: []raft.Server{
					{
						ID:      raft.ServerID(cfg.Replication.RaftNodeID),
						Address: raft.ServerAddress(cfg.Replication.RaftBindAddr),
					},
				},
			})
		}

		// Wire raft into the executor
		executor.SetRaft(&raftBridge{node: r})
	}

	if wal != nil && cfg.Replication.Role == "primary" && cfg.Replication.ListenAddr != "" && !cfg.Replication.RaftEnabled {
		primary := replication.NewPrimaryStream()
		if err := primary.Start(cfg.Replication.ListenAddr, wal); err != nil {
			_ = wal.Close()
			return fmt.Errorf("replication listener failed: %w", err)
		}
		wal.Subscribe(primary.BroadcastRecord)
		i.primaryStream = primary
	}
	if cfg.Replication.Role == "replica" {
		replica := replication.NewReplicaStream(cfg.Replication.PrimaryAddr, executor)
		replica.Start()
		i.replicaStream = replica
	}

	go func() {
		defer i.recoverTenantTask("expiry")
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := kv.DeleteExpiredLimit(1000)
				if n > 0 {
					metrics.TotalExpired.Add(int64(n))
				}
			case <-i.ctx.Done():
				return
			}
		}
	}()

	if cfg.Persistence.Enabled && snapshotter != nil {
		go func() {
			defer i.recoverTenantTask("checkpoint")
			interval := cfg.Persistence.SnapshotInterval
			if interval == 0 {
				interval = time.Hour
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					path, err := executor.Checkpoint(snapshotter)
					if err != nil {
						logger.Error("Snapshot failed: %v", err)
					} else {
						logger.Info("Snapshot saved to %s", path)
					}
				case <-i.ctx.Done():
					return
				}
			}
		}()
	}

	if cfg.Persistence.WALSync == "everysec" && wal != nil {
		go func() {
			defer i.recoverTenantTask("wal-sync")
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := wal.Sync(); err != nil {
						metrics.TenantReady.Store(0)
					}
				case <-i.ctx.Done():
					return
				}
			}
		}()
	}

	// WAL size threshold monitor — trigger snapshot when WAL grows beyond threshold
	if cfg.Persistence.Enabled && wal != nil && snapshotter != nil {
		go func() {
			defer i.recoverTenantTask("wal-monitor")
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if wal.NeedsRotation() {
						logger.Info("WAL exceeded size threshold, triggering emergency snapshot...")
						if path, err := executor.Checkpoint(snapshotter); err != nil {
							logger.Error("Emergency snapshot failed: %v", err)
						} else {
							logger.Info("Emergency snapshot saved: %s", path)
						}
					}
				case <-i.ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer i.recoverTenantTask("rate-limit-cleanup")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rateLimit.Cleanup(10 * time.Minute)
			case <-i.ctx.Done():
				return
			}
		}
	}()

	i.respServer = api.NewRESPServer(
		&cfg.Server, &cfg.Security.TLS, executor, aclStore, enforcer, rateLimit, auditGuard, metrics, logger,
	)
	i.httpServer = api.NewHTTPServer(&cfg.Server, metrics, executor, os.Getenv("DBX_INTERNAL_API_TOKEN"), enforcer, auditGuard)

	go func() {
		defer i.recoverTenantTask("http-server")
		if err := i.httpServer.ListenAndServe(i.ctx); err != nil {
			logger.Info("HTTP server stopped: %v", err)
		}
	}()

	go func() {
		defer i.recoverTenantTask("resp-server")
		i.serverErr <- i.respServer.ListenAndServe(i.ctx)
	}()

	return nil
}

func (i *Instance) recoverTenantTask(name string) {
	if recovered := recover(); recovered != nil {
		if i.metrics != nil {
			i.metrics.TenantReady.Store(0)
		}
		err := fmt.Errorf("tenant task %s panicked: %v", name, recovered)
		i.logger.Error("%v", err)
		select {
		case i.serverErr <- err:
		default:
		}
	}
}

func (i *Instance) Stop() {
	i.stopOnce.Do(func() {
		if i.metrics != nil {
			i.metrics.TenantReady.Store(0)
		}
		if i.cancel != nil {
			i.cancel()
		}
		if i.respServer != nil {
			i.respServer.Shutdown()
		}
		if i.replicaStream != nil {
			i.replicaStream.Stop()
		}
		if i.primaryStream != nil {
			i.primaryStream.Stop()
		}
		if i.cfg.Persistence.Enabled && i.snapshotter != nil && i.executor != nil {
			if path, err := i.executor.Checkpoint(i.snapshotter); err == nil {
				i.logger.Info("Final snapshot saved: %s", path)
			}
		}
		if i.vecStore != nil {
			i.vecStore.CloseAll()
		}
		if i.wal != nil {
			i.wal.Close()
		}
		if i.auditGuard != nil {
			i.auditGuard.Flush()
			i.auditGuard.Close()
		}
		i.logger.Info("DBX instance shutdown complete")
	})
}

func (i *Instance) ErrorChannel() <-chan error {
	return i.serverErr
}

// SetInitialUsers installs tenant-scoped credentials before listeners start.
func (i *Instance) SetInitialUsers(users []*auth.User) {
	i.initialUsers = append([]*auth.User(nil), users...)
}

// UpsertUser updates a live tenant credential.
func (i *Instance) UpsertUser(user *auth.User) {
	if i.aclStore != nil {
		i.aclStore.AddUser(user)
	}
}

// DeleteUser revokes a credential for new and existing connections.
func (i *Instance) DeleteUser(name string) {
	if i.aclStore != nil {
		i.aclStore.DeleteUser(name)
	}
}

// CreateBackup captures a mutation-consistent, checksummed tenant archive.
func (i *Instance) CreateBackup(tenantID, outputPath string) (persistence.BackupManifest, error) {
	var manifest persistence.BackupManifest
	if i.executor == nil || i.snapshotter == nil {
		return manifest, fmt.Errorf("tenant persistence is unavailable")
	}
	err := i.executor.WithMaintenanceCheckpoint(i.snapshotter, func(sequence uint64, snapshotPath string) error {
		var err error
		manifest, err = persistence.CreateBackupArchive(
			tenantID, i.cfg.Persistence.DataDir, snapshotPath, outputPath, sequence,
		)
		return err
	})
	return manifest, err
}
