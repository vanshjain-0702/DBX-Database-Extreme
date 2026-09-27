# Legal Auditor — one-page PRD

**Parent:** DBX (per-tenant memory engine)  
**Child product:** Legal Auditor  
**Status:** MVP scaffold in this directory

## One-line

Per-client legal document memory and audit recall — each law-firm client (or matter) is one sealed DBX tenant.

## Buyer

Boutique firm legal ops, solo practitioners with multi-client corpora, or legal AI startups that need **structural** client isolation (not key prefixes).

## First workflow (MVP)

**Contract clause risk scan**

1. Firm creates a client → Legal Auditor provisions one DBX tenant.
2. Upload contract text (paste or `.txt` / `.md` for MVP; PDF later).
3. App chunks, embeds (caller-side), `VADD`s into that tenant only.
4. Auditor runs a checklist query (“indemnity cap”, “auto-renewal”, “governing law”).
5. Results return as citations from that client’s chunks only.

## Success metrics

| Metric | Target for design partners |
|---|---|
| Time to first audit | < 15 minutes from signup |
| Cross-client recall | Zero hits from sibling tenant on isolation demo |
| Offboard | Export + purge one client without touching others |

## Non-goals (explicit)

- Full contract lifecycle management (CLM)
- Petabyte e-discovery / litigation hold platforms
- Embedding models inside DBX (embeddings stay with the caller)
- Public multi-region “DBX Cloud” before partner proof
- Putting multiple clients in one tenant with `client_id:` prefixes

## Parent / child ownership

See [BOUNDARIES.md](BOUNDARIES.md).
