# DBX Architecture

## Overview

DBX is a multi-tenant, in-memory database engine combining a Redis-compatible Key-Value store with a native Vector Search engine.

## Components

### 1. DBX Orchestrator (`cmd/dbx-orchestrator`)
The control plane. It is the single entry point for all clients and the admin dashboard.

**Responsibilities:**
- JWT-based authentication and authorization
- Tenant provisioning and lifecycle management
- Reverse-proxying data plane requests to the correct DBX node
- Distributed consensus via HashiCorp Raft (for multi-node HA setups)

**Key Packages:**
- `internal/orchestrator/manager.go` — Tenant state machine
- `internal/orchestrator/raft_node.go` — Raft cluster membership
- `internal/orchestrator/proxy.go` — HTTP reverse proxy to data nodes

### 2. DBX Server (`cmd/dbx-server`)
The data plane. One instance runs per tenant (or per shard in cluster mode).

**Responsibilities:**
- Serving Redis RESP3 protocol commands over TCP
- Managing the in-memory KV store (strings, hashes, lists, sets, sorted sets, streams)
- Managing the HNSW Vector index
- WAL logging and snapshotting for persistence

**Key Packages:**
- `internal/engine/kv.go` — Key-Value store operations
- `internal/engine/hnsw.go` — Hierarchical Navigable Small World vector index
- `internal/engine/vector.go` — Vector insert and ANN search API
- `internal/persistence/wal.go` — Write-Ahead Log

### 3. Dashboard (`dashboard/`)
A React + Vite + TailwindCSS SPA compiled and embedded directly into the Orchestrator binary at build time via Go's `embed.FS`.

## Data Flow

```
Client Request
     │
     ▼
Orchestrator (Port 8000)
  1. Verify JWT
  2. Identify tenant from path (/t/{tenantID}/...)
  3. Look up tenant's data node port
  4. Reverse proxy to DBX Server
     │
     ▼
DBX Server (Port 808X)
  1. Parse Redis RESP command
  2. Route to KV engine or Vector engine
  3. Return RESP response
```

## Persistence

DBX uses a two-layer persistence model:

1. **WAL (Write-Ahead Log):** Every write operation is logged to disk before being applied to memory. On crash recovery, the WAL is replayed.
2. **Snapshots:** Periodic full memory snapshots are taken (configurable interval). On restart, the latest snapshot is loaded first, then the WAL is replayed from that checkpoint.
3. **S3 Backup:** The orchestrator can trigger an upload of the snapshot to an S3-compatible bucket via the `/api/tenants/backup` endpoint.

## Security Model

- **Authentication:** JWT Bearer tokens issued by the orchestrator.
- **Authorization:** Role-based, with `admin` having full access.
- **Rate Limiting:** Per-IP brute-force protection on the `/api/login` endpoint (5 failures = 60-second lockout).
- **DoS Protection:** `http.MaxBytesReader` limits on all data plane request bodies.
- **Admin Credentials:** Stored as bcrypt hashes; never in plaintext.
- **TLS:** Supported on the control plane; disabled with `-insecure-http` flag for local development only.
