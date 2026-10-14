# Jurix

Jurix is a matter-based commercial contract review workspace backed by DBX. It provides a document diligence matrix, source-linked findings, reusable firm playbooks with explicit numeric rules, reviewer decisions and clause proposals, pre-bill narrative/code checks, and downloadable Markdown reports.

See [the upgrade review](../docs/jurix-upgrade-review.md) for the public product research, implemented features, validation and further recommendations.

Start the existing DBX orchestrator, then run from the repository root:

```powershell
python -m pip install -r jurix/requirements.txt
python -m uvicorn app:app --app-dir jurix --host 127.0.0.1 --port 8091
```

Open `http://127.0.0.1:8091`, register or sign in, create a matter, add a document or a sample, and run its audit. Imports support UTF-8 text/Markdown, text PDFs and DOCX; reviewers verify extracted text before committing it. OCR, translation, semantic rules and dictation require configured providers and explicit matter consent. Review checks are a first pass against firm-defined standards and require lawyer verification.

In **Findings → Matter playbook**, add NDA term or monetary liability rules and optional firm preferred clauses/references. Save a version to the firm library to reuse it in another matter. Reviewers accept or reject clause proposals with a rationale; accepted proposals export as before/after comparisons for counsel to apply. Source agreements remain preserved.

The **Pre-bill review** matter tab accepts UTF-8 CSV time entries and checks narratives, client-configured hours thresholds and the ABA 1997 Litigation/Activity code sets. Reviews retain snapshots and export reports. These checks require billing partner verification and do not guarantee invoice acceptance. Accepted anchored clause proposals can export as text-derived Word tracked changes; original document formatting is not reproduced. Regulatory watches compare configured authoritative publications and create tasks for counsel to assess; the worker refreshes due sources periodically.

The frontend source is in `frontend/`; its production build is served from `static/`. Files under `previews/` are design references with sample data and are not deployed:

```powershell
cd jurix/frontend
npm.cmd ci
npm.cmd run build
```

The browser uses same-origin API requests, HTTP-only session cookies and CSRF verification for writes. `/api/health` reports public liveness; `/api/readiness` requires a session and checks current DBX connectivity. Capability flags come from server configuration. Client data, document versions, findings and review history come from the backend; screens do not supply production sample records.

Backend configuration uses the existing `DBX_CONTROL_URL`, `DBX_RESP_HOST`, `DBX_RESP_PORT`, `DBX_ADMIN_USER` and `DBX_ADMIN_PASSWORD` environment variables. Default accounts and records now use encrypted SQLite under `jurix/data/`; production requires an absolute `JURIX_DATABASE_PATH` and distinct encryption/audit keys. Legacy JSON registries require explicit migration with `migrate.py`; starting the app does not import them automatically.

Run the backend checks in an isolated Python environment:

```powershell
python -m pip install -r jurix/requirements-dev.txt
python -m pytest jurix --basetemp .cache/jurix-browser/tests
```

See [the private-server deployment guide](deploy/README.md) for configuration, build, startup, migration and backup commands, and [the frontend readiness report](../docs/jurix-frontend-readiness.md) for verification results and remaining live-environment checks.
