---
title: "The Tenant Is the Database: Isolated Memory for Multi-Customer AI Products"
published: false
description: "DBX makes each customer an isolated engine — working state and vector recall in one process, one WAL, one delete. Here's why, and how to try it on your laptop."
tags: ai, golang, database, tutorial
cover_image: https://vanshjain-0702.github.io/DBX-Database-Extreme/assets/og-image.jpg
canonical_url:
series: DBX
---

You are shipping an agent platform, a copilot, or a vertical AI SaaS. Every one of *your* customers needs their own memory: session state, a scratchpad, rate counters, and a private RAG corpus. The stores you can buy were built around one large shared cluster. Tenancy is something you bolt on afterwards.

That bolt-on has a shape you already know:

```
SET tenant:acme:session:42 '{"step": 3}'
VSEARCH memories <query> TOPK 8 FILTER tenant_id = 'acme'
```

It works until a tool call forgets the filter. Then neighbour context lands in an autonomous reasoning chain, and the database never noticed — because the database never had a tenant. It had a prefix.

[DBX](https://github.com/vanshjain-0702/DBX-Database-Extreme) is the other shape: **the per-tenant memory engine for AI products.** One isolated store per customer, holding their working state *and* their vector memory. Isolation is a directory, a WAL, an HNSW index, and (on Linux production) a process — not a naming convention.

This is the product walkthrough. The security kernel (Landlock, envelope encryption, `SO_PEERCRED`) has its own engineering post in this series. Claims below are true in the binary; the source of truth is [`docs/positioning.md`](https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/positioning.md).

## The failure is structural, not a missing `WHERE`

A typical agent stack stores session KV in one system and embeddings in another. You write to both. After a crash you hope they didn't drift. Backup is cluster-wide, so you cannot restore *one* customer. "Delete my data" is a scan. A noisy tenant's working set evicts a quiet neighbour's cache. Per-customer pricing is guesswork because the database has no object that means "this customer."

None of that is fixed by making the shared cluster faster.

Two honest alternatives exist, and both show their cost around tenant 200:

1. **A microVM per user.** Right answer for untrusted *code*. Heavy answer for a WAL, an mmap'd index, and a few thousand keys.
2. **Row-level filters and collection names.** Cheap, and still one process, one filesystem, one key. A hijacked tool is still a function in the same address space.

DBX's claim is narrower than "replace your cache and your vector database." It is: **make one customer's KV plus vector memory the unit you can provision, back up, and shred.**

## What a tenant actually is

`POST /tenants` returns a live engine. That engine is not a namespace inside a shared heap. It is:

- its own data directory
- its own write-ahead log
- its own 8-bit scalar-quantized HNSW index (mmap'd `.vec` rows, ADC at query time)
- its own snapshot lineage
- on Linux `strict`, its own `dbx-server` process (~14–17 MiB RSS idle)

Deleting a tenant deletes that directory. There is no prefix sweep and no cross-customer blast radius. Export and restore are file operations.

Working state (KV with TTL) and semantic recall (vectors) share the process, the connection, the directory, and the backup archive. There is no dual-write path between a cache and a separate vector service.

```
┌─────────────────────────────────────────────────────────────────┐
│  HTTP control plane :8000     Authenticated RESP ingress :6380  │
│      tenant lifecycle, scoped credentials, routing, backups     │
│─────────────────────────────────────────────────────────────────│
│  Tenant A               │  Tenant B               │  Tenant N…  │
│  KV + HNSW + WAL        │  KV + HNSW + WAL        │  …          │
│  own directory          │  own directory          │             │
└─────────────────────────────────────────────────────────────────┘
```

The orchestrator owns provisioning, AUTH, and routing. Each tenant owns its data.

Two profiles — pick one, do not claim both at once:

| Profile | When | What you get |
|---|---|---|
| `inprocess` | `make run-dev`, CI, density soak | Directory + ACL + quotas. Shared Go runtime. |
| `strict` | Linux production (Compose/Helm default) | Process + Landlock + encrypted WAL/meta/graph + `SO_PEERCRED` |

Production (TLS, or `DBX_PRODUCTION=1`) **refuses `inprocess`** unless you set `DBX_ALLOW_INPROCESS=1`. That is how the security USP stays on when you say "production."

## Fifteen minutes on a laptop

Prerequisites: Go 1.25+, Node.js 20+, Python 3.10+ (for the SDK). Or Docker.

```bash
git clone https://github.com/vanshjain-0702/DBX-Database-Extreme.git
cd DBX-Database-Extreme
make run-dev
```

Dashboard: [http://127.0.0.1:8000](http://127.0.0.1:8000). `make run-dev` logs you in as `admin` / `adminadminadmin`. `examples/quickstart.py` reads `DBX_ADMIN_PASSWORD` and falls back to that same string.

The product API is `TenantMemory.remember` / `recall` / `forget` — not a pile of Redis commands with a vector sidecar:

```python
from dbx import ControlPlane, TenantMemory

plane = ControlPlane("http://127.0.0.1:8000")
plane.login("admin", admin_password)

mem = TenantMemory.open(plane, "acme-corp")
mem.remember("session:42", '{"step": 1}')
mem.remember("doc:1", "customer prefers dark mode", vector=[0.1, 0.2, 0.9])
print(mem.recall([0.1, 0.2, 0.8]))
mem.forget("doc:1")
plane.shred("acme-corp")
```

`open` provisions if needed and mints a writer key. `recall` is ANN over *that tenant's* index. `shred` is `DeleteTenant(purge=true)`: on `strict`, the wrapped DEK is zeroed first, then the directory goes away.

The same 15-minute path lives in [`examples/quickstart.py`](https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/examples/quickstart.py). LangChain without an OpenAI key: [`examples/langchain-rag`](https://github.com/vanshjain-0702/DBX-Database-Extreme/tree/main/examples/langchain-rag). Session JSON from Next.js: [`examples/nextjs-cache`](https://github.com/vanshjain-0702/DBX-Database-Extreme/tree/main/examples/nextjs-cache).

### Your existing RESP clients already work

DBX speaks RESP on public `:6380`. `redis-py`, `ioredis`, and `go-redis` connect without a custom driver. That is an on-ramp — the cost of trying DBX is "AUTH, then SET." It is **not** a claim that DBX substitutes for a tuned Redis cluster.

The first command on `:6380` must be `AUTH tenantID:keyID secret`. Mint keys from the dashboard **Tenant keys** page or `POST /api/v1/tenants/{id}/keys`. The secret is shown once. A **reader** can `GET` and `VSEARCH` and cannot `SET` or `VADD`. Orchestrator tenants have no default superuser.

```python
from dbx import DBXClient

db = DBXClient(
    host="localhost",
    port=6380,
    tenant="acme-corp",
    key_id="key-id",
    secret="one-time-key-secret",
)
db.set("session:42", '{"thread": "onboarding", "step": 3}')
db.vadd("memories", "doc:1", [0.1, 0.2, 0.9])
db.vsearch("memories", [0.1, 0.2, 0.8], top_k=5)
```

```typescript
import { createClient } from 'redis';

const client = createClient({ url: 'redis://localhost:6380' });
await client.connect();
await client.sendCommand(['AUTH', 'acme-corp:key-id', 'one-time-key-secret']);
await client.set('session:abc', JSON.stringify({ userId: 42 }));
```

Point one connection at one tenant. Do not share a pool across customers.

## The lifecycle that defines the product

Everything else is a data-plane detail. These are the calls that make "tenant" a first-class object:

```bash
# Provision an isolated engine
curl -X POST http://localhost:8000/api/provision \
  -H "Authorization: Bearer $DBX_TOKEN" \
  -d '{"id": "acme-corp", "name": "Acme Corp"}'

# Back up one customer, not the cluster
curl -X POST http://localhost:8000/api/tenants/backup \
  -H "Authorization: Bearer $DBX_TOKEN" \
  -d '{"id": "acme-corp"}'

# Restore that checksummed archive
curl -X POST http://localhost:8000/api/tenants/restore \
  -H "Authorization: Bearer $DBX_TOKEN" \
  -d '{"id": "acme-corp", "path": "data/backups/backup_acme-corp_....dbx.zip"}'

# Off-boarding is one call. purge=true erases that directory.
curl -X POST http://localhost:8000/api/tenants/delete \
  -H "Authorization: Bearer $DBX_TOKEN" \
  -d '{"id": "acme-corp", "purge": true}'
```

Per-tenant cost is `GET /api/v1/tenants/{id}/usage`. Prometheus is `GET /metrics` on the orchestrator (Bearer JWT or `DBX_INTERNAL_API_TOKEN`). Hibernate stops the worker and leaves ciphertext on disk; wake brings it back.

## Isolation Kernel, in one paragraph

On Linux production (`DBX_ISOLATION_MODE=strict`) a tenant is a sealed `dbx-server` process:

1. **Filesystem.** After bind, Landlock so sibling `$DATA/tenants/{other}/…` cannot be `open()`'d. The kernel returns `EACCES`.
2. **Crypto.** A 256-bit DEK, wrapped by `DBX_KEK`, seals WAL frames, KV checkpoints, vector ids, and the HNSW graph. The worker never sees the KEK. `purge=true` shreds the wrap file first.
3. **IPC.** RESP/HTTP listen on Unix sockets mode `0600`. `SO_PEERCRED` accepts only the orchestrator PID.

Firecracker remains a stronger sandbox. DBX does not claim otherwise. What those systems do not do is make **per-customer memory plus vector search** the sealed unit, at a density you can host on one node.

Honest limits, because they are part of the design:

- SQ8 `.vec` rows stay mmap'd **plaintext** so idle tenants live in page cache. Embedding confidentiality at rest is LUKS/fscrypt. DBX encrypts the searchable surface (ids, graph, WAL, checkpoints).
- Landlock governs file opens, not `connect()` or `stat()`. Socket isolation is `SO_PEERCRED`.
- Data in use is plaintext inside the worker.
- Certified ANN search p50 at 100k × 128 is **2.304 ms**, not sub-millisecond.

The full walkthrough — Go Landlock, `LockOSThread`, DEK wrap, peer-PID accept loop — is [Building Secure Multi-Tenant AI Memory in Go](https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/blog/building-secure-multi-tenant-ai-memory.md). Source of truth: [`docs/isolation.md`](https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/isolation.md).

## What isolation is allowed to cost

Benchmarks exist to prove isolation is not expensive — not to rank DBX against a tuned cluster of something else. Certification host (27 August 2026, Windows 10, 18 logical processors, 15.6 GiB RAM; Linux CI re-runs the harness):

| Gate | Measured |
|---|---|
| Vector ingest (100k × 128, batches of 1,000) | 7,233 vec/s |
| ANN search p50 / p95 / p99 | 2.304 / 3.132 / 3.730 ms |
| Recall@10 mean / p05 (SQ8 vs float32 brute force) | 0.920 / 0.800 |
| Strict idle worker RSS | ~14–17 MiB |
| Preview profile | 100 tenants/node, 100k vectors/tenant |

v1 is cleared for **single-node production** under that profile: durable strings + vectors. Optional async WAL replicas can sit beside the primary (the primary still acks locally). Cluster/sharding, data-plane Raft, and non-string RESP mutation families still fail closed.

The metric that matches the business model is **cost per active tenant**, not peak ops/s on a shared instance. Quantized mmap rows are how idle tenants stay nearly free: payload is roughly a quarter of float32, and a tenant nobody queried today lives in page cache instead of resident RAM.

## What DBX is deliberately not

| If you need… | Use that instead |
|---|---|
| Peak single-instance KV throughput for one huge shared workload | Redis / Dragonfly |
| Billion-vector ANN, sharding, heavy filtering | Qdrant / Milvus |
| A fully managed, zero-ops vector service | Pinecone |
| Joins, SQL, a system of record | Postgres (+ pgvector) |

DBX sits *in front of* the system of record and *underneath* the agent. Indexes are sized for per-tenant working sets. Embeddings never leave your VPC: one self-hosted binary, dashboard compiled in, no sidecar control plane.

License is BSL 1.1: free to self-host, including inside your own commercial SaaS. Not a license to offer DBX itself as a managed service to third parties. Converts to Apache 2.0 after four years.

## Run it

{% embed https://github.com/vanshjain-0702/DBX-Database-Extreme %}

```bash
git clone https://github.com/vanshjain-0702/DBX-Database-Extreme.git
cd DBX-Database-Extreme
make run-dev          # laptop
# or
make docker-up        # Compose defaults to strict on Linux
```

Then `python examples/quickstart.py`. Watch the 5-minute product walkthrough if you want the dashboard first.

{% cta https://github.com/vanshjain-0702/DBX-Database-Extreme %}
Clone the DBX repo
{% endcta %}

{% cta https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/ %}
Read the technical docs
{% endcta %}

- Source: [github.com/vanshjain-0702/DBX-Database-Extreme](https://github.com/vanshjain-0702/DBX-Database-Extreme)
- v1.1.0: [releases/tag/v1.1.0](https://github.com/vanshjain-0702/DBX-Database-Extreme/releases/tag/v1.1.0)
- Docs: [vanshjain-0702.github.io/DBX-Database-Extreme/docs](https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/)
- Positioning: [`docs/positioning.md`](https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/positioning.md)
- Isolation Kernel: [`docs/isolation.md`](https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/isolation.md)
- Quickstart: [docs/quickstart.html](https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/quickstart.html)
- Walkthrough video: [demo.html](https://vanshjain-0702.github.io/DBX-Database-Extreme/demo.html)

If you have one workload and one tenant, you probably do not need this. If you have five hundred customers who each need memory, the tenant is the database.
