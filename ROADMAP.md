# DBX Roadmap

Ordered by how much each item strengthens the product thesis in
[docs/positioning.md](docs/positioning.md), not by implementation difficulty.

Every item answers the same question: **does this make "one isolated store per customer"
more true, cheaper, or safer?** If it doesn't, it is not on this list.

---

## Now — correctness and blast radius

These came out of a code audit, not a wishlist. They are ahead of the density work
because a tenant that loses data or takes down its neighbours is not isolated in any
sense that matters.

The August 2026 hardening implementation completed the WAL v2, quota, bounded-expiry,
panic-boundary, scoped-key, single-ingress, tombstone, and checksummed-restore work below.
Vector ingest, recall@10, search p99, noisy-neighbor unit isolation, and Linux CI race
coverage now pass. Version 1 is **GO for the single-node profile**. Remaining follow-ups
are a 100/25 density soak and engine-package coverage. Async WAL replicas are
available without routing writes through Raft.

### 0a. Panic isolation
**Status:** implemented on the RESP, HTTP, executor, instance-task, and Raft apply
paths; full cross-tenant fault matrix still incomplete · **Thesis:** USP 1

`recover()` wraps tenant RESP connections, HTTP handlers, `Executor.Execute`,
background instance tasks, and both Raft FSM `Apply` methods. A panicking command is
tested to return `ERR internal server error` and leave the executor serving later
commands. A panic that escapes those boundaries can still terminate the process.

**Done when:** a deliberately panicking command kills one connection, and every other
tenant keeps serving.

### 0b. Enforce per-tenant memory
**Status:** implemented for the v1 string/vector surface · **Thesis:** USP 1, USP 3

Writes that would exceed `engine.max_memory` are rejected with `OOM tenant quota exceeded`
before WAL. Neighbours keep serving. Tenants still share one Go heap, so a host-level
`DBX_NODE_MEMORY_BUDGET` admission check is the process-wide backstop.

**Done when:** a tenant that exceeds its ceiling is rejected, and its neighbours are unaffected.

### 0c. Sample the expiry reaper instead of scanning
**Status:** implemented with per-shard deadline heaps · **Thesis:** USP 3

Each tenant runs a 1-second ticker calling `KVStore.DeleteExpired()`, which walks
every key in all 256 shards under write locks. Cost is O(keys × tenants) per second
regardless of how many keys actually expired.

**Done when:** expiry cost is proportional to the number of expiring keys, not to
keyspace size.

### 0d. Write the restore path
**Status:** implemented; repeated fault drills pending · **Thesis:** USP 1

`RunBackup` zips the live tenant directory — no quiesce, staged through a full copy
in the temp directory — and there is no code anywhere that downloads, unpacks, or
restores that archive. A backup you cannot restore is not a backup.

**Done when:** a tenant can be recreated on a clean node from its archive alone.

### 0e. Fixed (recorded here so it does not regress)
The HNSW graph was persisted only by `Close()`, which shutdown never called. After
any restart the graph loaded empty while metadata still listed every id, and WAL
replay could not repair it because replayed `VADD`s skip graph insertion for known
ids — so `VSEARCH` returned an empty list with no error and every stored vector was
silently unsearchable. Indexes now rebuild the graph from their mmap rows on open,
shutdown persists the graph, and `TestVectorStoreRebuildsGraphAfterCrash` covers it.

---

## Next — density and safety per tenant

### 1. Single ingress port for all tenants
**Status:** implemented on public `:6380` · **Thesis:** USP 1, USP 3

Today each tenant binds its own RESP port (`6401`, `6402`, …). Tenant density is therefore
capped by the operating system and the cloud network long before it is capped by memory —
which directly contradicts the claim that a tenant is cheap. HTTP already multiplexes on
`:8000` via `/t/{tenant}`; RESP does not.

**Shape of the work:** a RESP front door on a single port that reads the tenant identity from
`AUTH` / `HELLO` and routes to the right in-process engine. One port, one Kubernetes Service,
one firewall rule, regardless of tenant count.

**Current limit:** one public port is implemented, while loopback backend listeners remain;
the certified v1 cap is therefore 100 tenants/node, not 1,000.

