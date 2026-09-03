# DBX Positioning

This document is the source of truth for what DBX is, who it is for, and where we
deliberately refuse to compete. If a README, a landing page, a sales deck, or a feature
proposal contradicts this document, one of the two is wrong and it is usually not this one.

---

## 1. The one-sentence thesis

> **DBX is the per-tenant memory engine for AI products: one isolated store per customer,
> holding both their working state and their vector memory.**

Everything else — the RESP wire protocol, the HNSW index, the dashboard, the
public site in `website/` — is in service of that sentence.

---

## 2. Why we are not positioning against the giants

The instinct is to say "cheaper Redis + Pinecone in one binary." That framing loses, for
three reasons:

**It invites a comparison we cannot win.** Redis has fifteen years of C optimization and an
enormous ecosystem. Qdrant and Milvus have teams dedicated to billion-scale ANN. If the buyer's
mental model is "is this a better Redis?", every conversation becomes a benchmark argument
about someone else's strongest axis.

**It targets a buyer who is already served.** The team running one big shared cache is happy.
They are not shopping. Displacing a working Redis is a knife fight over a few dollars a month.

**It makes our real advantage invisible.** Per-tenant isolation is not a feature on the Redis
comparison chart, because Redis does not have that row. Framed as "Redis alternative," our
best property reads as an odd implementation detail instead of the reason to buy.

So we do not sell a replacement. We sell a **category the giants do not serve**: databases
where the tenant, not the cluster, is the unit of everything.

---

## 3. The customer and their pain

**Ideal customer profile:** a team shipping a product where each of their customers needs
their own memory.

- AI agent platforms where every end-customer's agent has private recall
- Vertical AI SaaS with a per-client RAG corpus (legal, medical, support, finance)
- On-prem or regulated deployments — one binary inside the customer's network
- Any team currently running a cache *and* a vector store and hand-rolling tenancy over both

**Disqualifiers:** one tenant, one workload; a need for billion-vector ANN; a need for SQL,
joins, or a system of record. We should say no to these quickly and honestly.

**What they feel today:**

| Pain | What it costs them |
|---|---|
| Tenancy is a key prefix | One missing prefix in one query is a cross-customer data leak |
| Two systems (cache + vector DB) | Dual writes, drift after crashes, two on-call surfaces, two bills |
| Backup is cluster-wide | Cannot restore or export a single customer |
| Deletion is a scan | "Delete my data" becomes a sprint instead of an API call |
| Shared memory pool | One noisy tenant evicts a quiet tenant's working set |
| Per-customer pricing is guesswork | No way to attribute cost or capacity to a customer |

Every one of those is a structural consequence of a database built around a shared cluster.
None of them is fixed by making that cluster faster.

---

## 4. Our unique selling propositions

These are the five claims we make. Each has to remain literally true in the codebase.

### USP 1 — The tenant is a first-class object
One API call creates a live, isolated engine: its own data directory, its own write-ahead log,
its own HNSW index, its own snapshot lineage, its own process-level state. Provision, back up,
and delete operate on exactly one customer. Isolation is structural, not a naming convention.

*Proof in code:* `internal/orchestrator/manager.go` (`Provision`, `DeleteTenant`,
`removeTenant`), `internal/server/instance.go`.

### USP 2 — State and memory in one engine
KV with TTL and HNSW vectors live in the same process, behind one connection, in one data
directory, and inside one backup archive. An agent's session and its semantic recall cannot
drift apart, because there is no second system to drift from.

*Proof in code:* `internal/query/executor.go` dispatches KV and vector commands through the
same executor; `internal/orchestrator/backup.go` archives the whole tenant directory.

*Honest limit:* the periodic `.rdb` snapshot serializes KV only — see
`internal/persistence/snapshot.go`, where vector entries are explicitly skipped because they
live in `.vec` mmap files. A single atomic KV+vector point-in-time image does not exist yet.
Do not claim one.

### USP 3 — Cost scales with active tenants, not signed tenants
Vectors are stored as 8-bit scalar-quantized rows in an mmap'd file, with asymmetric distance
computation at query time. Payload is roughly a quarter of float32, and an idle tenant lives in
page cache rather than resident RAM. That is what makes hosting many small tenants viable.

*Proof in code:* `quantizeVector` / `writeSQ8` in `internal/engine/vector.go`, ADC search in
`internal/engine/hnsw.go`.

### USP 4 — One self-hosted binary, operator UI included
Embeddings never leave the customer's network. The dashboard, console, data explorer, and
vector playground are embedded in the binary. There is no separate control-plane service to
deploy and no vendor to send data to.

*Proof in code:* `dashboard/` embedded via `cmd/dbx-orchestrator`.

### USP 5 — Isolation Kernel: a tenant is a sealed execution domain
On Linux production (`DBX_ISOLATION_MODE=strict`) a tenant is its own process,
Landlock filesystem, cgroup, envelope-encrypted durable files, and a Unix socket
that only the orchestrator PID can open. Neighbours cannot open its files, connect
to its sockets, or unwrap its key. Revoking the tenant DEK cryptographically shreds
data at rest without a scan.

