# DBX Support Extension (preview)

A manually started support utility that connects to DBX's dedicated support
API. It does not install a background service, create a
startup task, connect to tenant RESP, or access stored values/vectors. When the
command exits, the support session is passive. The read and optional wake
capabilities remain in process memory for that session and are not written to
disk.

This preview coordinates five deterministic local specialist checks: tenant lifecycle, capacity,
error trends, control-plane consistency, and the support-data scope guard. A
casebook analyst attaches DBX regression-test references, and a central manager
sorts their findings. The watch-mode learner stores bounded, local-only
signatures for unclassified symptoms. It stores no tenant IDs, error messages,
values, vectors, or credentials.

The casebook includes known operational and persistence symptoms with DBX
regression references. Use `dbx-support cases` to inspect it. The casebook is
curated seed knowledge, not a model trained on every DBX defect. The learner
records observations for later reproduction; it cannot infer or enable a fix.
The only DBX recovery action is an optional, one-time wake of tenant IDs that
the operator explicitly allowlists for a running `watch` session. DBX checks its
own tenant allowlist, writes an audit event before acting, and applies a
per-tenant cooldown. The extension verifies the result and writes a local audit
record too. It cannot repair crashes, WAL corruption, vector correctness
defects, or code.

## Requirements and install

- Python 3.10 or newer.
- A DBX support read capability with a server-configured tenant allowlist.
- For optional wake recovery, a separate support wake capability and server
  allowlist. Wake permissions must be a subset of read permissions.
- HTTPS for remote DBX deployments. HTTP is accepted only for loopback DBX.

Configure the DBX server with distinct random values for
`DBX_SUPPORT_READ_TOKEN` and, if recovery is wanted,
`DBX_SUPPORT_WAKE_TOKEN`. Set `DBX_SUPPORT_READ_TENANTS` to the exact
comma-separated tenant IDs this support installation may inspect. Set
`DBX_SUPPORT_WAKE_TENANTS` to the smaller set it may wake. Generate tokens
with a secret manager or `python -c "import secrets; print(secrets.token_urlsafe(48))"`.
Do not reuse the DBX operator JWT, internal API token, or tenant data-plane key.
The matching environment variables are wired into the Compose and Kubernetes
deployment examples. Rotate a capability by replacing its secret and restarting
the orchestrator.

From this directory:

```powershell
python -m pip install .
dbx-support --help
```

To run from source without installing:

```powershell
python -m dbx_support --help
```

Run the extension's contract, fault-injection, and diagnostic tests from this
directory. The Go integration gate runs the actual Python client against an
in-process DBX support HTTP server and starts a synthetic tenant engine.

```powershell
python -m unittest discover -s tests -v
```

From the repository root, run the scoped API and Python recovery integration:

```powershell
go test -count=1 -v ./internal/orchestrator ./internal/server -run 'Support|RecoverTenantTask'
```

List bundled incident knowledge without connecting to DBX:

```powershell
dbx-support cases
```

Run synthetic scenarios without connecting to a DBX node:

```powershell
dbx-support lab
```

Replay the support and recovery curriculum with reproducible variations:

```powershell
python -m dbx_support train --seed 17 --variants 20
```

The campaign evaluates 51 labelled diagnostic scenarios and 16 recovery-policy
scenarios per variant. Twenty variants run 1,340 drills. It varies tenant IDs,
memory scale, and unrelated counters, and checks exact diagnoses, safe
abstention, verification, and prevention of repeat mutations. A failed drill
or unsafe action returns a nonzero exit code. It uses synthetic clients and
temporary audit files, with no credentials or network connection. This is a
deterministic evaluation curriculum, not neural model training; it never
promotes observations into new recovery permissions.

The 31-entry casebook includes reviewed regression references for fatal WAL
recovery, tenant task panic, backup identity, checkpoint selection, vector
integrity and migration, replication authentication/tampering, encrypted key
mismatch, production isolation, protocol parsing, and configuration validation.
Tests verify that every reference names a real test. Catalogue-only cases do
not imply that snapshot telemetry can detect those failures. Only WAL recovery
and tenant task panic currently have stable codes recognized by the local log
scanner. See [the curriculum and coverage contract](../docs/support-recovery-curriculum.md).

Scan an explicitly selected local JSON-lines log. The current DBX build emits
`DBX_WAL_RECOVERY_ERROR` and `DBX_TENANT_TASK_PANIC`; the scanner returns reviewed
incident guidance and regression references, without printing or retaining raw lines:

