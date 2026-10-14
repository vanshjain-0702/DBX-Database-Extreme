"""Firm-defined review standards and evidence-bound reviewer decisions.

These are first-pass commercial review checks, not jurisdictional legal advice.
"""
from __future__ import annotations

import hashlib
import json
from typing import Any, Dict, List

CHECK_KEYS = ["indemnity", "renewal", "law", "breach", "confidentiality", "termination", "assignment", "data_return"]
DEFAULT_POLICY = {
    "name": "Commercial MSA",
    "liability_floor_months": 12,
    "renewal_notice_days": 30,
    "breach_notice_hours": 72,
    "accepted_laws": ["Delaware"],
    "enabled_checks": CHECK_KEYS,
    "custom_rules": [],
}
PRESETS = [
    {"id": "commercial", "name": "Commercial MSA", "policy": DEFAULT_POLICY},
    {"id": "privacy", "name": "Data processing review", "policy": {
        **DEFAULT_POLICY, "name": "Data processing review", "breach_notice_hours": 48,
        "enabled_checks": ["law", "breach", "confidentiality", "termination", "data_return"],
    }},
    {"id": "focused", "name": "Core commercial checks", "policy": {
        **DEFAULT_POLICY, "name": "Core commercial checks", "enabled_checks": CHECK_KEYS[:4],
    }},
]
REVIEW_STATES = {"open", "in_review", "resolved", "accepted_risk"}
DECIDED_STATES = {"resolved", "accepted_risk"}


def normalize_policy(raw: Dict[str, Any] | None = None) -> Dict[str, Any]:
    policy = {**DEFAULT_POLICY, **(raw or {})}
    name = policy.get("name")
    if not isinstance(name, str) or not name.strip() or len(name) > 100:
        raise ValueError("playbook name must contain 1–100 characters")
    out = {"name": name.strip()}
    for field, ceiling in [("liability_floor_months", 120), ("renewal_notice_days", 365), ("breach_notice_hours", 720)]:
        value = policy.get(field)
        if isinstance(value, bool) or not isinstance(value, int) or not 1 <= value <= ceiling:
            raise ValueError(f"{field} must be an integer from 1 to {ceiling}")
        out[field] = value
    laws = policy.get("accepted_laws")
    if not isinstance(laws, list) or not 1 <= len(laws) <= 20 or any(
        not isinstance(law, str) or not law.strip() or len(law) > 80 for law in laws
    ):
        raise ValueError("accepted_laws must contain 1–20 jurisdiction names")
    out["accepted_laws"] = list(dict.fromkeys(law.strip() for law in laws))
    enabled = policy.get("enabled_checks")
    if not isinstance(enabled, list) or any(not isinstance(key, str) or key not in CHECK_KEYS for key in enabled):
        raise ValueError("select supported review checks")
    out["enabled_checks"] = [key for key in CHECK_KEYS if key in enabled]
    from custom_rules import normalize_rules
    out["custom_rules"] = normalize_rules(policy.get("custom_rules", []))
    if not out["enabled_checks"] and not out["custom_rules"]:
        raise ValueError("select at least one supported review check or custom rule")
    return out


def evidence_fingerprint(item: Dict[str, Any], policy: Dict[str, Any]) -> str:
    fields = {key: item.get(key) for key in ("id", "state", "finding", "quote", "chunk_id", "anchor", "proposed_text", "authority", "provenance")}
    fields["policy"] = policy
    return hashlib.sha256(json.dumps(fields, sort_keys=True).encode()).hexdigest()


def apply_reviews(document_runs: List[Dict[str, Any]], reviews: Dict[str, Any]) -> List[Dict[str, Any]]:
    rows = []
    for doc in document_runs:
        items = []
        for item in doc["items"]:
            review = reviews.get(item["id"], {})
            if review.get("fingerprint") != item.get("fingerprint"):
                review = {"status": "open", "note": "", "author": "", "updated_at": None, "suggestion_decision": "pending"}
            items.append({**item, "review": {k: v for k, v in review.items() if k != "fingerprint"}})
        rows.append({**doc, "items": items})
    return rows


def aggregate_items(document_runs: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    """Keep the most severe occurrence visible in the matter overview."""
    worst: Dict[str, Any] = {}
    ranks = {"clear": 0, "medium": 1, "missing": 2, "high": 3}
    for doc in document_runs:
        for item in doc["items"]:
            previous = worst.get(item["key"])
            if previous is None or ranks[item["state"]] > ranks[previous["state"]]:
                worst[item["key"]] = item
    return [worst[key] for key in CHECK_KEYS if key in worst] + [item for key, item in worst.items() if key not in CHECK_KEYS]


def suggestion(key: str, policy: Dict[str, Any]) -> str:
    if key.startswith("custom:"):
        rule = next(rule for rule in policy["custom_rules"] if key == "custom:" + rule["id"])
        return rule["preferred_text"] or "Confirm the extracted value, exceptions and scope against this firm's rule before proposing a revision."
    suggestions = {
        "indemnity": f"Propose a liability cap based on at least {policy['liability_floor_months']} months of fees; counsel should confirm carve-outs and scope.",
        "renewal": f"Propose a non-renewal notice window no longer than {policy['renewal_notice_days']} days and confirm when notice must be delivered.",
        "law": "Confirm the governing law clause names an accepted jurisdiction: " + ", ".join(policy["accepted_laws"]) + ".",
        "breach": f"Propose an express duty to notify the customer of security incidents within {policy['breach_notice_hours']} hours; confirm the trigger and recipient.",
        "confidentiality": "Confirm confidentiality duties, permitted disclosures, exclusions and survival after termination.",
        "termination": "Confirm termination rights, notice, cure periods and the consequences of termination.",
        "assignment": "Confirm consent requirements for assignment, permitted transfers and change-of-control treatment.",
        "data_return": "Confirm return or deletion of customer data at termination, timing, retained copies and certification.",
    }
    return suggestions[key]
