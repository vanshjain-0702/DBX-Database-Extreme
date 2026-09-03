# Isolation Kernel

This is the source of truth for DBX tenant isolation. Claims here must remain
literally true in the code. See [positioning.md](positioning.md).

## The claim

**A DBX tenant is a sealed execution domain.** Keys, vectors, WAL records, and
sockets that belong to one customer are not reachable from another tenant by
naming convention. On Linux production (`DBX_ISOLATION_MODE=strict`) the Linux
kernel enforces that.

This is the security USP. It is not "the strongest sandbox ever built." Firecracker,
gVisor, and Qubes exist. What those systems do not do is make **per-customer memory
plus vector search** the unit that is sealed. Shared-cluster stores (Redis prefixes,
Qdrant collections, Pinecone namespaces) authenticate at the API and share a
process, a filesystem, and a key.

## Profiles

| Mode | Who uses it | Seals |
|---|---|---|
| `inprocess` | Tests, soak, `make run-dev` | Directory, ACL, quotas |
| `standard` | macOS/Windows, or Linux without a worker binary | Envelope encryption, Unix sockets, `SO_PEERCRED` on Linux |
| `strict` | Linux production (Docker/Helm default) | `standard` plus a dedicated `dbx-server` process, Landlock LSM, cgroup v2 |

Unset `DBX_ISOLATION_MODE` is `inprocess` so CI density tests and
`make run-dev` (`-insecure-http`) keep their shape. Docker Compose and Helm
set `strict` and `DBX_PRODUCTION=1`. A production-shaped process **refuses
to boot in `inprocess`** unless you set `DBX_ALLOW_INPROCESS=1`. That is how
the security USP stays on when you claim production.

`strict` on a non-Linux kernel degrades to `standard` and says so. Do not market
macOS or Windows as Landlock-isolated.

## The five seals (strict)

1. **Process.** The orchestrator `exec`s `dbx-server` per tenant. A panic or
   memory blow-up is one worker. Tenants do not share a Go heap.
2. **Filesystem.** After bind, the worker calls Landlock so it cannot **open**
   paths outside its tenant directory, except read-only `/proc` and `/etc` and
   read/write `/dev` (the Go runtime and timezone data need them). Reading a
   sibling `$DATA/tenants/{other}/...` is denied by the kernel. Each worker also
   gets its own control token, so it cannot authenticate to a neighbour's
   control endpoints.
3. **Memory.** The orchestrator places the worker in a cgroup v2 with
   `memory.max` equal to the tenant quota **when the host delegates cgroup
   writes**. Most containers do not: expect
   `cgroup mkdir: permission denied`, which is logged and non-fatal. Treat the
   cgroup as a bonus on bare metal and the application quota plus
   `DBX_NODE_MEMORY_BUDGET` as the real limit.
4. **Crypto.** Each tenant has a 256-bit DEK, wrapped by `DBX_KEK` (also
   256-bit, hex). WAL frames, snapshots, `.vec.meta` (ids and tombstones), and
   `.hnsw` (the graph) are AES-256-GCM. The worker receives only its DEK on
   stdin; it never sees the KEK, and the KEK is stripped from its environment.
   `DeleteTenant(purge=true)` shreds the wrapped DEK first, so leftover
   ciphertext is unreadable without a scan. Backups carry the wrapped DEK so a
   restore can still be opened by an operator holding the KEK.
5. **IPC.** Tenant RESP/HTTP/replication listen on Unix sockets mode `0600` in
   the tenant directory, and Linux `SO_PEERCRED` accepts only the orchestrator
   PID on the RESP and HTTP sockets. Public `:6380` and `:8000` stay TCP (TLS on
   the control plane). mTLS *on* the Unix sockets is deliberately not used: peer
   credentials plus POSIX permissions are the correct same-host control, and
   loopback TLS handshakes are a measured cost. Replication uses
   `isolation.DialTimeout` so Unix-socket replicas get the same 1-second
   connect timeout as TCP replicas.

