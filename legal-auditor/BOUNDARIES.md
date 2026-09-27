# Parent / child contract — DBX ↔ Legal Auditor

## Hard rule

**One end-client (or matter) = one DBX tenant.**  
Legal Auditor never stores multiple clients’ corpora in a shared index with name prefixes.

## What DBX (parent) owns

- Engine, Isolation Kernel, RESP ingress, WAL, HNSW, quotas
- Provision, keys, usage, backup/export, hibernate, purge/shred
- Density economics (cost per active tenant)

## What Legal Auditor (child) owns

- Firm signup / simple auth
- Mapping `firm_user → clients → dbx_tenant_id`
- Document ingest UX, chunking, embedding choice
- Audit checklists, reports, citations UI
- GTM to law firms; pricing for the vertical product

## Decision test

| Question | Goes to |
|---|---|
| Can a neighbor read this client’s vectors? | DBX |
| How do we label “indemnity” clauses? | Legal Auditor |
| How do we delete one client’s data? | DBX (`purge`) |
| How do we price seats for lawyers? | Legal Auditor |

## Future children

Support Copilot, Medical Memory, and others must reuse the same contract: thin app + thick DBX. Do not fork the engine per vertical.
