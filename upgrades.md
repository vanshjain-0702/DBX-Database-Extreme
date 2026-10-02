# DBX operational upgrades

The [v1.3.0 release notes](docs/releases/v1.3.0.md) collect the changes since
v1.2.0 and describe deployment and replication upgrade steps.

## Shipped in v1.3.0

### Live vector migration

`VMIGRATE START/ADD/SWAP/CANCEL` builds a shadow vector index while the current
index remains searchable. Applications supply re-embedded vectors, then promote
a non-empty target or cancel the migration. Lifecycle operations are WAL-durable.
Migration needs temporary storage and is not yet a replicated WAL protocol.

### Historical vector search

`VSEARCH ... AS_OF <unix-nanoseconds>` replays retained vector WAL history into
a temporary index. This supports retrieval audits and incident reproduction.
Historical search is slower than current search and bounded by WAL retention.

### Scoped support and recovery

The optional support extension reads allowlisted tenant summaries and can
perform an explicitly enabled, audited wake of a hibernated tenant. Separate
credentials, server cooldowns, and client post-checks bound the action. See the
[support curriculum](docs/support-recovery-curriculum.md).

## Future work

Storage tiering and automatic cold-vector paging remain roadmap items. DBX
v1.3.0 does not implement transparent S3 paging. Further recovery actions require
case-specific deployed signals, preconditions, audit, limits, and post-checks.
