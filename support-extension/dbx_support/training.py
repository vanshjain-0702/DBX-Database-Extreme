"""Repeatable diagnosis and recovery-policy drills using synthetic data only."""

from __future__ import annotations

from copy import deepcopy
from dataclasses import replace
from pathlib import Path
import random
import tempfile

from .casebook import CASEBOOK_VERSION, all_cases
from .client import SupportConnectionError
from .lab import SCENARIOS, evaluate_scenario, fixture
from .recovery import RecoveryManager
from .team import SupportTeam


class _DrillClient:
    def __init__(self, postcheck: object, *, lost_response: bool = False,
                 verification_outage: bool = False) -> None:
        self.postcheck = postcheck
        self.lost_response = lost_response
        self.verification_outage = verification_outage
        self.wakes: list[str] = []

    def wake_tenant(self, tenant_id: str) -> None:
        self.wakes.append(tenant_id)
        if self.lost_response:
            raise SupportConnectionError("synthetic lost response", retryable=True)

    def snapshot(self) -> object:
        if self.verification_outage:
            raise SupportConnectionError("synthetic verification outage", retryable=True)
        return self.postcheck


def _recovery_drills(directory: Path) -> list[dict]:
    asleep = fixture("hibernated", healthy=False)
    awake = fixture(healthy=True)
    conflict = deepcopy(asleep)
    conflict["usage"][0]["status"] = "running"
    duplicate = deepcopy(asleep)
    duplicate["tenants"].append(deepcopy(duplicate["tenants"][0]))
    missing_usage = deepcopy(asleep)
    missing_usage["usage"] = []
    unknown_tenant = {"tenants": [], "usage": []}
    tests = (
        ("wake succeeds", asleep, awake, True, False, False, False, "recovered", 1),
        ("lost response but healthy", asleep, awake, True, True, False, False, "recovered", 1),
        ("unhealthy postcheck", asleep, fixture(healthy=False), True, False, False, False, "unverified", 1),
        ("lost response and still asleep", asleep, asleep, True, True, False, False, "failed", 1),
        ("verification outage", asleep, awake, True, False, True, False, "unverified", 1),
        ("malformed postcheck", asleep, None, True, False, False, False, "unverified", 1),
        ("conflicting postcheck", asleep, conflict, True, False, False, False, "unverified", 1),
        ("tenant vanished after wake", asleep, unknown_tenant, True, False, False, False, "unverified", 1),
        ("audit unavailable", asleep, awake, True, False, False, True, "skipped", 0),
        ("running tenant abstention", awake, awake, True, False, False, False, "none", 0),
        ("down tenant abstention", fixture("down"), awake, True, False, False, False, "none", 0),
        ("no allowlist abstention", asleep, awake, False, False, False, False, "none", 0),
        ("unlisted tenant abstention", unknown_tenant, awake, True, False, False, False, "skipped", 0),
        ("duplicate identity abstention", duplicate, awake, True, False, False, False, "skipped", 0),
        ("conflicting precondition abstention", conflict, awake, True, False, False, False, "skipped", 0),
        ("missing usage abstention", missing_usage, awake, True, False, False, False, "skipped", 0),
    )
    results = []
    for index, (name, before, after, enabled, lost, outage, bad_audit, outcome, calls) in enumerate(tests):
        client = _DrillClient(after, lost_response=lost, verification_outage=outage)
        audit = directory / f"drill-{index}.jsonl"
        if bad_audit:
            audit.write_text("directory blocker", encoding="utf-8")
            audit = audit / "blocked.jsonl"
        manager = RecoveryManager(client, {"synthetic-a"} if enabled else set(), audit)
        events = manager.repair(deepcopy(before))
        # A second poll must never repeat a mutation, including ambiguous outcomes.
        manager.repair(deepcopy(before))
        observed = events[0].outcome if events else "none"
        unsafe = len(client.wakes) > calls or any(tenant != "synthetic-a" for tenant in client.wakes)
        results.append({"scenario": name, "expected_outcome": outcome, "observed_outcome": observed,
                        "expected_wake_calls": calls, "observed_wake_calls": len(client.wakes),
                        "unsafe_action": unsafe,
                        "pass": observed == outcome and len(client.wakes) == calls and not unsafe})
    return results


def run_training_campaign(*, seed: int = 17, variants: int = 5) -> dict:
    """Replay labels and perturb unrelated counters and identities.

    Evaluates deterministic rules, not model weights. It does not change
    capabilities, promote novel observations, or enable catalogue-only repairs.
    """
    if not 1 <= variants <= 100:
        raise ValueError("training variants must be between 1 and 100")
    rng = random.Random(seed)
    team = SupportTeam()
    diagnostic_results = []
    recovery_results = []
    with tempfile.TemporaryDirectory(prefix="dbx-support-training-") as root:
        for variant in range(variants):
            tenant_id = f"synthetic-{rng.randrange(10**9)}"
            scale = rng.randrange(1, 10000)
            for original in SCENARIOS:
                snapshot = deepcopy(original.snapshot)
                previous = deepcopy(original.previous)
                for candidate in (snapshot, previous):
                    if not isinstance(candidate, dict):
                        continue
                    for collection, identity in (("tenants", "id"), ("usage", "tenant_id")):
                        rows = candidate.get(collection)
                        if not isinstance(rows, list):
                            continue
                        for row in rows:
                            if not isinstance(row, dict):
                                continue
                            if row.get(identity) == "synthetic-a":
                                row[identity] = tenant_id
                            if collection == "usage":
                                for counter in ("memory_used_bytes", "memory_limit_bytes"):
                                    if type(row.get(counter)) is int:
                                        row[counter] *= scale
                                row["keys"] = rng.randrange(10000)
                                row["vectors"] = rng.randrange(10000)
                scenario = replace(original, name=f"{original.name} / variant {variant + 1}",
                                   snapshot=snapshot, previous=previous)
                diagnostic_results.append(evaluate_scenario(team, scenario))
            directory = Path(root) / str(variant)
            directory.mkdir()
            recovery_results.extend(_recovery_drills(directory))
    all_results = diagnostic_results + recovery_results
    failures = [row for row in all_results if not row["pass"]]
    return {
        "format": "dbx-support-training-v1", "casebook_version": CASEBOOK_VERSION,
        "seed": seed, "variants": variants, "casebook_entries": len(all_cases()),
        "diagnostic_drills": len(diagnostic_results), "recovery_drills": len(recovery_results),
        "passed": len(all_results) - len(failures), "total": len(all_results),
        "unsafe_actions": sum(row["unsafe_action"] for row in recovery_results),
        "failures": failures, "production_actions_taken": False,
        "scope": "Synthetic diagnosis and wake policy only; not exhaustive defect coverage or model training.",
    }
