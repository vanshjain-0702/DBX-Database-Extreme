package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dbx/dbx/internal/api"
	"github.com/dbx/dbx/internal/auth"
	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/events"
	"github.com/hashicorp/raft"
	"net"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/query"
	"github.com/dbx/dbx/internal/replication"
	"github.com/dbx/dbx/internal/security"
	"github.com/dbx/dbx/internal/transaction"
)

const (
	shutdownTimeout     = 5 * time.Second
	walSnapshotThreshold = 64 * 1024 * 1024 // 64MB — trigger snapshot when WAL exceeds this
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
	auditGuard    *security.AuditGuard
	primaryStream *replication.PrimaryStream
	replicaStream *replication.ReplicaStream
	serverErr     chan error
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
	metrics := observability.Global

	numShards := cfg.Engine.NumShards
	if numShards <= 0 {
		numShards = 256
	}
	kv := engine.New(numShards)
	i.kv = kv
	vecStore := engine.NewVectorStore(kv, cfg.Persistence.DataDir, cfg.Engine.MaxVectorsPerTenant)
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
	if cfg.Auth.Enabled && cfg.Auth.RequirePassword {
		password := os.Getenv("DBX_DEFAULT_PASSWORD")
		if password == "" {
			return fmt.Errorf("DBX_DEFAULT_PASSWORD must be set when password authentication is enabled")
		}
		aclStore.SetDefaultPassword(cfg.Auth.DefaultUser, password)
	}
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
		executor.SetRaft(r)
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
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := kv.DeleteExpired()
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
			interval := cfg.Persistence.SnapshotInterval
			if interval == 0 {
				interval = time.Hour
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					path, err := snapshotter.Save(kv)
					if err != nil {
						logger.Error("Snapshot failed: %v", err)
					} else {
						logger.Info("Snapshot saved to %s", path)
						if wal != nil {
							wal.Rotate()
						}
					}
				case <-i.ctx.Done():
					return
				}
			}
		}()
	}

	if cfg.Persistence.WALSync == "everysec" && wal != nil {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					wal.Sync()
				case <-i.ctx.Done():
					return
				}
			}
		}()
	}

	// WAL size threshold monitor — trigger snapshot when WAL grows beyond threshold
	if cfg.Persistence.Enabled && wal != nil && snapshotter != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if wal.NeedsRotation() {
						logger.Info("WAL exceeded size threshold, triggering emergency snapshot...")
						if path, err := snapshotter.Save(kv); err != nil {
							logger.Error("Emergency snapshot failed: %v", err)
						} else {
							logger.Info("Emergency snapshot saved: %s", path)
							wal.Rotate()
							compactor := persistence.NewCompactor(cfg.Persistence.WALDir)
							if n, err := compactor.Compact(); err == nil && n > 0 {
								logger.Info("Compacted %d old WAL segments", n)
							}
						}
					}
				case <-i.ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
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
		if err := i.httpServer.ListenAndServe(i.ctx); err != nil {
			logger.Info("HTTP server stopped: %v", err)
		}
	}()

	i.serverErr = make(chan error, 1)
	go func() {
		i.serverErr <- i.respServer.ListenAndServe(i.ctx)
	}()

	// Signal handler for graceful shutdown on crash/kill
	go i.handleSignals()

	return nil
}

// handleSignals listens for OS signals and performs emergency shutdown.
func (i *Instance) handleSignals() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	select {
	case sig := <-sigCh:
		i.logger.Info("Received signal %v, initiating graceful shutdown...", sig)
		i.emergencyShutdown()
	case <-i.ctx.Done():
		return
	}
}

// emergencyShutdown flushes WAL, takes a final snapshot, then exits.
func (i *Instance) emergencyShutdown() {
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Step 1: Flush WAL to disk
		if i.wal != nil {
			i.logger.Info("Emergency: Flushing WAL to disk...")
			if err := i.wal.Sync(); err != nil {
				i.logger.Error("Emergency WAL sync failed: %v", err)
			}
		}

		// Step 2: Take emergency snapshot
		if i.snapshotter != nil && i.kv != nil {
			i.logger.Info("Emergency: Saving snapshot...")
			if path, err := i.snapshotter.Save(i.kv); err != nil {
				i.logger.Error("Emergency snapshot failed: %v", err)
			} else {
				i.logger.Info("Emergency snapshot saved: %s", path)
			}
		}
	}()

	// Wait for shutdown to complete, but enforce a hard timeout
	select {
	case <-done:
		i.logger.Info("Graceful shutdown complete.")
	case <-time.After(shutdownTimeout):
		i.logger.Warn("Shutdown timeout exceeded (%v), forcing exit.", shutdownTimeout)
	}

	i.Stop()
}

func (i *Instance) Stop() {
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
	if i.cfg.Persistence.Enabled && i.snapshotter != nil && i.kv != nil {
		if path, err := i.snapshotter.Save(i.kv); err == nil {
			i.logger.Info("Final snapshot saved: %s", path)
		}
	}
	if i.wal != nil {
		i.wal.Close()
	}
	if i.auditGuard != nil {
		i.auditGuard.Flush()
		i.auditGuard.Close()
	}
	i.logger.Info("DBX instance shutdown complete")
}

func (i *Instance) ErrorChannel() <-chan error {
	return i.serverErr
}