*Proof in code:* `internal/isolation`, `internal/orchestrator/worker.go`,
`docs/isolation.md`.

*Honest limits:* `.vec` rows stay mmap'd and unencrypted so USP 3 survives —
embedding confidentiality at rest is an fscrypt/LUKS job, while DBX encrypts the
searchable surface (ids, graph, WAL, checkpoints). Landlock governs file opens,
not `connect()` or `stat()`; cross-tenant socket access is stopped by
`SO_PEERCRED`. cgroup limits need a host that delegates cgroup writes, which
most containers do not. Data in use is plaintext inside the worker. User
namespaces are not applied. Density under `strict` was re-measured at about
14–17 MiB RSS per idle worker. `inprocess` remains the local/CI density
profile and is not this USP. Production (TLS, or `DBX_PRODUCTION=1`) refuses
`inprocess` unless `DBX_ALLOW_INPROCESS=1`.
Windows/macOS run `standard` (encryption + Unix sockets) without Landlock.
Never say "the strongest sandbox ever built" — see `docs/isolation.md`.

**Not a USP, but our on-ramp:** DBX speaks RESP, so existing clients connect without a custom
driver. We describe this as "your clients already work," never as "drop-in Redis replacement."
It lowers the cost of trying DBX; it is not the reason to choose DBX.

---

## 5. Explicit non-goals

Saying these out loud is what keeps the product coherent.

| We do not compete on | The right tool | Why we stay out |
|---|---|---|
| Peak single-instance KV throughput | Redis, Dragonfly | Their design target; our cycles go to isolation |
| Billion-vector ANN, sharding, heavy filtering | Qdrant, Milvus | Our indexes are sized for per-tenant working sets |
| Fully managed, zero-ops vector service | Pinecone | We are software you run — deliberately |
| Relational modeling, joins, ACID across entities | Postgres (+ pgvector) | DBX is memory, not a system of record |
| Analytics and OLAP over history | ClickHouse, DuckDB | Wrong shape entirely |

DBX sits *in front of* the system of record and *underneath* the agent. That is the whole
territory we want.

---

## 6. How we talk about performance

Benchmarks exist to prove that isolation is not expensive — not to rank us against a tuned
cluster of something else.

**Rules for any performance claim we publish:**

1. State the hardware, the client, the connection count, and whether it was pipelined.
2. Never quote a competitor's number we did not measure ourselves under the same conditions.
3. Never derive a throughput figure from a latency percentile.
4. If a number is single-tenant, say so. Our interesting numbers are per-tenant density:
   how many active tenants fit on one node, and what an idle tenant costs.
5. Publish recall alongside vector latency. Quantization has a quality cost; hiding it makes
   the whole page untrustworthy.

The metric we actually want to own is **cost per active tenant per month**, because that is
the number our buyer's business model depends on.

---

## 7. Business model

- **Self-host, free, including inside your own commercial SaaS.** BSL 1.1. The people we most
  want using DBX are companies embedding it in their product; the license explicitly permits it.
- **Commercial license** for anyone offering DBX itself as a managed service to third parties.
- **The natural billable unit is the tenant**, which conveniently matches how our customers
  bill their own customers.
- Converts to Apache 2.0 after four years, so adopting DBX is not a permanent bet on us.

---

## 8. Sequencing: what makes this thesis real

Ordered by how much each one strengthens the positioning above, not by difficulty. Detail in
[ROADMAP.md](../ROADMAP.md).

1. **Single ingress port for all tenants.** Shipped on public `:6380`. Loopback backend
   listeners remain; the certified cap is 100 tenants/node.
2. **Per-tenant quotas and usage accounting.** Shipped. `GET /api/v1/tenants/{id}/usage`
   and orchestrator `GET /metrics` are the meter. CI density soak is 12/4; operators run
   `make soak`.
3. **Scoped credentials per tenant.** Shipped. Reader cannot SET/VADD (tested). Dashboard
   Tenant keys mints roles. Orchestrator tenants have no default superuser.
4. **Hot standby.** Async WAL replicas shipped; the primary acks locally. Data-plane Raft
   still fails closed.
5. **Published recall for quantized search.** Shipped on the certification host (recall@10
   mean 0.920 / p05 0.800). Linux CI re-runs the harness.

---

## 9. Language guide

| Say | Don't say |
|---|---|
| "Per-tenant memory engine for AI products" | "All-in-one AI database" |
| "Your existing RESP clients work" | "Drop-in Redis replacement" |
| "One isolated engine per customer" | "Multi-tenant namespaces" |
| "Cost per active tenant" | "Cheaper than Pinecone" |
| "Sized for per-tenant working sets" | "Scales to billions of vectors" |
| "Measured on one node, single tenant" | "Faster than Redis" |

If a claim requires naming a competitor to make sense, it is a weak claim. Rewrite it around
what the customer gets.
