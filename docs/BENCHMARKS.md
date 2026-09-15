# DBX Performance & Benchmarks

At DBX, we believe in **100% transparent and reproducible** benchmarks. We prioritize strict per-tenant isolation, but we absolutely refuse to sacrifice throughput to get there.

Because standard benchmarking tools (like older versions of `redis-benchmark`) sometimes struggle with DBX's strict two-argument `AUTH <tenant> <secret>` mechanism, we believe the most honest way to prove our performance is to **give you the exact benchmarking script we use**.

Don't take our word for it. Review the code, compile it, and run it on your own hardware.

---

## 1. Key-Value Performance (`redis-benchmark`)

The following benchmarks were recorded using the industry-standard `redis-benchmark` tool. To allow `redis-benchmark` (which uses a single-argument `AUTH`) to connect, these tests were run with DBX authentication temporarily disabled, directly against the plaintext port.

- **Client & Server:** Localhost (bypassing network latency).
- **Protocol:** Raw TCP on port `6380` (Insecure mode, Auth disabled).
- **Concurrency:** 64 concurrent connections.
- **Pipelining:** 64 commands per pipeline.
- **Total Operations:** 512,000 requests.

### The Results

| Operation | Total Ops | Throughput | p99 Latency |
|---|---|---|---|
| **SET** (String) | 512,000 | **75,583 ops/sec** | **< 19 ms** (24% < 1ms) |
| **GET** (String) | 512,000 | **77,458 ops/sec** | **< 14 ms** (23% < 1ms) |

### How to Reproduce
1. Start standalone DBX with `configs/local.yaml` (`auth.enabled: false` keeps the open NoPass `default` user):
   ```bash
   go run ./cmd/dbx-server -config configs/local.yaml
   ```
2. Run `redis-benchmark`:
   ```bash
   redis-benchmark -p 6380 -c 64 -P 64 -t set,get -n 512000
   ```

Or use the in-repo RESP harness (works with passworded or open local profiles):
```bash
go run ./cmd/dbx-benchmark -addr 127.0.0.1:6380 -workers 64 -per-worker 8000
```

---

## 2. Vector Database Performance (ANN Benchmarks)

DBX features an integrated 8-way sharded HNSW graph for vector search. We benchmark this using our open-source `dbx-vector-benchmark` harness, which measures ingest throughput, search latency, and Recall@10 against a float32 brute-force baseline.

- **Dataset:** 100,000 vectors, 128 dimensions
- **Queries:** 50 deterministic recall queries, `k=10`
- **Quantization:** SQ8 (8-bit scalar quantization)

### The Results

| Metric | Measured |
|---|---|
| **Vector Ingest** | **6,838 vectors/sec** |
| **ANN Search (p50)** | **2.006 ms** |
| **ANN Search (p95)** | **7.011 ms** |
| **ANN Search (p99)** | **9.383 ms** |
| **Recall@10 (Mean)** | **0.920** |
| **Recall@10 (p05)** | **0.800** |

### How to Reproduce
Run the built-in vector benchmark harness from the DBX repository:
```bash
go run ./cmd/dbx-vector-benchmark/main.go -count 100000 -dim 128 -queries 50 -k 10
```

---

## 3. Isolation density soak

Operators should also run the certified noisy-neighbor drill:
```bash
go run ./cmd/dbx-soak -idle 100 -active 25 -for 3s
```

This measures idle GET latency while other tenants write. It is independent of the network listener.
