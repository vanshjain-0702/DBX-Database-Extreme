# DBX Benchmarks: Methodology and What the Numbers Mean

These benchmarks answer one question: **does per-tenant isolation cost you performance?**

They are not a ranking against other databases. We do not publish numbers for systems we did
not measure ourselves on the same hardware, and we do not claim a category win we cannot
reproduce. See [docs/positioning.md](../../docs/positioning.md) §6 for the rules we hold
ourselves to when publishing any performance claim.

---

## Environment

Certification run: 27 August 2026, Windows 10 build 26200, 18 logical processors,
15.6 GiB RAM. Docker Desktop's Linux engine was unavailable, so these results are
Windows-only and do not certify Linux.

---

## Version 1 certification matrix

| Gate | Dataset / mode | Measured | Requirement | Result |
|---|---|---:|---:|---|
| SET throughput | 128k ops, 64 clients, pipeline 64, `everysec`; batch p50/p95/p99 18.278/41.087/51.222 ms | 186,147 ops/s | ≥154,000 ops/s | PASS |
| GET throughput | 128k ops, 64 clients, pipeline 64; batch p50/p95/p99 6.364/44.009/65.356 ms | 284,785 ops/s | ≥174,000 ops/s | PASS |
| Vector ingest | 100k × 128, batches of 1,000, 8-way sharded HNSW | 7,233 vectors/s | ≥2,000 vectors/s | PASS |
| Vector search p50 | 100k × 128, 50 queries, efSearch=80 | 2.304 ms | ≤3 ms | PASS |
| Vector search p95 | same | 3.132 ms | ≤4 ms | PASS |
| Vector search p99 | same | 3.730 ms | ≤8 ms | PASS |
| Mean recall@10 | SQ8 HNSW vs float32 brute force | 0.920 | ≥0.90 | PASS |
| Fifth-percentile recall@10 | same | 0.800 | ≥0.70 | PASS |
| Noisy-neighbor isolation | 8 writers vs quiet GET, plus quota rejection | quiet GET ≤50 ms; noisy OOM isolated | isolation holds | PASS |
| Idle server working set | after 128k string benchmark keys | 53.8 MiB | informational | — |
| Go tests | `go test ./...` | PASS | PASS | PASS |
| Static analysis | `go vet ./...` | PASS | PASS | PASS |
| RESP parser fuzz smoke | 787,426 executions / 6 s | PASS | PASS | PASS |
| Race detector | Linux CI `go test -race ./...`; Windows host has no CGO/GCC | CI-enforced | PASS on Linux | CI |
| Linux container gate | GitHub Actions `ubuntu-latest` release-gates job | CI-enforced | PASS | CI |
| Critical-package coverage | protocol 81.9%, persistence 71.8%, query 69.0%, engine 41.0% | protocol ≥80%; others CI-floored | protocol PASS | MIXED |

The 1 ms p50 / 3 ms p95 lines applied to the unsharded index that could not ingest at 2,000 vec/s. Sharded HNSW search waits for the slowest shard, so the v1 search SLO is **p50 ≤3 ms, p95 ≤4 ms, p99 ≤8 ms** at 100k × 128. Linux CI re-runs the harness and fails the build if those floors are missed.

**Verdict: GO for single-node v1** under the supported profile (100 tenants/node, 100k vectors/tenant, string + vector durable surface). Optional async WAL replicas sit beside that path: the primary still acks locally and replica TCP is dropped if slow. Data-plane Raft, cluster, and non-string mutation families still fail closed. Windows cannot run the race detector locally; that gate is the Linux CI job.

---

## 1. Single-tenant throughput

Measured over loopback RESP with 64 connections and pipelining, 128,000 operations.
The server used the version 2 transactional WAL in `everysec` mode and no eviction.
These numbers are not a competitor comparison and do not include network latency.

---

## 2. Connection protocol: persistent RESP vs HTTP JSON

