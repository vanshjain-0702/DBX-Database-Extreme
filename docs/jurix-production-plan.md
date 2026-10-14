# JURIX production implementation plan

Target: Docker on a private server. Preserve existing data and matter-to-DBX-tenant isolation.

1. Replace JURIX default identity/record persistence with transactional SQLite WAL and encrypted payloads/secrets; provide explicit dry-run/import/backup tooling for legacy JSON. Implement hashed expiring sessions, revocation, cookie authentication/CSRF, firm memberships and roles. Keep legacy test adapters compatible; do not rewrite the parent DBX product.
2. Add production configuration validation, input/body/time limits, secure HTTP headers, sanitised error IDs, safe health/readiness endpoints, login throttling and private DBX connections.
3. Add immutable source archives, bounded PDF/DOCX/text extraction, configured handwriting OCR with page/confidence provenance, editable extraction review and immutable version commits. Translation drafts retain source links and legal glossary instructions.
4. Add opt-in schema-constrained LLM playbook rules, verified source quotations, model/prompt provenance and reviewed proposals. Export accepted nonoverlapping changes as tracked Word redlines derived from ingested text. Preserve reviewer decisions and source files.
5. Add authoritative HTTPS regulatory-source watches, version hashes, scheduled checks and matter-specific possible-impact tasks with source/effective-date metadata and reviewer decisions. Require counsel confirmation of applicability.
6. Add explicit microphone/audio transcription with legal vocabulary, editable transcript/note drafts and no local audio retention. Configured provider capability and per-matter consent gate external processing.
7. Replace decorative dark/photo-heavy UI with original components informed by attributed Dribbble references: navy navigation, light document canvas, compact matter table, readable evidence/decision panels, responsive layouts and accessible controls.
8. Provide private-network Docker services, TLS reverse proxy, durable storage, health checks, worker, migration/backup/restore commands, pinned dependencies and CI. Validate security/account boundaries, extraction/redline/source verification, provider failure paths, watch scheduling and desktop/mobile user workflows. External-provider accuracy and deployed infrastructure need live acceptance verification before a public production-ready claim.

No existing firm data is deleted or automatically migrated during implementation. External AI credentials, jurisdiction/source subscriptions and language accuracy cannot be fabricated; configured integrations are tested with provider fixtures until authorised live credentials are supplied.
