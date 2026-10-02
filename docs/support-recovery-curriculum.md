# Support and recovery curriculum

The support extension reduces routine diagnostic work and can perform one
explicitly enabled tenant wake per session. DBX itself already supervises failed
tenant engines with a bounded restart policy. The extension does not add another
restart loop. Unknown software defects and data integrity failures require a
verified repair; no finite test suite establishes automatic repair of every bug.

## Run the gates

From `support-extension`:

```powershell
python -m unittest discover -s tests -v
python -m dbx_support lab
python -m dbx_support train --seed 17 --variants 20
```

From the repository root:

```powershell
go test ./...
go test -count=1 -v ./internal/orchestrator ./internal/server -run 'Support|RecoverTenantTask'
```

The campaign has 51 labelled diagnostic and 16 recovery-policy scenarios per
variant, including near matches where automation must abstain. Seeds produce
repeatable identities and counter variations. The report records passed/total
drills, unsafe actions, and failed expectations; failure returns a nonzero exit
status. It uses synthetic data and temporary local audits. It changes no model
weights, capabilities, deployed configuration, or casebook permissions.

## Coverage and autonomous decisions

| Failure family | Evidence exercised | Support response |
|---|---|---|
| Down, starting, running unhealthy, or unknown lifecycle | Labelled snapshots and real lifecycle regressions | Diagnose; no wake of running/down tenants |
| Deliberate hibernation | Real support API to Python recovery integration | One audited wake for an allowlisted tenant, followed by running/healthy verification |
| Quota pressure/exhaustion | Warning boundary, exhausted boundary, unlimited quota, real quota-before-WAL regressions | Warn or mark critical; retain quota enforcement |
| Old/new/reset error counters | First observation, new tenant, interval increase, reset, connection gap | Establish tenant-specific baselines; avoid claiming historical errors are new |
| Malformed, duplicate, or conflicting telemetry | Collection types, nested payloads, bool/string counters, overflow, duplicate IDs, missing usage | Reject malformed telemetry; hold recovery until consistent |
| Credentials and transport faults | Dedicated HTTP endpoints, distinct tokens, redirects, 401/403, transient HTTP errors, TLS failure, oversized/truncated/invalid JSON | Stop on auth/TLS/schema failure; retry transient reads with capped exponential backoff; never retry an ambiguous wake |
| Recovery outcome uncertainty | Lost response, unhealthy result, missing tenant, malformed result, unavailable verification | Verify independently; record failed/unverified outcome and suppress repeat mutations |
| Local audit or learning-store failure | Pre-action audit denial, final audit failure, malformed store, atomic replacement failure, bounded candidates | Skip unaudited actions; preserve state; continue diagnostics when optional learning storage fails |
| Specialist crash | Injected specialist exception and watch-loop gate | Keep completed diagnostics; hold recovery for the incomplete cycle |
| WAL corruption or tenant task panic | Stable local log codes and Go corruption/panic regressions | Emit reviewed guidance and references; preserve evidence; respect existing bounded supervision |
| Backup/checkpoint/vector/replication/encryption/isolation faults | Casebook references to real DBX regression tests, plus the full Go suite | Catalogue guidance only; snapshot telemetry cannot establish these diagnoses |

`logs --file PATH` reads only the explicitly selected local file. It recognizes
the stable WAL and task-panic codes and returns aggregate counts, severity, case
IDs, test references, and reviewed recovery guidance. Arbitrary log instructions
and unknown codes never authorize an action.

The candidate learner stores only validated codes, severity, timestamps, counts,
and quarantine status, bounded to 500 entries and a 1 MiB read limit. Invalid
state is preserved. Repeated observations do not authorize a repair.

## Release evidence and limits

`.github/workflows/support-recovery.yml` runs the Python tests, 20-variant
campaign, and real support API integration on Linux and Windows with Python
3.10 and 3.14. It enables the Go race detector on Linux and preserves campaign
reports as CI artifacts. The existing production release gates remain responsible
for strict Linux isolation and the vector benchmark. A configured workflow is
not evidence that remote CI has run.

Local Windows results do not validate Linux namespace, Landlock, or seccomp
behavior. Fatal persistence/backup/replication cases have real regression
references, but synthetic telemetry drills do not inject those defects into
customer databases. Those cases remain diagnose/preserve-evidence guidance.
Code repair, restore, promotion, deletion, quota changes, and security changes
remain outside the support capability.

Add a new executable recovery only after its deployed signal, independent
reproducer, preconditions, action limit, durable audit, post-check, and failed or
ambiguous outcomes are tested. A catalogue entry or repeated observation alone
is insufficient evidence to enable a production mutation.

## Local verification on 2026-10-02

Windows verification passed 43 support tests, all 1,340 synthetic drills with
seed 17 and 20 variants, the full `go test ./...` suite, and `go vet ./...`.
The explicit Go/Python integration also passed scoped reads, an audited wake
of a real synthetic engine, post-check verification, and duplicate suppression.
The [saved campaign report](support-training-report.json) contains its counts
and failure list. Linux race/isolation checks and remote CI were not run locally.
