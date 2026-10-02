"""Deterministic, domain-scoped diagnostic rules used by specialist agents."""

from __future__ import annotations

from dataclasses import dataclass, field

from .schema import snapshot_problem


@dataclass(frozen=True)
class Finding:
    severity: str
    code: str
    subject: str
    message: str
    next_step: str
    agent: str = "unassigned"
    case_id: str = ""
    test_refs: tuple[str, ...] = field(default_factory=tuple)
    automated_recovery: str = "not_available"


def lifecycle_findings(snapshot: dict) -> list[Finding]:
    findings = []
    for tenant in snapshot["tenants"]:
        tenant_id = str(tenant.get("id", "unknown"))
        status = str(tenant.get("status", "unknown")).lower()
        if status == "down":
            findings.append(Finding(
                "critical", "DBX_TENANT_DOWN", tenant_id,
                "Tenant is down according to the DBX control plane.",
                "Check DBX orchestrator logs and tenant worker health; no restart was attempted.",
                agent="tenant-lifecycle",
            ))
        elif status == "starting":
            findings.append(Finding(
                "warning", "DBX_TENANT_STARTING", tenant_id,
                "Tenant is still starting.",
                "Wait for another check; if this persists, collect DBX service logs.",
                agent="tenant-lifecycle",
            ))
        elif status == "hibernated":
            findings.append(Finding(
                "info", "DBX_TENANT_HIBERNATED", tenant_id,
                "Tenant is hibernated; this may be intentional.",
                "Wake it only if required; the separate support recovery path needs an explicit tenant allowlist and wake capability.",
                agent="tenant-lifecycle",
            ))
        elif status == "running" and tenant.get("healthy") is False:
            findings.append(Finding(
                "critical", "DBX_TENANT_UNHEALTHY", tenant_id,
                "Tenant reports running but its health check failed.",
                "Preserve worker logs and check service health; waking a running tenant is inappropriate.",
                agent="tenant-lifecycle",
            ))
        elif status != "running":
            findings.append(Finding(
                "warning", "DBX_TENANT_STATUS_UNKNOWN", tenant_id,
                "DBX reported an unfamiliar tenant status.",
                "Check this DBX version's lifecycle documentation.",
                agent="tenant-lifecycle",
            ))
    return findings


def capacity_findings(snapshot: dict, memory_warn_percent: float = 80.0) -> list[Finding]:
    findings = []
    for row in snapshot["usage"]:
        tenant_id = str(row.get("tenant_id", "unknown"))
        used = _integer(row.get("memory_used_bytes"))
        limit = _integer(row.get("memory_limit_bytes"))
        if limit > 0 and used * 100.0 / limit >= memory_warn_percent:
            percent = used * 100.0 / limit
            findings.append(Finding(
                "critical" if used >= limit else "warning",
                "DBX_TENANT_MEMORY_EXHAUSTED" if used >= limit else "DBX_TENANT_MEMORY_HIGH", tenant_id,
                f"Tenant memory usage is {percent:.1f}% of its configured limit.",
                "Review tenant workload and configured quota; this agent does not change limits.",
                agent="capacity",
            ))
    return findings


def error_findings(snapshot: dict, previous: dict | None = None) -> list[Finding]:
    findings = []
    previous_usage = {
        str(row.get("tenant_id", "")): row for row in (previous or {}).get("usage", [])
    }
    for row in snapshot["usage"]:
        tenant_id = str(row.get("tenant_id", "unknown"))
        errors = _integer(row.get("errors"))
        baseline = previous_usage.get(tenant_id)
        old_errors = _integer((baseline or {}).get("errors"))
        if baseline is not None and errors > old_errors:
            findings.append(Finding(
                "warning", "DBX_ERRORS_INCREASED", tenant_id,
                f"DBX error counter increased by {errors - old_errors} since the last check.",
                "Inspect DBX logs for this interval. The current API does not expose error details.",
                agent="error-trend",
            ))
        elif baseline is not None and errors < old_errors:
            findings.append(Finding(
                "info", "DBX_ERROR_COUNTER_RESET", tenant_id,
                "DBX error counter decreased; its observation baseline was reset.",
                "Compare subsequent counters; this decrease does not establish that an incident was repaired.",
                agent="error-trend",
            ))
        elif baseline is None and errors > 0:
            findings.append(Finding(
                "info", "DBX_ERRORS_CUMULATIVE", tenant_id,
                f"DBX reports {errors} cumulative errors; this snapshot cannot tell when they occurred.",
                "Use watch mode to determine whether the counter is still increasing.",
                agent="error-trend",
            ))
    return findings


def consistency_findings(snapshot: dict) -> list[Finding]:
    tenants = {str(row.get("id", "")) for row in snapshot["tenants"]}
    usage = {str(row.get("tenant_id", "")) for row in snapshot["usage"]}
    findings = []
    for tenant_id in sorted(tenants - usage):
        findings.append(Finding(
            "warning", "DBX_USAGE_MISSING", tenant_id,
            "Tenant is listed, but its usage snapshot is missing.",
            "Repeat the check; if persistent, include the control-plane responses in a support report.",
            agent="control-plane-consistency",
        ))
    for tenant_id in sorted(usage - tenants):
        findings.append(Finding(
            "warning", "DBX_USAGE_TENANT_UNLISTED", tenant_id,
            "Usage returned a tenant that was absent from the tenant list.",
            "Repeat the check; persistent control-plane disagreement needs DBX support review.",
            agent="control-plane-consistency",
        ))
    usage_rows = {row["tenant_id"]: row for row in snapshot["usage"]}
    for tenant in snapshot["tenants"]:
        row = usage_rows.get(tenant["id"], {})
        status = tenant["status"].lower()
        if (("status" in row and row["status"].lower() != status)
                or ("hibernated" in tenant and tenant["hibernated"] != (status == "hibernated"))
                or ("hibernated" in row and row["hibernated"] != (status == "hibernated"))
                or (status == "hibernated" and tenant.get("healthy") is True)):
            findings.append(Finding(
                "warning", "DBX_LIFECYCLE_INCONSISTENT", tenant["id"],
                "Tenant lifecycle and usage signals disagree.",
                "Repeat the snapshot before recovery; do not act on conflicting lifecycle evidence.",
                agent="control-plane-consistency",
            ))
    return findings


def scope_findings(snapshot: dict) -> list[Finding]:
    """Validate structure and types before any specialist sees the payload."""
    problem = snapshot_problem(snapshot)
    if not problem:
        return []
    return [Finding(
        "critical", problem, "control-plane",
        "Support telemetry is malformed or outside the reviewed schema; diagnostics have stopped.",
        "Discard this snapshot and check the API schema boundary before continuing recovery.",
        agent="support-scope-guard",
    )]


def analyze(snapshot: dict, previous: dict | None = None, memory_warn_percent: float = 80.0) -> list[Finding]:
    """Compatibility helper used by callers that want the combined rule set."""
    boundary = scope_findings(snapshot) + (scope_findings(previous) if previous is not None else [])
    if boundary:
        return boundary
    return (
        lifecycle_findings(snapshot)
        + capacity_findings(snapshot, memory_warn_percent)
        + error_findings(snapshot, previous)
        + consistency_findings(snapshot)
    )


def _integer(value: object) -> int:
    try:
        return max(0, int(value))
    except (TypeError, ValueError, OverflowError):
        return 0
