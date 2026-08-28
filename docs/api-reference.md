# DBX API Reference

The public application data plane is authenticated RESP on port `6380`. The first command
must be:

```
AUTH <tenantID>:<keyID> <secret>
```

The HTTP `/t/{tenantID}/query` path requires an operator JWT and is intended for the
dashboard and operational tooling, not as a public tenant API.

## Authentication

### POST `/api/login`
Get a JWT token.

**Request:**
```json
{ "username": "admin", "password": "yourpassword" }
```
**Response:**
```json
{ "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." }
```

---

## Tenant Management

### GET `/api/tenants`
List all provisioned tenants. Requires auth.

### POST `/api/provision`
Create a new tenant: an isolated engine with its own data directory, WAL, vector index, and
snapshot lineage. Requires auth.

**Request:**
```json
{ "id": "my-app-prod", "name": "My Production App", "replicas": 1 }
```

`replicas` is optional (0–2). `0` is the certified single-node path. `1` or `2`
starts async WAL replicas (`{id}-r1`, `{id}-r2`). The primary still acks writes
locally; replica lag is possible.

### POST `/api/tenants/promote`
Fail a public tenant over to a replica. AUTH identity is unchanged; the control
plane swaps data directories and loopback ports onto the replica copy, then
restarts the replica set.

```json
{ "replica_id": "my-app-prod-r1" }
```

### POST `/api/tenants/backup`
Create a mutation-consistent archive with a versioned SHA-256 manifest. Writes for that tenant
pause briefly; reads and other tenants remain available. Requires auth.

**Request:**
```json
{ "id": "my-app-prod" }
```

### POST `/api/tenants/restore`
Validate and atomically restore a checksummed archive into an existing tenant. The target
tenant is stopped; a failed validation or startup rolls back to its previous directory.

```json
{ "id": "my-app-prod", "path": "data/backups/backup_my-app-prod_....dbx.zip" }
```

### `/api/v1/tenants/{tenantID}/keys`
- `POST`: create a 256-bit tenant key. Body fields: `name`, `role` (`reader`, `writer`, or
  `tenant-admin`), and optional `key_patterns`.
- `GET`: list key metadata. Secret hashes are never returned.
- `DELETE /api/v1/tenants/{tenantID}/keys/{keyID}`: revoke immediately, including existing
  RESP connections.

### POST `/api/tenants/delete`
Off-board a tenant: stop its engine and remove it from the control plane. With `purge: true`
the tenant's data directory is erased, which is what a customer deletion request requires.
No other tenant's data is read or modified. Requires auth.

**Request:**
```json
{ "id": "my-app-prod", "purge": true }
```
**Response:**
```json
{ "status": "deleted", "id": "my-app-prod", "purged": true }
```

### GET `/api/v1/tenants/{id}/usage` and GET `/api/usage`
Per-tenant keys, live vectors, memory, disk bytes, and command counters. `/api/usage` lists
every tenant. This is the meter an operator's billing loop should call.

### POST `/api/tenants/export` / `/api/tenants/import`
Aliases for backup and restore. The archive is a portable tenant file another DBX node can
import. SHA-256 manifest, rollback on a failed restore.

### POST `/api/v1/tenants/{id}/hibernate` and `/wake`
Stop a cold engine and keep its directory. Wake rehydrates from that directory. Idle tenants
should not occupy a live process. Sentinel will not restart a hibernated tenant.

### GET `/metrics`
Prometheus text on the orchestrator. With `-insecure-http` (local dev) this is open.
In production it requires `Authorization: Bearer` — either an operator JWT or
`DBX_INTERNAL_API_TOKEN`. Per-tenant gauges: keys, vectors, memory, disk,
commands, hibernated. JSON snapshots remain at `GET /t/{tenantID}/metrics`. Tenant engines also
expose `/usage` and `/metrics/prometheus` to the orchestrator internal token.

---

## KV Commands

The durable v1 profile guarantees string, TTL, vector, health/introspection, and
authentication commands. Unsupported mutation families fail before dispatch. Compatible
read commands may remain available for inspection, but hashes, lists, sets, sorted sets,
streams, JSON, bitmap, geo, transactions, pub/sub, `FLUSH*`, cluster, and replication
mutations are not part of the v1 contract.

| Command | Example |
|---|---|
| `SET` | `["SET", "key", "value"]` or `["SET", "key", "value", "EX", "60"]` |
| `SETEX` | `["SETEX", "key", "60", "value"]` — same durable path as `SET … EX`; what node-redis `setEx` sends |
| `GET` | `["GET", "key"]` |
| `DEL` | `["DEL", "key"]` |
| `KEYS` | `["KEYS", "*"]` |
| `EXPIRE` | `["EXPIRE", "key", "3600"]` |
| `TTL` | `["TTL", "key"]` |
| `PERSIST` | `["PERSIST", "key"]` |
| `MSET` | `["MSET", "a", "1", "b", "2"]` |
| `MGET` | `["MGET", "a", "b"]` |

---

## Vector Commands

Each vector command takes the index key first, so one
tenant can hold several independent indexes.

| Command | Example | Notes |
|---|---|---|
| `VADD` | `["VADD", "memories", "doc:1", "0.1", "0.2", "0.9"]` | Adds or replaces one vector. Prefer the batch form for bulk loads. |
| `VADD_BATCH` | `["VADD_BATCH", "memories", "3", "doc:1", "0.1", "0.2", "0.9", ...]` | Writes the batch then inserts into HNSW; metadata is flushed on close/checkpoint. |
| `VDEL` | `["VDEL", "memories", "doc:1"]` | Writes a generation tombstone. Deleted rows may be traversed but are never returned. |
| `VCOMPACT` | `["VCOMPACT", "memories"]` | Rewrites active rows and rebuilds HNSW; tenant-admin only. |
| `VSEARCH` | `["VSEARCH", "memories", "0.1", "0.2", "0.8", "5"]` | Cosine similarity over the HNSW index, top-k last. |

There is no `VGET` or `VSET`. The 100k-vector recall@10, ingest, and search-latency
gates pass on the certified profile. See the
[certification matrix](../scripts/benchmarks/performance_analysis.md) before selecting
a production capacity.

---

## Metrics

### GET `/t/{tenantID}/metrics`
Returns real-time performance metrics for the given tenant.

**Response:**
```json
{
  "total_commands": 150432,
  "avg_latency_ns": 3200,
  "tenant_memory_used_bytes": 52428800,
  "tenant_memory_limit_bytes": 536870912,
  "tenant_ready": 1,
  "active_conns": 12
}
```