## Implementation notes

- **VADD_BATCH metadata persist.** Every batch write flushes ids/tombstones to
  the sealed `.meta` file so a checkpoint that stores the TypeVector key as nil
  can reopen the mmap on restart.
- **Vector reopen after snapshot.** Recovery calls `VectorStore.ReopenPersisted`
  after WAL replay to attach mmap indexes for TypeVector keys that the snapshot
  restored with a nil value. Without this, VSEARCH returns an empty list while
  KV still has the index key.
- **Worker HTTP timeout.** The Unix-socket HTTP client for worker control
  endpoints has a 2-minute timeout so large-tenant backup downloads do not
  time out.
- **Per-worker tokens.** Each sandboxed worker receives a unique
  `DBX_INTERNAL_API_TOKEN` at spawn. The HTTP proxy resolves it per request so
  a worker restart does not leave the cached proxy holding a stale credential.

## What this does not claim

Read this section before repeating any of the claims above.

- **`.vec` rows are not encrypted by DBX.** SQ8 rows stay mmap'd so idle tenants
  live in page cache (USP 3). Decrypting them into anonymous memory would put
  every tenant's vectors on the Go heap, and re-encrypting the whole file per
  insert is not affordable. For embedding confidentiality at rest, run the data
  directory on fscrypt or LUKS. What DBX encrypts is the searchable surface —
  ids, tombstones, the graph, the WAL, and checkpoints — so shredding a DEK
  leaves anonymous SQ8 bytes with no ids and no index.
- **Landlock governs file opens, not sockets or metadata.** ABI 1–3 has no right
  covering `connect()` to an existing Unix socket, and `stat()` on a sibling
  path still succeeds. Cross-tenant socket access is stopped by `SO_PEERCRED`
  and file mode, not by Landlock. The replication socket has no peer-PID check
  because a replica worker is a different PID than the orchestrator.
- **There is no network restriction.** A worker can still open outbound TCP.
  Landlock ABI 4 network rules are not used.
- **Data in use is plaintext in the worker.** A debugger on that PID, root, or a
  kernel that ignores Landlock can read it.
- **User namespaces are not applied.** Workers run as the orchestrator uid.
  Landlock and peer-PID checks are what stop a sibling worker, not a different
  uid.
- **Density is process-bound under `strict`.** The published 100 tenants/node
  figure was measured in-process (shared Go runtime, idle data in page cache).
  Re-measured on Linux 6.12 with Landlock: a freshly started idle worker is
  about **14–17 MiB RSS**, worst idle GET stayed **2–12 ms** with 8 idle / 4
  active writers. 100 sealed tenants would therefore cost roughly **1.5 GiB**
  of process RSS before any corpus. Use `inprocess` when density is the goal;
  use `strict` when the kernel boundary is the goal. `DBX_LARGE=1 go test
  ./internal/orchestrator/ -run TestStrictModeDensityAndRSS` re-runs the drill.
- **`inprocess` is still available** and is the default. It is a density and
  development profile, not the security USP.
- This is not "the strongest sandbox ever built." Firecracker, gVisor, and Qubes
  are stronger sandboxes. What is unusual here is that the sealed unit is one
  customer's KV plus vector memory, with a key you can destroy.

## Operator contract

- Set `DBX_KEK` to 64 hex characters before enabling `standard` or `strict`.
  Missing KEK is a boot failure, not a silent plaintext fallback.
- Keep `dbx-server` next to `dbx-orchestrator` (Docker image does this;
  `DBX_SERVER_BIN` overrides).
- Public TLS is unchanged: do not run `-insecure-http` in production.
- For embedding confidentiality, put `DBX_DATA_DIR` on LUKS or fscrypt.
  Set `DBX_REQUIRE_DISK_ENCRYPTION=1` to refuse boot on a plaintext volume.
  `.vec` rows are otherwise readable after a DEK shred.
- Hibernating a tenant stops the worker and leaves ciphertext on disk. Wake
  unwraps the DEK again.
