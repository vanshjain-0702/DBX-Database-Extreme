# Jurix upgrade review

Reviewed on 2 October 2026. This is a product comparison and implementation record based on public product documentation and Jurix's local code and interface.

## What the prescan found

Jurix already had authenticated, matter-scoped document ingestion, DBX retrieval, four commercial checks, reviewer notes and Markdown export APIs. Its React interface left Run Audit and Upload Contract disconnected, displayed placeholder Checklists/Exports/Firm pages, and did not expose reviewer notes or clickable evidence. Matter creation read the wrong response field; client cards displayed an undefined client name. Connection and isolation indicators were hardcoded.

## Research and implemented upgrades

| Reference | Relevant public capability | Upgrade implemented in Jurix |
|---|---|---|
| [Luminance for Legal](https://www.luminance.com/solutions/department/legal) | Applying review criteria and surfacing missing protections and unusual terms | Eight first-pass checks, severity filters, document-specific missing-clause results and review follow-up suggestions |
| [Litera review and Kira](https://www.litera.com/capabilities/review) | Review grids, source-linked findings and exportable diligence reports | Document-by-check matrix, selected finding with source passage and full-document reader, review memorandum downloads |
| [Spellbook transactional lawyer workflow](https://help.spellbook.legal/en/articles/9079366-using-spellbook-as-a-transactional-lawyer) | Reusable review guidelines, preferred positions and fallback positions | Matter playbook presets, configurable thresholds and accepted governing laws, suggested reviewer follow-ups |

These mappings are our product-design interpretation of the sources. The vendors did not audit or endorse Jurix. Their hosted models and integrations were not added to Jurix.

## Review workflow

1. Sign in or register a firm account, then create a matter.
2. Add contract text, import a UTF-8 `.txt`/`.md` file, or add a sample MSA/DPA.
3. Set the matter's playbook, then run the audit.
4. Select a matrix cell or finding to inspect its evidence and source document.
5. Record Open, In review, Resolved or Accepted risk. Final decisions require a rationale. Add document-specific reviewer notes as needed.
6. Download the review memorandum from the matter or the Exports page.

Default firm standards are a 12-month fee-based liability floor, a maximum 30-day non-renewal notice window, a maximum 72-hour incident notice window and Delaware governing law. These are editable demonstration standards, not statutory requirements. The data-processing preset uses a 48-hour notice threshold and a focused selection of checks.

The four additional checks locate confidentiality, termination, assignment and return/deletion provisions. A match remains Medium risk pending counsel review; locating words does not establish that a protection is adequate. A missing result has no invented supporting quotation. Each document is checked separately, and the matter overview retains the most severe result for each check.

Reviewer decisions persist against an evidence fingerprint containing the result, source and applied policy. Rerunning unchanged evidence preserves decisions; changing the policy invalidates them while retaining decision history. Documents added after or during an audit make its results stale and block new decisions until rerun. Automated check results and human decisions remain separate.

New matters receive unique tenant provisioning identities even when different accounts use the same client name. Existing matter data is retained. The live health indicator reflects the API's actual DBX connection status; it is not a claim about infrastructure security certification.

## Run locally

From the repository root, run the DBX orchestrator using the existing local-development configuration described in [README.md](../README.md), then:

```powershell
python -m pip install -r legal-auditor/requirements.txt
python -m uvicorn app:app --app-dir jurix --host 127.0.0.1 --port 8091
```

The checked-in built frontend is served at `http://127.0.0.1:8091`. After frontend edits:

```powershell
cd jurix/frontend
npm.cmd ci
npm.cmd run build
```

For frontend development, `npm.cmd run dev` serves port 5173 and forwards `/api` to 8091. The frontend uses hash routes, so routes work with the static server.

## Validation

Run the targeted service and HTTP tests with:

```powershell
python -m pytest jurix/test_playbook.py jurix/test_workbench.py -q
```

The suite covers the original scoring samples, per-document evidence and missing clauses, configurable thresholds, persistence and invalidation of decisions, stale audits including documents arriving during an audit, account authorization, unique same-name provisioning and authenticated HTTP review/export flows. Tests use temporary stores and a fake DBX bridge. They require pytest and FastAPI's TestClient dependency (`httpx` in the current environment); live DBX validation is separate.

## Suggested next upgrades

- PDF/DOCX text extraction with page anchors and OCR confidence, preserving source files and versions.
- Document version comparisons and downloadable Word redlines, with explicit lawyer approval before applying edits.
- Obligation extraction and reminders, including reference dates, timezones and reviewer confirmation.
- Firm team membership, reviewer assignment and role-based access, backed by explicit invitations and authorization tests.
- Benchmarked clause extraction on representative contracts and approved jurisdiction-specific playbooks before broader compliance claims.

The current audit uses deterministic text heuristics and local retrieval. It does not perform exhaustive legal analysis, infer enforceability, supply authoritative legal research, or automatically alter a contract. PDF/Word extraction, OCR, scheduled reminders and external AI services remain future work.

## Requested trust features — 3 October 2026

The six proposals were assessed against Jurix's existing commercial document-review workflow. Reputation is best supported by verifiable behavior and clear limits; this implementation does not make a measured claim about customer trust, saved hours or revenue.

| Proposal | Decision and delivered behavior |
|---|---|
| Playbook-driven automated redlining | Implemented the explicit-rule and counsel-review portions: account-scoped reusable firm playbooks with immutable saved versions, custom agreement-term maxima and fixed monetary liability floors, NDA/all-document scope, preferred clause proposals, acceptance/rejection with rationale, and downloadable accepted-clause comparisons. Existing matter thresholds and checks remain available. The engine uses deterministic checks; natural-language LLM interpretation and tracked Word redlines are deferred. |
| Billing/time-entry auditor | Added a matter's **Pre-bill review** tab, UTF-8 CSV import, configurable client guidelines, possible block-billing and vague-narrative flags, selected hour thresholds and code membership checks for the original ABA 1997 Litigation/Activity sets. Saved review snapshots and Markdown exports retain entries, applied guidelines, reviewer and time. |
| Handwritten/multilingual OCR and translation | Deferred. No source-file/page preservation or reviewed OCR/translation provider is configured. A provider must be tested on representative FIRs and court documents in the firm's actual languages, with confidence and lawyer correction workflows. General OCR capability is not proof of handwriting support in every language; [AWS's current Textract documentation](https://aws.amazon.com/textract/faqs/) describes language restrictions. |
| Regulatory drift alerts | Deferred. Jurix has no curated live regulatory feed, jurisdiction/applicability metadata, effective-date reconciliation or scheduled monitor. A retrieval match cannot establish that a new law applies to an old agreement. Implement source/version tracking and reviewer-confirmed impacts before advertising automatic compliance alerts. |
| Explainable source-linked audit trails | Strengthened existing evidence review with verified exact-text line/character anchors, one-click highlighted source navigation and retained evidence/policy/proposal snapshots for each decision. Custom rules can carry firm-supplied HTTPS reference links. No statute or case citation is invented when the applicable basis is a firm standard. The history is application-level persistence, not a certified tamper-proof record. |
| Legal-specific voice dictation | Deferred. There is no configured transcription provider, legally evaluated lexicon or audio-handling workflow. A general speech recognizer would not demonstrate fine-tuning on legal terms or accurate statutory citations. |

### Firm playbooks and clause proposals

In **Findings → Matter playbook**, add an NDA term rule (24 months represents the requested two-year maximum) or a monetary cap rule (USD 50,000 represents the requested dollar floor). Configure scope and any counsel-approved preferred wording/reference. Standard checks can be disabled when only custom rules are required. Save the matter policy and rerun the audit. Save an independent version to the firm library to reuse it elsewhere; loading a saved version copies it into the matter editor and still requires saving the matter policy. **Checklists** lists saved versions. The library belongs to the current firm account; multi-user firm membership remains future work.

Custom term checks support explicit integer years/months, including common written numbers. Renewal, compound or conflicting wording requires manual review. Monetary checks compare one explicit fixed amount in USD, INR, EUR or GBP against the chosen floor. Fee-based caps, different currencies, multiple amounts, scaled amounts and detected exceptions require review; the app does not convert currencies. These are first-pass text patterns, not full clause interpretation. Non-NDA documents are skipped for an NDA-scoped custom rule; an audit with no applicable checks is rejected instead of reporting compliance.

For a flagged custom rule with preferred wording, record a proposal decision and rationale. **Accept for clause comparison export** approves inclusion in a review comparison; it does not resolve the risk or modify the source agreement. The export includes the original passage, firm wording and reviewer. Rejected/pending proposals are excluded. Stale audits cannot accept/export proposals. Evidence, rule, authority or wording changes invalidate prior acceptance on rerun while preserving the historical decision snapshot. After this upgrade, existing saved audits should be rerun to establish the expanded fingerprints.

Line anchors refer to the ingested plain text, not PDF page coordinates. An exact substring must be verified before an anchor is emitted. Missing clauses have no invented quotation or source anchor. Firm-supplied links are clearly labelled and require counsel verification; Jurix does not fetch them or independently validate their legal applicability.

### Billing review

Import or paste a CSV with this exact header:

```csv
date,timekeeper,hours,task_code,activity_code,narrative
2026-10-02,Associate,0.5,L120,A104,Reviewed liability provisions for supplier negotiation
```

Dates use `YYYY-MM-DD`; hours must be finite, positive and at most 24. Quote narratives containing commas or newlines. Each review accepts up to 500 entries / 1,000,000 characters. Configure the client's required code fields, minimum narrative words, block-billing check, per-entry/routine hour thresholds and routine phrases before review. A sample is available in the interface. Correct narratives/hours using actual timekeeper information and rerun to create a new snapshot. Earlier snapshots remain available in the selector; exports always use that snapshot's original guidelines, rather than any unsaved editor settings.

The [UTBMS resource maintained by the LEDES Oversight Committee](https://utbms.com/) distinguishes task and activity codes and notes that clients choose their coding requirements. The validator is explicitly limited to the [original ABA Litigation](https://utbms.com/aba-litigation-codes/) and [Activity](https://utbms.com/aba-activity-codes/) sets, including litigation phase codes. It does not validate every UTBMS set, LEDES invoice format, code-to-narrative semantics or client-specific custom codes. Billing flags are first-pass review prompts, not fee/ethics findings or invoice-acceptance guarantees. Task allocations and replacement narratives are never fabricated. The [ABA's billing guidance](https://www.americanbar.org/groups/litigation/resources/newsletters/commercial-business/5-billing-tips-young-lawyers/) supports clear descriptions and avoiding block billing; the chosen thresholds are product defaults, not ABA requirements.

Billing entries/snapshots persist in the existing matter-scoped local records store. This feature does not connect to a billing vendor, submit an invoice or send document/audio data to external AI providers.

### Added validation

```powershell
python -m pytest jurix/test_playbook.py jurix/test_workbench.py jurix/test_trust_features.py -q
cd jurix/frontend
npm.cmd run build
```

Validation completed: **79 service/HTTP tests passed**, and `npm.cmd run build` passed. The new tests cover rule ambiguity and scope, currencies, invalid inputs, exact/repeated/Unicode text anchors, firm version/account boundaries, proposal rationale/history/invalidation, unchanged source text, CSV quoting and malformed values, configurable billing flags, durable snapshots, exports and authenticated HTTP endpoints. Tests use fake DBX and temporary stores; they do not benchmark legal interpretation, an OCR/STT model, live regulatory monitoring or production billing-vendor compatibility.

An isolated local browser smoke check verified login, highlighted source navigation (including Unicode before a citation), proposal acceptance/export, firm version saves, policy staleness and rerun invalidation, sample billing flags, guideline customization and exporting a historical snapshot. Desktop and 390-pixel mobile checks passed with no page runtime errors or page-wide horizontal overflow. The in-app browser timed out during setup and the agent-browser CLI download was blocked by network restrictions; the check used the repository's existing Puppeteer dependency against a temporary fake-DBX server. Screenshots and the smoke result are local QA artifacts under `.cache/jurix-browser/`. Live DBX integration remains separate.
