# DBX changelog

## v1.3.0 — 2026-10-02

DBX v1.3.0 brings live vector migration, historical search, framework adapters,
stronger worker isolation, encrypted replication, and scoped support recovery
to the per-tenant memory engine.

- **Vector operations:** WAL-durable `VMIGRATE` lifecycle and
  `VSEARCH ... AS_OF` over retained WAL history.
- **Integrations:** LangChain and LlamaIndex adapters, bounded batch ingest,
  RAG examples, Legal Auditor, and Jurix example applications.
- **Isolation and replication:** all-thread Landlock seals, Unix-only seccomp
  networking, quota-based worker memory limits, mutual replication
  authentication, and AES-256-GCM WAL frames.
- **Support:** tenant-allowlisted diagnostics, separate wake credentials,
  pre-action audit, cooldown and post-checks, stable log codes, local redacted
  bundles, 43 support tests, and 1,340 synthetic drills.
- **Reliability and operator UI:** ordered concurrent WAL writes, tenant-local
  historical replay, durable migration recovery, tenant-start coordination,
  flexible HTTP command parsing, refreshed dashboard views, and release gates.
- **Version alignment:** engine commands and HTTP info, binaries, dashboard,
  SDK, Helm app version, deployment examples, and the official marketing site
  identify DBX as `1.3.0`.

Read the [full release notes and upgrade steps](docs/releases/v1.3.0.md),
[official announcement](https://dbxdb.co.in/posts.html#v1-3-0-2026-10-02), and
[source comparison](https://github.com/vanshjain-0702/DBX-Database-Extreme/compare/v1.2.0...v1.3.0).

## v1.2.0 — 2026-09-10

The recall release added `VSEARCH MIN_SCORE/EF/SPACE`, `VSIM`, `VFUSE`, Python
recall helpers, Semantic/Similar-to-id/Multimodal playground tabs, and the public
site on the existing Isolation Kernel.

[Release](https://github.com/vanshjain-0702/DBX-Database-Extreme/releases/tag/v1.2.0)
· [Historical announcement](https://dbxdb.co.in/posts.html#v1-2-0-2026-09-10).

## Earlier releases

The public [changelog](https://dbxdb.co.in/changelog.html) records v1.1.0's
Isolation Kernel and v1.0.0's supported single-node release.