```powershell
dbx-support logs --file .\dbx.jsonl
```

## Start a one-time diagnostic session

```powershell
dbx-support doctor --url http://127.0.0.1:8000
```

For remote DBX, use its TLS URL:

```powershell
dbx-support doctor --url https://dbx.example.com
```

The tool prompts for the support read capability without echoing it, then fetches
only the configured tenant summaries. It does not retain the capability after
exit. This capability cannot call DBX's operator APIs.

## Start a temporary watch session

```powershell
dbx-support watch --url https://dbx.example.com --interval 30
```

Watch mode polls only while the command is running. It compares successive error
counters, reports health findings, and updates a small local candidate store for
unclassified observations. Press `Ctrl+C` to stop. There is no daemon, scheduled
job, autostart entry, persistent network connection, or cloud support call.

### Enable a narrowly scoped automatic recovery

If you explicitly opt in, the extension prompts for the separate wake
capability and calls only DBX's `POST /api/support/v1/tenants/{id}/wake` route.
The server enforces its own tenant allowlist, records an audit event before
acting, and rate-limits each tenant. The extension also attempts at most once
per tenant per process and verifies that it returns running and healthy.
Hibernation may be intentional, so only allowlist tenants that may be woken.

```powershell
dbx-support watch --url https://dbx.example.com --interval 30 `
  --auto-wake-tenant tenant-a --auto-wake-tenant tenant-b
```

Both capabilities are static secrets, so protect them in a secret manager and
rotate them after exposure. The read token accesses only the server-allowlisted
snapshot. The wake token can invoke only the wake route and only for the
server-configured tenant allowlist; it cannot provision tenants, restore
backups, alter keys, or change DBX configuration.

### Create an optional local support bundle

This writes a redacted bundle on the support operator's machine. It is not
uploaded. Review it before sharing through your own support channel.

```powershell
dbx-support collect --url https://dbx.example.com --out .\incident.json
```

## Safety and limitations

- Diagnostics use only `GET /api/support/v1/snapshot`; recovery uses only the
  separate capability on `POST /api/support/v1/tenants/{id}/wake`.
- It never asks for a tenant key, DBX KEK, or data-plane credential.
- Terminal reports contain tenant IDs, status, usage counters, and findings.
  Local bundles remove tenant IDs and omit values, documents, embeddings,
  passwords, and JWTs.
- The client projects DBX responses onto an explicit telemetry allowlist. The
  shared guard validates collection structure, unique safe identities, scalar
  types, bounded counters, and port ranges before specialists or recovery see
  them. Conflicting lifecycle state holds recovery. Redirects are refused and
  declared response lengths are checked before accepting telemetry.
- The learner persists only incident code, severity, timestamps, occurrence
  count, and candidate status. Its file is under the local user's application
  state directory; on POSIX it is mode-restricted. It does not learn a debugging
  procedure or promote a case into executable automation.
- It has no cloud endpoint and sends no telemetry to DBX or a third party.
- `logs` analyzes only a local file chosen by the operator and counts known
  structured error codes. It does not upload, print raw lines, or preserve
  arbitrary log fields.
- `lab` checks diagnostic rules against synthetic snapshots only; it is not a
  customer-data reproduction environment and does not run DBX tests.
- Automatic wake is opt-in per tenant and process, checked against both client
  and server allowlists, rate limited server-side, and status checked before
  and after. DBX fails closed if it cannot record the pre-action audit event.
- A non-zero historical error counter is not treated as a new incident in
  one-shot mode; watch mode reports only a counter increase.
- Current DBX APIs do not expose enough information to diagnose WAL corruption,
  command-level latency, replica lag, or an incorrect vector result. The tool
  reports its limited scope rather than guessing.
- Specialists are deterministic Python components, not separately trained
  language models. The team is an advisory preview, not a compliance product.
  Except for the explicitly enabled wake action, it does not attempt recovery.
  It cannot repair data corruption or apply DBX code fixes.

## Planned next increments

1. Expand the reviewed catalogue from DBX source history, CI failures, and
   confirmed incidents; sign it and measure near-match false positives.
2. Add a support-specific short-lived capability and audited DBX recovery API
   before adding worker restarts or any wider remediation.
3. Add repair actions only after case-specific preconditions, post-checks,
   rollback, fault-injection, and strict-isolation gates exist.

The extension remains separately installed and manually started. DBX exposes
only the scoped support endpoints needed by it; the DBX data plane remains
independent of the support process.
