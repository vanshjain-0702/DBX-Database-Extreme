<p align="center">
  <img src="dashboard/src/assets/logo.jpg" alt="DBX Logo" width="80" />
</p>

<h1 align="center">DBX</h1>

<p align="center">
  <strong>The All-in-One AI Database. Redis-compatible KV Store + Native Vector Search.</strong>
  <br />
  Multi-tenant. Blazing fast. Built for the AI era.
</p>

<p align="center">
  <a href="https://github.com/vanshjain-0702/DBX-Database-Extreme/actions"><img src="https://github.com/vanshjain-0702/DBX-Database-Extreme/actions/workflows/build-and-test.yml/badge.svg" alt="Build Status" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-BSL%201.1-orange.svg" alt="License" /></a>
  <a href="https://github.com/vanshjain-0702/DBX-Database-Extreme/releases"><img src="https://img.shields.io/github/v/release/vanshjain-0702/DBX-Database-Extreme" alt="Release" /></a>
  <img src="https://img.shields.io/badge/go-1.25+-blue.svg" alt="Go Version" />
</p>

---

## Why DBX?

Modern AI applications require **two separate infrastructure pieces**:

| Problem | Traditional Solution | Cost |
|---|---|---|
| Session cache & fast KV | Redis | $30–$150/month |
| Vector/Embedding search | Pinecone, Qdrant | $70–$300/month |

**DBX eliminates both.** It is a single binary that speaks the Redis protocol for KV operations *and* provides native HNSW-indexed vector similarity search — in the same in-memory engine, with the same connection string.

---

## Features

- ⚡ **Redis-Compatible** — Drop-in replacement for all Redis string, hash, list, set, and sorted-set operations. No SDK changes needed.
- 🧠 **Native Vector Search** — Built-in HNSW index for embedding storage and ANN similarity queries. Designed for RAG, LLM memory, and semantic caching.
- 🏢 **Multi-Tenant Orchestrator** — Provision isolated database namespaces via API in milliseconds. Built on HashiCorp Raft for consensus.
- 🔐 **Production Security** — JWT authentication, bcrypt admin passwords, per-IP rate limiting, DoS body-size limits, and TLS termination.
- 📊 **Beautiful Dashboard** — A full-featured, dark-mode admin UI with real-time metrics, a data explorer, vector playground, and a `CMD+K` command palette.
- 💾 **Durable Persistence** — Write-Ahead Log (WAL), point-in-time snapshots, S3 backup integration.
- 🐳 **Deploy Anywhere** — Single binary, Docker, or Kubernetes (Helm charts included).

---

## Quickstart

### Option 1: Docker (Recommended)

```bash
docker run -p 8000:8000 \
  -e DBX_ADMIN_PASSWORD=yourpassword \
  -e DBX_JWT_SECRET=yoursecretkey \
  ghcr.io/dbx/dbx:latest
```

Open the dashboard at **http://localhost:8000** and log in with `admin` / `yourpassword`.

### Option 2: Build from Source

**Prerequisites:** Go 1.25+, Node.js 20+

```bash
git clone https://github.com/dbx/dbx.git
cd dbx

# Build all binaries
make build

# Start in local dev mode
make run-dev
```

### Option 3: One-Click Docker Compose

```bash
git clone https://github.com/dbx/dbx.git
cd dbx
make docker-up
```

---

## Connect Your Application

### Python (AI / LangChain)
```python
from dbx import DBXClient

db = DBXClient(host="localhost", port=6380, token="your_dbx_token")

# Standard KV
db.set("user:1", '{"name": "Alice", "plan": "pro"}')
print(db.get("user:1"))

# Vector search
db.vset("doc:1", embedding=[0.1, 0.2, 0.9, ...], metadata={"text": "DBX is fast"})
results = db.vsearch(query_embedding=[0.1, 0.2, 0.8, ...], top_k=5)
```

### Node.js / TypeScript
```typescript
import { createClient } from 'redis'; // DBX is Redis-compatible!

const client = createClient({ url: 'redis://localhost:6380' });
await client.connect();
await client.set('session:abc', JSON.stringify({ userId: 42 }));
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     DBX Orchestrator (Port 8000)                │
│         Control Plane: JWT Auth, Tenant Routing, Raft           │
│─────────────────────────────────────────────────────────────────│
│  Tenant A (Port 8081)  │  Tenant B (Port 8082)  │  Tenant N... │
│  ┌──────────────────┐  │  ┌──────────────────┐  │              │
│  │  KV Engine       │  │  │  KV Engine       │  │              │
│  │  (Redis RESP)    │  │  │  (Redis RESP)    │  │              │
│  │  Vector Index    │  │  │  Vector Index    │  │              │
│  │  (HNSW)         │  │  │  (HNSW)         │  │              │
│  │  WAL + Snapshot  │  │  │  WAL + Snapshot  │  │              │
│  └──────────────────┘  │  └──────────────────┘  │              │
└─────────────────────────────────────────────────────────────────┘
```

For a deep technical dive, read the [Architecture Document](docs/architecture.md).

---

## Project Structure

```
dbx/
├── cmd/
│   ├── dbx-server/         # Storage node entry point
│   └── dbx-orchestrator/   # Control plane entry point
├── internal/
│   ├── engine/             # Core KV + HNSW Vector engine
│   ├── orchestrator/       # Multi-tenant Raft orchestrator
│   ├── protocol/           # Redis RESP3 parser
│   ├── persistence/        # WAL, Snapshots, S3 Backup
│   ├── security/           # ACL, Rate Limiting, Encryption
│   └── api/                # HTTP REST API handlers
├── dashboard/              # React + Vite admin dashboard
├── sdk/
│   └── python/             # Official Python SDK
├── examples/
│   ├── langchain-rag/      # LangChain + DBX RAG example
│   └── nextjs-cache/       # Next.js session caching example
├── deploy/
│   ├── docker-compose.yml  # Local development stack
│   └── kubernetes/         # Helm charts for production
└── docs/                   # Architecture and API reference
```

---

## Benchmarks

Tested on a local single-node environment with 50,000 KV operations and 10,000 vectors:

| Operation | DBX Performance (Single Node) |
|---|---|
| SET (string) | ~119,531 ops/sec |
| GET (string) | ~200,000 ops/sec |
| Vector Ingestion (VADD) | 10,000 vectors (128-dim) in 86.5s (Security Check Enabled) |
| ANN Similarity (VSEARCH) | ~604 Queries / Sec (p50: 1.65ms, p99: 23.88ms) |
| Security | 100% Passed (Rate Limiting, ACL, XSS, DoS, Path Traversal) |

---

## License

DBX is licensed under the [Business Source License 1.1 (BSL 1.1)](LICENSE).

- ✅ **Free to use** for production, personal, and commercial applications.
- ✅ **Free to self-host** for any purpose.
- ❌ **Cannot** be offered as a managed cloud database service without a commercial agreement.

The license converts to Apache 2.0 after 4 years.

---

## Contributing

We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and open an issue or pull request.

---

<p align="center">Built with ❤️ by the DBX team.</p>
