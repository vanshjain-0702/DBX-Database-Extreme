# Future Upgrades Roadmap

This document tracks active upgrades for the DBX architecture.

### 1. "Shadow Migration" (Live Vector Space Updates)
- **The Problem:** Upgrading to a new AI embedding model (e.g., Ada to text-embedding-3) completely shifts the vector math space. Currently, developers must delete their entire database, re-embed millions of documents, and re-upload, causing massive downtime.
- **The DBX Solution:** Because DBX has a control plane (Orchestrator), a `VMIGRATE` command can be implemented. DBX will silently spin up a shadow HNSW graph in the background. As the developer streams in the new vectors, DBX answers live queries from the old graph. When 100% loaded, DBX performs an atomic pointer swap in memory for zero-downtime migrations.

### 2. Time-Traveling Vectors (Immutable MVCC Memory)
- **The Problem:** Standard vector databases overwrite old data. In AI auditing and compliance, you often need to know exactly what context the AI was operating on at a specific time in the past.
- **The DBX Solution:** DBX already uses a Write-Ahead Log (WAL). By implementing Multi-Version Concurrency Control (MVCC) for vectors, a command like `VSEARCH AS_OF <timestamp>` could be added. DBX would traverse the WAL backwards to run a similarity search on exactly what the data looked like at that specific moment in the past.

### 4. Zero-Cost Cold Storage (S3-Tiered Paging)
- **The Problem:** Keeping millions of idle vectors in RAM is cost-prohibitive. Competitors force users to choose between high-cost RAM or slow disk storage.
- **The DBX Solution:** Leverage the existing S3 integration to build "Tiered Paging." DBX automatically monitors tenant vectors. If nodes are untouched for 7 days, they are silently flushed to AWS S3. Upon a query hit, DBX streams them back into RAM on the fly, offering effectively "infinite" storage while keeping compute costs near zero.
