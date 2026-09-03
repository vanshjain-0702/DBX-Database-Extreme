# DBX Architecture

## Overview

DBX is a per-tenant memory engine: every tenant gets its own isolated in-memory store holding
both key-value working state and HNSW-indexed vector memory, with its own WAL and its own
snapshot lineage. The orchestrator owns tenant lifecycle; each tenant owns its data.

The wire protocol is RESP2-compatible, so existing clients connect without a custom driver. See
[positioning.md](positioning.md) for what that does and does not imply.

## Components

### 1. DBX Orchestrator (`cmd/dbx-orchestrator`)
The control plane. It is the single entry point for all clients and the admin dashboard.

**Responsibilities:**
- JWT-based authentication and authorization
- Tenant lifecycle: provision (`/api/provision`, optional `replicas`), promote
  (`/api/tenants/promote`), back up (`/api/tenants/backup`), restore
  (`/api/tenants/restore`), export/import aliases, hibernate/wake
  (`/api/v1/tenants/{id}/hibernate`, `/wake`), usage (`/api/v1/tenants/{id}/usage`,
  `/api/usage`), and delete (`/api/tenants/delete`, optionally purging)
- Prometheus text on `GET /metrics` (Bearer JWT or `DBX_INTERNAL_API_TOKEN` in
  production; open only with `-insecure-http`). JSON snapshots remain at
  `GET /t/{tenantID}/metrics`.
- One public RESP ingress on `:6380`, authenticated with tenant-scoped keys
- Reverse-proxying data plane requests to the current primary
- Atomic local control-plane state. Data-plane Raft and cluster mode fail closed.
  Sentinel does not restart a hibernated tenant.

Because each tenant owns a separate data directory, every lifecycle operation affects exactly
one customer. There is no cross-tenant scan and no shared keyspace to sweep.

**Key Packages:**
- `internal/orchestrator/manager.go` — Tenant state machine
- `internal/orchestrator/resp_proxy.go` — authenticated single-port RESP ingress
- `internal/orchestrator/proxy.go` — HTTP reverse proxy to data nodes

### 2. DBX Server (`cmd/dbx-server`)
The data plane. One instance runs per tenant member (primary or replica).

**Responsibilities:**
- Serving the supported RESP2-compatible v1 command surface over loopback TCP
- Managing durable strings and TTLs; unsupported mutation families are rejected
- Managing the HNSW Vector index
- WAL logging and snapshotting for persistence
- Optional async WAL streaming: a primary broadcasts committed records; a replica
  applies them read-only. Writes are never acknowledged through Raft.

**Key Packages:**
- `internal/engine/kv.go` — Key-Value store operations
- `internal/engine/hnsw.go` — Hierarchical Navigable Small World vector index
- `internal/engine/vector.go` — Vector insert and ANN search API
- `internal/persistence/wal.go` — Write-Ahead Log

### 3. Dashboard (`dashboard/`)
A React + Vite + Tailwind CSS SPA compiled and embedded into the orchestrator
binary at build time via Go's `embed.FS`. **Tenant keys**
(`/cluster/{id}/keys`) mints `reader` / `writer` / `tenant-admin` credentials;
the secret is shown once. Console, explorer, and vector playground talk to
`:8000` with an operator JWT. They are operator tools, not the public tenant API.

The public marketing site is **not** this UI. It is static HTML in `website/`.
Open `website/index.html` (or `make site`). GitHub Pages is 404 while the
repository is private; `dbxdb.io` has no DNS. See `website/README.md`.

## Data Flow

```
Application Request
     │
     ▼
RESP Ingress (Port 6380)
  1. Require AUTH tenantID:keyID secret
  2. Verify the persistent scoped credential
  3. Route to the tenant's loopback listener
  4. Re-resolve the key on every command for immediate revocation
     │
     ▼
Tenant DBX Server (loopback)
  1. Enforce role and every affected-key pattern
  2. Append one WAL v2 state-image transaction and ack locally
  3. Apply to KV or vector engine
  4. Return RESP response
  5. Replicas receive WAL frames on a side channel (no extra write RTT)
```

## Persistence

DBX uses a two-layer persistence model:

1. **WAL v2:** Length-framed, sequenced, CRC-protected state-image transactions are appended
   before apply. `always` fsyncs before acknowledgement; `everysec` defines a one-second loss window.
2. **Checkpoints:** A sequence-bearing KV snapshot is atomically installed before covered WAL
   segments are removed. Only a final partial frame may be truncated; CRC corruption is fatal.
3. **Vector files:** SQ8 rows, generation tombstones, metadata, and a rebuildable checksummed
   HNSW cache live in the tenant directory.
4. **Backup/restore:** A maintenance lock produces a manifest with SHA-256 checksums. Restore
   validates into a sibling directory and swaps with rollback. `POST /api/tenants/export`
   and `/import` are aliases. Hibernate stops the engine process and keeps the directory.
5. **Density:** CI runs 12 idle / 4 active engines. Operators run `make soak` for
   100 idle / 25 active. That is not a 100-orchestrator-process soak.

## Security Model

- **Control-plane authentication:** JWT Bearer tokens issued to operators.
- **Data-plane authentication:** 256-bit tenant keys stored only as hashes. Roles
  `reader`, `writer`, and `tenant-admin` are enforced on every RESP command.
  A reader cannot `SET`, `SETEX`, `VADD`, or `VDEL`. Revoke deletes the user so
  existing connections fail on the next command.
- **Authorization:** `reader`, `writer`, and `tenant-admin` roles with key-pattern scopes.
- **Rate Limiting:** Per-IP brute-force protection on the `/api/login` endpoint (5 failures = 60-second lockout).
- **DoS Protection:** `http.MaxBytesReader` limits on all data plane request bodies.
- **Admin Credentials:** Stored as bcrypt hashes; never in plaintext.
- **TLS:** Supported on the control plane; disabled with `-insecure-http` flag for local development only.
- **Isolation Kernel:** Linux production (`DBX_ISOLATION_MODE=strict`) seals each tenant as its own process, Landlock filesystem, cgroup, envelope-encrypted WAL/vectors, and Unix sockets restricted by `SO_PEERCRED`. Each worker gets a unique control token; the HTTP proxy resolves it per request. Replication dials through `isolation.DialTimeout` (Unix socket + 1s timeout). Production (TLS or `DBX_PRODUCTION=1`) refuses `inprocess` unless `DBX_ALLOW_INPROCESS=1`. Set `DBX_REQUIRE_DISK_ENCRYPTION=1` to refuse boot on a plaintext volume (`.vec` rows are otherwise unencrypted). See [isolation.md](isolation.md).
