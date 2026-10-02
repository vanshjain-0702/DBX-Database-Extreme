"""Reviewed telemetry boundary shared by diagnostics, transport, and recovery."""

from __future__ import annotations

import re


TENANT_FIELDS = {
    "id", "name", "http_port", "resp_port", "status", "healthy", "engine",
    "role", "replica_of", "replication_port", "replicas", "hibernated",
}
USAGE_FIELDS = {
    "tenant_id", "status", "hibernated", "keys", "vectors", "memory_used_bytes",
    "memory_limit_bytes", "disk_bytes", "commands", "errors", "avg_latency_ns",
}
COUNTERS = USAGE_FIELDS - {"tenant_id", "status", "hibernated"}
PORTS = {"http_port", "resp_port", "replication_port"}


def safe_tenant_id(value: object) -> bool:
    return (isinstance(value, str) and value not in ("", ".", "..")
            and re.fullmatch(r"[A-Za-z0-9._-]+", value) is not None)


def snapshot_problem(snapshot: object) -> str:
    """Return a fixed code, never a payload, for malformed or out-of-scope input."""
    if not isinstance(snapshot, dict):
        return "DBX_SUPPORT_SNAPSHOT_INVALID"
    if set(snapshot) - {"tenants", "usage"}:
        return "DBX_SUPPORT_SCOPE_BREACH"
    for collection, fields, identity in (
        ("tenants", TENANT_FIELDS, "id"),
        ("usage", USAGE_FIELDS, "tenant_id"),
    ):
        rows = snapshot.get(collection)
        if not isinstance(rows, list):
            return "DBX_SUPPORT_SNAPSHOT_INVALID"
        seen: set[str] = set()
        for row in rows:
            if not isinstance(row, dict):
                return "DBX_SUPPORT_SNAPSHOT_INVALID"
            if set(row) - fields:
                return "DBX_SUPPORT_SCOPE_BREACH"
            tenant_id = row.get(identity)
            if not safe_tenant_id(tenant_id) or tenant_id in seen:
                return "DBX_SUPPORT_SNAPSHOT_INVALID"
            seen.add(tenant_id)
            if collection == "tenants" and "status" not in row:
                return "DBX_SUPPORT_SNAPSHOT_INVALID"
            for key, value in row.items():
                if key in COUNTERS or key in PORTS:
                    maximum = 65535 if key in PORTS else 2**63 - 1
                    if type(value) is not int or not 0 <= value <= maximum:
                        return "DBX_SUPPORT_SNAPSHOT_INVALID"
                elif key in {"healthy", "hibernated"}:
                    if type(value) is not bool:
                        return "DBX_SUPPORT_SNAPSHOT_INVALID"
                elif key == "replicas":
                    if (not isinstance(value, list)
                            or any(not safe_tenant_id(item) for item in value)):
                        return "DBX_SUPPORT_SNAPSHOT_INVALID"
                elif (not isinstance(value, str) or len(value) > 1024
                      or any(ord(char) < 32 or ord(char) == 127 for char in value)):
                    return "DBX_SUPPORT_SNAPSHOT_INVALID"
    return ""
