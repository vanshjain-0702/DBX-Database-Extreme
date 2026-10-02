"""Policy-gated, one-shot recovery through a single existing DBX API action."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
import json
import os
from pathlib import Path

from .client import DBXSupportClient, SupportConnectionError
from .diagnostics import consistency_findings, scope_findings
from .learning import default_learning_path
from .schema import safe_tenant_id


@dataclass(frozen=True)
class RepairEvent:
    tenant_id: str
    action: str
    outcome: str
    detail: str


class RecoveryManager:
    """Wake only explicitly allowlisted hibernated tenants, once per session.

    Hibernation is usually intentional. There is no automatic action unless the
    operator starts watch with an explicit tenant allowlist. This manager does
    not restart workers, alter settings, or touch tenant data.
    """

    def __init__(self, client: DBXSupportClient, allowed_tenants: set[str] | None = None,
                 audit_path: Path | None = None) -> None:
        self.client = client
        self.allowed_tenants = set(allowed_tenants or ())
        if any(not safe_tenant_id(tenant_id) for tenant_id in self.allowed_tenants):
            raise ValueError("recovery allowlist contains an invalid tenant ID")
        self._attempted: set[str] = set()
        self.audit_path = Path(audit_path) if audit_path is not None else default_learning_path().with_name("repair-audit.jsonl")

    def repair(self, snapshot: dict) -> list[RepairEvent]:
        events: list[RepairEvent] = []
        if not self.allowed_tenants:
            return events
        # Apply the guard here too: direct callers cannot bypass team checks.
        if scope_findings(snapshot) or consistency_findings(snapshot):
            return [RepairEvent(tenant_id, "wake", "skipped",
                                "snapshot validation or lifecycle consistency failed; no action was taken")
                    for tenant_id in sorted(self.allowed_tenants - self._attempted)]
        tenants = {
            str(row.get("id", "")): row
            for row in snapshot.get("tenants", [])
            if isinstance(row, dict)
        }
        for tenant_id in sorted(self.allowed_tenants):
            tenant = tenants.get(tenant_id)
            if tenant is None:
                events.append(RepairEvent(tenant_id, "wake", "skipped", "tenant is absent from the current snapshot"))
                continue
            if tenant_id in self._attempted:
                continue
            if str(tenant.get("status", "")).lower() != "hibernated":
                continue

            self._attempted.add(tenant_id)
            try:
                self._audit(tenant_id, "wake", "attempting")
            except OSError:
                events.append(RepairEvent(tenant_id, "wake", "skipped", "local audit is unavailable; no action was taken"))
                continue
            request_error = ""
            try:
                self.client.wake_tenant(tenant_id)
            except SupportConnectionError:
                # A timeout or lost response can happen after DBX accepted the
                # operation. Check state before classifying the result; never retry.
                request_error = "wake response was not confirmed"
            try:
                verified = self.client.snapshot()
            except SupportConnectionError:
                self._record_outcome(tenant_id, "unverified")
                events.append(RepairEvent(tenant_id, "wake", "unverified", "could not verify DBX state; no retry this session"))
                continue
            if scope_findings(verified) or consistency_findings(verified):
                self._record_outcome(tenant_id, "unverified")
                events.append(RepairEvent(tenant_id, "wake", "unverified",
                                          "post-check telemetry is invalid or inconsistent; no retry this session"))
                continue
            current = next(
                (row for row in verified["tenants"] if str(row.get("id", "")) == tenant_id),
                None,
            )
            if (current and str(current.get("status", "")).lower() == "running"
                    and current.get("healthy") is True):
                audited = self._record_outcome(tenant_id, "recovered")
                detail = "tenant reports running and healthy after the action"
                if request_error:
                    detail += "; DBX wake response was lost, but the post-check succeeded"
                if not audited:
                    detail += "; final audit record could not be written"
                events.append(RepairEvent(tenant_id, "wake", "recovered", detail))
            else:
                outcome = "failed" if request_error else "unverified"
                self._record_outcome(tenant_id, outcome)
                detail = "tenant did not report running and healthy; automation stopped"
                if request_error:
                    detail = "wake response was lost and tenant is not healthy; no retry this session"
                events.append(RepairEvent(tenant_id, "wake", outcome, detail))
        return events

    def _audit(self, tenant_id: str, action: str, outcome: str) -> None:
        self.audit_path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        fd = os.open(self.audit_path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        try:
            if os.name != "nt":
                os.chmod(self.audit_path, 0o600)
            record = {
                "time": datetime.now(timezone.utc).isoformat(timespec="seconds"),
                "tenant_id": tenant_id,
                "action": action,
                "outcome": outcome,
            }
            with os.fdopen(fd, "a", encoding="utf-8") as handle:
                handle.write(json.dumps(record, sort_keys=True) + "\n")
                handle.flush()
                os.fsync(handle.fileno())
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            raise

    def _record_outcome(self, tenant_id: str, outcome: str) -> bool:
        try:
            self._audit(tenant_id, "wake", outcome)
            return True
        except OSError:
            return False