### 2. Per-tenant quotas and usage accounting
**Status:** implemented; density/noisy-neighbor gate pending · **Thesis:** USP 1, business model

Isolation without limits is not isolation. `Tenant` currently carries ports and a data
directory but no memory ceiling, key ceiling, vector ceiling, or rate limit, and we expose no
per-tenant usage counters.

**Shape of the work:** quota fields on the tenant record, enforcement in the engine's write
path, and a usage endpoint reporting keys, vectors, bytes resident, bytes on disk, and
operations for each tenant.

**Done when:** a runaway tenant hits its own ceiling and no other tenant notices, and an
operator can answer "what does this customer cost me?" from the API.

### 3. Scoped credentials per tenant
**Status:** implemented with immediate revocation · **Thesis:** USP 1

Roles and an ACL check exist (`internal/auth/role.go`, `internal/auth/acl.go`), but issued API
keys and dashboard tokens carry no role, no command allowlist, and no key-prefix scope. The
default user has full permissions, and `VADD` is missing from the write-command set, so a
"reader" can still ingest vectors.

**Shape of the work:** attach a role to every issued key, wire ACL enforcement to token claims,
complete the write-command classification (vector and flush commands included), and expose key
creation with a role in the dashboard.

**Done when:** a read-only key can search vectors and cannot write, delete, or flush anything —
verified by a test, not by inspection.

---

## Then — durability that matches the isolation story

### 4. Per-tenant hot standby
**Status:** async WAL replicas shipped; Raft-on-writes still rejected · **Thesis:** USP 1

Losing a node today without a replica means restoring a tenant from an archive.
Each tenant can now run a primary plus up to two asynchronous WAL replicas.
The primary appends locally and acks; replica TCP is a non-blocking side
channel. Ingress keeps AUTH'ing the original tenant id; `POST /api/tenants/promote`
swaps the public tenant onto the replica's data directory and ports.

Data-plane Raft remains fail-closed: its log is still in-memory and snapshots
are placeholders. Do not enable `raft_enabled` on tenant engines.

**Constraint that must not be violated:** replicate WAL bytes, not marshaled command structs,
and acknowledge locally for asynchronous replicas. Routing every write through JSON consensus
is what previously cost roughly two orders of magnitude of write throughput.

**Done when:** killing the primary for one tenant costs seconds, not minutes, and the other
tenants on the node are unaffected. Promote currently restarts the replica set (seconds of
unavailable writes for that tenant only).

### 5. Published recall for quantized search
**Status:** 100k recall@10, ingest, and search-latency gates passing · **Thesis:** USP 3

We claim a ~4× smaller vector payload from SQ8 but publish nothing about the accuracy cost, so
half of the claim is unverifiable.

**Shape of the work:** a recall@k harness against a fixed dataset, an opt-in float32 storage
mode for accuracy-sensitive tenants, and both numbers in the benchmark documentation.

**Measured:** 100k × 128 SQ8 produces mean recall@10 0.920 and fifth-percentile 0.800,
ingest 7,233 vectors/s, and search p50/p95/p99 of 2.304/3.132/3.730 ms on the
certification host. Linux CI re-runs the harness.

---

## Later — scale within the same thesis

### 6. Tenant tiering: hibernate and wake
Idle tenants should cost close to nothing. Evict a cold tenant's engine from memory entirely
and rehydrate it from its own snapshot on first request. This is the strongest possible version
of "cost scales with active tenants."

### 7. Per-tenant export and import as a first-class artifact
A single portable file per tenant that another DBX node can restore. Turns customer migration,
on-prem hand-off, and point-in-time recovery into a file copy.

### 8. Product quantization for large tenants
Only once customers actually arrive with tenants large enough to need it. SQ8 is sufficient for
per-tenant working sets, which is the shape we design for.

---

## Deliberately not on this roadmap

These are good ideas for a different product. See
[docs/positioning.md](docs/positioning.md) §5.

- Beating Redis on single-instance KV throughput
- Sharded, billion-vector ANN with complex payload filtering
- A managed DBX cloud service (the BSL exists so this stays our decision)
- SQL, joins, or transactions across entities
