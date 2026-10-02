"""Quarantined, local-only learner for unclassified failure observations."""

from __future__ import annotations

import json
import os
from pathlib import Path
import re
import tempfile
from typing import Iterable

from .casebook import case_for
from .diagnostics import Finding


class NovelIssueLearner:
    """Records signatures for later reproduction; it never invents runbooks."""

    name = "novel-issue-learner"
    specialty = "aggregates unseen symptom signatures in a local quarantine store"

    def __init__(self, path: Path | None = None) -> None:
        self.path = Path(path) if path is not None else default_learning_path()

    def observe(self, findings: Iterable[Finding]) -> int:
        candidates = [
            finding for finding in findings
            if case_for(finding.code) is None or finding.code == "DBX_ERRORS_INCREASED"
        ]
        if not candidates:
            return 0
        if any(not _safe_signature(finding.code, finding.severity) for finding in candidates):
            raise RuntimeError("invalid support learning signature; no observations were written")

        state = self._read()
        observations = state.setdefault("observations", {})
        now = _utc_now()
        for finding in candidates:
            # Do not persist tenant IDs, error messages, database values, vectors,
            # endpoint URLs, credentials, or model prompts.
            key = f"{finding.code}:{finding.severity}"
            current = observations.get(key)
            if not isinstance(current, dict):
                current = {
                    "code": finding.code,
                    "severity": finding.severity,
                    "first_seen": now,
                    "occurrences": 0,
                    "status": "candidate-needs-reproduction",
                }
                observations[key] = current
            current["last_seen"] = now
            current["occurrences"] = min(current.get("occurrences", 0) + 1, 2**63 - 1)

        # Bound the local store so a noisy event stream cannot fill the disk.
        if len(observations) > 500:
            keep = sorted(
                observations.items(),
                key=lambda item: str(item[1].get("last_seen", "")),
                reverse=True,
            )[:500]
            state["observations"] = dict(keep)
        self._write(state)
        return len(candidates)

    def _read(self) -> dict:
        if not self.path.exists():
            return {"schema_version": 1, "observations": {}}
        try:
            with self.path.open("rb") as handle:
                raw = handle.read(1024 * 1024 + 1)
            if len(raw) > 1024 * 1024:
                raise ValueError("learning store too large")
            state = json.loads(raw.decode("utf-8"))
        except (OSError, ValueError, RecursionError):
            # Preserve a damaged learning file; never silently overwrite it.
            raise RuntimeError(f"cannot read support learning store: {self.path}") from None
        if (not isinstance(state, dict) or type(state.get("schema_version")) is not int or state["schema_version"] != 1
                or set(state) != {"schema_version", "observations"}
                or not isinstance(state.get("observations"), dict)
                or len(state["observations"]) > 500):
            raise RuntimeError(f"invalid support learning store: {self.path}")
        for key, entry in state["observations"].items():
            if (not isinstance(entry, dict)
                    or set(entry) != {"code", "severity", "first_seen", "last_seen", "occurrences", "status"}
                    or not _safe_signature(entry.get("code"), entry.get("severity"))
                    or key != f"{entry['code']}:{entry['severity']}"
                    or entry["status"] != "candidate-needs-reproduction"
                    or type(entry["occurrences"]) is not int
                    or not 1 <= entry["occurrences"] <= 2**63 - 1
                    or not _timestamp(entry["first_seen"]) or not _timestamp(entry["last_seen"])):
                raise RuntimeError(f"invalid support learning store: {self.path}")
        return state

    def _write(self, state: dict) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        fd, temp_name = tempfile.mkstemp(prefix=".dbx-support-", dir=self.path.parent)
        temp_path = Path(temp_name)
        try:
            if os.name != "nt":
                os.chmod(temp_path, 0o600)
            with os.fdopen(fd, "w", encoding="utf-8") as handle:
                json.dump(state, handle, indent=2, sort_keys=True)
                handle.write("\n")
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temp_path, self.path)
            if os.name != "nt":
                os.chmod(self.path, 0o600)
        finally:
            try:
                temp_path.unlink(missing_ok=True)
            except OSError:
                pass


def default_learning_path() -> Path:
    if os.name == "nt":
        root = Path(os.environ.get("LOCALAPPDATA", Path.home() / "AppData" / "Local"))
        return root / "DBX" / "Support" / "observations.json"
    root = Path(os.environ.get("XDG_STATE_HOME", Path.home() / ".local" / "state"))
    return root / "dbx-support" / "observations.json"


def _utc_now() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _safe_signature(code: object, severity: object) -> bool:
    return (isinstance(code, str) and len(code) <= 96
            and re.fullmatch(r"DBX_[A-Z0-9_]+", code) is not None
            and isinstance(severity, str) and severity in {"info", "warning", "critical"})


def _timestamp(value: object) -> bool:
    if not isinstance(value, str) or len(value) > 40:
        return False
    from datetime import datetime

    try:
        return datetime.fromisoformat(value).tzinfo is not None
    except ValueError:
        return False
