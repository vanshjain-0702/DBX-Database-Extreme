# Design partners runbook (Phase 2)

**Gate:** MVP app + isolation demo pass locally (`python demo_isolation.py`).  
**Goal:** 1–3 real firms or legal startups on **strict Linux isolation** before any public cloud.

Do not start public signup or multi-region DBX Cloud in this phase.

## Partner profile

| Fit | Disqualify |
|---|---|
| Boutique firm / legal ops with 5–50 clients | Needs Petabyte e-discovery |
| Cares about client confidentiality / data residency | Wants shared vector DB with filters |
| Will upload real (or redacted) contracts | Will not run a weekly feedback call |

Target: **3 conversations → 1–2 active partners**.

## Environment (non-negotiable for real data)

- Linux host or Linux VM (not Windows) with Docker or bare `dbx-orchestrator`
- `DBX_ISOLATION_MODE=strict`, `DBX_KEK` set, control-plane TLS (no `-insecure-http` for partner data)
- Legal Auditor on the same private network; firm users authenticate to Legal Auditor only
- Backups: practice `export` + restore drill once before partner data lands

Checklist:

- [ ] Isolation demo green on the partner host
- [ ] TLS on `:8000` / RESP as required by production profile
- [ ] `make soak` or density sanity on that hardware
- [ ] Written data-processing note (who holds keys, retention, purge)

## Onboarding script (60 minutes)

1. Create firm account in Legal Auditor.
2. Provision **two** demo clients; show tenant ids differ.
3. Ingest one redacted MSA each; run checklist queries.
4. Run the verbal isolation claim: “Atlas cannot see Northwind.”
5. Export one client; purge the other; show sibling still serves.
6. Agree success metric for week 2 (e.g. 10 real matters ingested).

## Success metrics to record

| Metric | How to measure | Partner pass |
|---|---|---|
| Time to first audit | Clock from signup | < 15 min |
| Cross-tenant leak | Isolation demo + spot checks | Zero |
| Offboard | Export + purge one matter | < 10 min, no sibling impact |
| Willingness to expand | Verbal / email | “Yes, add N more clients” |

## Outreach template (short)

> We built Legal Auditor on DBX so each of your clients is a sealed memory engine—not a shared index with a client_id prefix. Looking for 1–2 design partners to run contract clause audits on a private Linux node (your VPC or ours). No public cloud yet. 60-minute setup, weekly feedback for four weeks.

## Exit criteria → Phase 3 (brand)

Activate [BRAND_PLAYBOOK.md](BRAND_PLAYBOOK.md) only when **at least one** of:

- A partner renews / expands beyond the pilot, or
- Written intent to pay (LOI / invoice), or
- Two partners complete the isolation + offboard drill successfully

Until then: keep one brand surface (DBX + Legal Auditor as a folder/product), no second company.
