"""Opt-in local structured-log scanner that emits aggregate codes only."""

from __future__ import annotations

import json
from collections import Counter
from pathlib import Path

from .casebook import case_for


KNOWN_CODES = {
    "DBX_WAL_RECOVERY_ERROR",
    "DBX_TENANT_TASK_PANIC",
}


def scan_structured_log(path: Path) -> dict:
    """Count allowlisted error codes; never return or persist raw log fields."""
    if path.stat().st_size > 256 * 1024 * 1024:
        raise ValueError("log scan limit exceeded (256 MiB)")
    counts: Counter[str] = Counter()
    malformed = 0
    lines = 0
    consumed = 0
    with path.open("rb") as handle:
        while True:
            raw = handle.readline(1024 * 1024 + 1)
            if not raw:
                break
            consumed += len(raw)
            if consumed > 256 * 1024 * 1024:
                raise ValueError("log scan limit exceeded (256 MiB)")
            lines += 1
            if lines > 1_000_000:
                raise ValueError("log scan limit exceeded (1,000,000 lines)")
            if len(raw) > 1024 * 1024:
                malformed += 1
                while raw and not raw.endswith(b"\n"):
                    raw = handle.readline(1024 * 1024 + 1)
                    consumed += len(raw)
                    if consumed > 256 * 1024 * 1024:
                        raise ValueError("log scan limit exceeded (256 MiB)")
                continue
            try:
                record = json.loads(raw)
            except (ValueError, RecursionError):
                malformed += 1
                continue
            if not isinstance(record, dict):
                malformed += 1
                continue
            code = record.get("code") or record.get("error_code")
            if isinstance(code, str) and code in KNOWN_CODES:
                counts[code] += 1
    return {
        "lines_scanned": lines,
        "malformed_lines": malformed,
        "known_error_counts": dict(sorted(counts.items())),
        "incidents": [{"code": code, "occurrences": count, "severity": "critical",
                       "case_id": case_for(code).case_id, "test_refs": list(case_for(code).tests),
                       "recovery_guidance": case_for(code).recovery, "automatic_action": "none"}
                      for code, count in sorted(counts.items())],
        "raw_log_content_retained": False,
    }
