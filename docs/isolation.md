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

Unset `DBX_ISOLATION_MODE` is `inprocess` so CI density tests keep their shape.
Production images set `strict`.

`strict` on a non-Linux kernel degrades to `standard` and says so. Do not market
macOS or Windows as Landlock-isolated.

## The five seals (strict)

1. **Process.** The orchestrator `exec`s `dbx-server` per tenant. A panic or
   memory blow-up is one worker. Tenants do not share a Go heap.
2. **Filesystem.** After bind, the worker calls Landlock so it cannot open paths
   outside its tenant directory except `/proc`, `/dev`, and `/etc` (Go runtime
   and timezones). Sibling `$DATA/tenants/{other}` is denied by the kernel.
3. **Memory.** The orchestrator places the worker in a cgroup v2 with
   `memory.max` equal to the tenant quota when the host delegates cgroup
   writes. If cgroup writes fail (common in locked-down containers) startup
   continues and the application quota still rejects writes.
4. **Crypto.** Each tenant has a 256-bit DEK, wrapped by `DBX_KEK` (also 256-bit,
   hex). WAL frames, snapshots, `.vec`, `.vec.meta`, and `.hnsw` are AES-256-GCM.
   The worker receives only its DEK on stdin. It never sees the KEK.
   `DeleteTenant(purge=true)` shreds the wrapped DEK first: ciphertext on disk
   becomes unreadable without scanning. Search still runs over plaintext rows
   **inside** the worker; that is required for mmap/ADC and is why the process
   seal exists.
5. **IPC.** Tenant RESP/HTTP/replication listen on Unix sockets mode `0600` in
   the tenant directory. Linux `SO_PEERCRED` accepts only the orchestrator PID.
   Public `:6380` and `:8000` stay TCP (TLS on the control plane). Combining
   mTLS *on* those Unix sockets is not used: peer credentials plus POSIX perms
   are the same-host control, and loopback TLS handshakes are a measured cost.

## What this does not claim

- Data in use is plaintext in the worker. A debugger on that PID, or a kernel
  that ignores Landlock, can read it.
- User namespaces are not applied yet. Workers run as the orchestrator uid.
  Landlock and peer-PID checks are what stop a sibling worker, not a different
  uid.
- Backups, S3 copies, and replicas have their own ciphertext. Revoking one DEK
  shreds that tenant's directory; copies wrapped under the same DEK become
  inert, copies re-encrypted under another key do not.
- `inprocess` is still available. It is a density profile, not the security USP.

## Operator contract

- Set `DBX_KEK` to 64 hex characters before enabling `standard` or `strict`.
- Keep `dbx-server` next to `dbx-orchestrator` (Docker image does this;
  `DBX_SERVER_BIN` overrides).
- Public TLS is unchanged: do not run `-insecure-http` in production.
- Hibernating a tenant stops the worker and leaves ciphertext on disk. Wake
  unwraps the DEK again.
