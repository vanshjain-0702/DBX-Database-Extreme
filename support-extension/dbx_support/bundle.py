"""Build a user-selected local incident bundle with tenant IDs removed."""

from __future__ import annotations

import json
import os
from pathlib import Path
import tempfile

from .team import TeamReport
from .casebook import case_for


def write_redacted_bundle(path: Path, snapshot: dict, report: TeamReport) -> None:
    tenant_ids = {
        str(row.get("id", "")) for row in snapshot.get("tenants", [])
        if isinstance(row, dict) and row.get("id")
    } | {
        str(row.get("tenant_id", "")) for row in snapshot.get("usage", [])
        if isinstance(row, dict) and row.get("tenant_id")
    }
    findings = []
    for finding in report.findings:
        findings.append({
            "severity": finding.severity,
            "code": finding.code,
            "subject": "tenant" if finding.subject in tenant_ids else "control-plane",
            # Persist reviewed prose only. Dynamic finding text can contain
            # arbitrary API strings even when identifiers are separately hidden.
            "message": case_for(finding.code).diagnosis if case_for(finding.code) else "Unclassified support symptom; reproduction required.",
            "agent": finding.agent,
            "case_id": finding.case_id,
            "test_refs": list(finding.test_refs),
            "automated_recovery": finding.automated_recovery,
        })
    statuses = {}
    for tenant in snapshot.get("tenants", []):
        status = str(tenant.get("status", "unknown"))
        if status not in {"running", "starting", "down", "hibernated"}:
            status = "unknown"
        statuses[status] = statuses.get(status, 0) + 1
    bundle = {
        "format": "dbx-support-incident-v1",
        "created_at": report.created_at,
        "casebook_version": report.casebook_version,
        "tenant_count": len(snapshot.get("tenants", [])),
        "tenant_status_counts": statuses,
        "usage_summary": {
            "memory_used_bytes": _sum(snapshot, "memory_used_bytes"),
            "memory_limit_bytes": _sum(snapshot, "memory_limit_bytes"),
            "keys": _sum(snapshot, "keys"),
            "vectors": _sum(snapshot, "vectors"),
            "errors": _sum(snapshot, "errors"),
        },
        "findings": findings,
        "tenant_identifiers_included": False,
        "raw_tenant_data_included": False,
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp_name = tempfile.mkstemp(prefix=".dbx-support-bundle-", dir=path.parent)
    temp_path = Path(temp_name)
    try:
        if os.name != "nt":
            os.chmod(temp_path, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(bundle, handle, indent=2, sort_keys=True)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temp_path, path)
        if os.name != "nt":
            os.chmod(path, 0o600)
    finally:
        temp_path.unlink(missing_ok=True)


def _sum(snapshot: dict, field: str) -> int:
    total = 0
    for row in snapshot.get("usage", []):
        try:
            total += max(0, int(row.get(field, 0) or 0))
        except (TypeError, ValueError, OverflowError):
            continue
    return total
