# Brand & corporate playbook (Phase 3+)

**Do not execute this playbook until** a design-partner signal exists
(see [DESIGN_PARTNERS.md](DESIGN_PARTNERS.md) exit criteria).

## Positioning

| Brand | Audience | Offer |
|---|---|---|
| **DBX** (parent) | AI product builders | Per-tenant memory engine; self-host free (BSL); commercial license if they offer managed DBX |
| **Legal Auditor** (child) | Law firms / legal ops | Vertical app: ingest, audit, export — powered by DBX underneath |

Buyers of Legal Auditor should not need to understand RESP. Buyers of DBX should not need legal clause taxonomies.

## When partner signal arrives — checklist

- [ ] Separate marketing URL (e.g. `legal.dbxdb.co.in` or `legalauditor.example`) pointing at the child app story
- [ ] Pricing for Legal Auditor in **matters / clients / seats** (maps to DBX tenants + usage API)
- [ ] Keep DBX site focused on the engine thesis ([docs/positioning.md](../docs/positioning.md))
- [ ] One legal entity is enough until ARR or a second vertical child ships
- [ ] Optional: invite-only hosted nodes **for Legal Auditor customers only** — not a public Pinecone-shaped DBX Cloud

## Pricing sketch (activate later)

| Plan | Billable unit | Backed by |
|---|---|---|
| Pilot | Fixed monthly + N clients | Cap tenants on one node |
| Growth | Per active client/matter | `GET /api/v1/tenants/{id}/usage` |
| Enterprise | Seats + residency + export SLA | Dedicated node, strict isolation |

Internal transfer: Legal Auditor “pays” DBX by infra cost per active tenant — same meter the parent will sell to other children.

## Second child (Support / Medical)

Only when Legal Auditor’s provision → ingest → audit → export → purge path is boring and automated. Reuse [BOUNDARIES.md](BOUNDARIES.md); do not fork the engine.

## Explicitly deferred

- Holding company / multiple LLCs on day one
- Public managed DBX Cloud for arbitrary third parties
- Claiming SOC 2 before partner process maturity