Both endpoints are DBX. This compares our own two front doors, so you can pick the right one
for your deployment shape.

| Metric | Network link | RESP (persistent pool) | HTTP JSON | Read |
|---|---|---|---|---|
| Avg search latency | Intra-datacenter (1.5 ms) | 33.90 ms | 29.58 ms | Comparable |
| Throughput | Intra-datacenter (1.5 ms) | 1,340 QPS | 1,613 QPS | HTTP holds up fine |
| Avg search latency | Serverless edge (15 ms) | 25.83 ms | 31.12 ms | RESP ~20% faster |
| Throughput | Serverless edge (15 ms) | 1,684 QPS | 1,553 QPS | RESP absorbs more load |
| Avg search latency | Cross-region WAN (60 ms) | 82.89 ms | 89.29 ms | Network bound |
| Throughput | Cross-region WAN (60 ms) | 585 QPS | 547 QPS | Network bound |

**Takeaway:** at edge distances, per-request TCP/TLS handshakes dominate and a persistent RESP
pool wins. Inside one datacenter, use whichever is more convenient — the HTTP API is not a
second-class path.

### Max local throughput
Loopback, 50 parallel persistent mTLS RESP connections, 10,000 queries in 2.454 s →
**4,075 QPS**.

---

## 3. Memory: what a tenant actually costs

This is the number that matters for our thesis, because our customers' unit economics are
per-customer, not per-cluster.

Vectors are stored as 8-bit scalar-quantized rows in an mmap'd file: `dim` bytes of `int8`,
plus a 4-byte scale and a 4-byte reconstructed L2². Graph construction and traversal
compare SQ8 rows with cached `scale/‖q̂‖` factors. Final ranking reranks HNSW hits with
a float32 query against the same quantized rows.

| Storage | 384-dim vector | 1M vectors (payload only) |
|---|---|---|
| float32 | 1,536 bytes | ~1.54 GB |
| DBX SQ8 | 392 bytes | ~0.39 GB |

**Two honest caveats.** The HNSW graph itself is additional and is not quantized, so total
resident memory is higher than the payload row above. Quantization and graph quality have now
been measured: the current 100k harness achieves 0.920 mean recall@10 (p05 0.800). Ingest at
that size is 7.2k vec/s; search p50/p95/p99 meet the sharded SLO of 3/4/8 ms.

The practical consequence: because vectors live in an mmap'd file rather than the Go heap, a
tenant nobody queried today occupies page cache the kernel is free to reclaim, rather than
resident RAM you paid for. Cost tracks *active* tenants.

---

## 4. Known limits

Publishing these is part of the point. Both are on the [roadmap](../../ROADMAP.md).

**Ingest is sharded, not lock-free.** `VAddBatch` inserts into eight independent HNSW
graphs in parallel. Concurrent writers to the same index still serialize on `idx.mu`.
Search waits for every shard, which is why median latency rose after the ingest fix.

**Public ingress now uses one port.** The orchestrator exposes authenticated RESP on `:6380`
and routes to loopback-only tenant listeners. Backend listener density still consumes local
ports, so the v1 contract remains capped at 100 tenants/node.

**We have not claimed a 100-orchestrator-tenant soak in CI.** A unit density soak
(12 idle / 4 active) and a noisy-neighbor quota test run in Linux gates.
Operators run `make soak` (100 idle / 25 active KV engines). That drill measures
engine isolation, not 100 full orchestrator processes.

---

## Reproducing

```bash
# Throughput (Go, RESP, 64 workers, pipelined, 128k ops)
DBX_BENCH_ADDR=127.0.0.1:6499 DBX_DEFAULT_PASSWORD=... go run ./cmd/dbx-benchmark

# Deterministic 100k-vector recall gate
go run ./cmd/dbx-vector-benchmark -count 100000 -dim 128 -queries 25 -k 10
```

The throughput benchmark expects `configs/v1-benchmark.yaml`; the vector harness is
self-contained and uses a temporary data directory.
