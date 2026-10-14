"""Pre-bill review against explicit client guidelines; no fee or ethics verdicts."""
from __future__ import annotations

import csv
import io
import math
import re
from datetime import date

# Original ABA Litigation task codes, including phase codes some clients accept.
TASK_CODES = set("L100 L110 L120 L130 L140 L150 L160 L190 L200 L210 L220 L230 L240 L250 L260 L300 L310 L320 L330 L340 L350 L390 L400 L410 L420 L430 L440 L450 L460 L470 L500 L510 L520 L530".split())
ACTIVITY_CODES = {f"A{i}" for i in range(101, 112)}
CODE_SOURCES = [
    {"title": "ABA Litigation codes (1997)", "url": "https://utbms.com/aba-litigation-codes/"},
    {"title": "ABA Activity codes (1997)", "url": "https://utbms.com/aba-activity-codes/"},
]
DEFAULT_BILLING_POLICY = {
    "name": "Client pre-bill guidelines", "require_task_code": True, "require_activity_code": True,
    "flag_block_billing": True, "min_narrative_words": 5, "max_entry_hours": 8.0,
    "routine_max_hours": 1.0, "routine_terms": ["scheduling", "calendaring", "schedule meeting", "calendar deadline"],
}
FIELDS = ["date", "timekeeper", "hours", "task_code", "activity_code", "narrative"]


def normalize_billing_policy(raw: dict | None = None) -> dict:
    policy = {**DEFAULT_BILLING_POLICY, **(raw or {})}
    name = policy["name"]
    if not isinstance(name, str) or not name.strip() or len(name) > 100:
        raise ValueError("billing guideline name must contain 1–100 characters")
    out = {"name": name.strip()}
    for key in ["require_task_code", "require_activity_code", "flag_block_billing"]:
        if not isinstance(policy[key], bool):
            raise ValueError(f"{key} must be a boolean")
        out[key] = policy[key]
    value = policy["min_narrative_words"]
    if isinstance(value, bool) or not isinstance(value, int) or not 1 <= value <= 100:
        raise ValueError("min_narrative_words must be an integer from 1 to 100")
    out["min_narrative_words"] = value
    for key in ["max_entry_hours", "routine_max_hours"]:
        value = policy[key]
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value) or not 0 < value <= 24:
            raise ValueError(f"{key} must be greater than zero and at most 24")
        out[key] = float(value)
    terms = policy["routine_terms"]
    if not isinstance(terms, list) or len(terms) > 30 or any(not isinstance(term, str) or not term.strip() or len(term) > 100 for term in terms):
        raise ValueError("routine_terms must contain at most 30 nonempty phrases")
    out["routine_terms"] = list(dict.fromkeys(t.strip().lower() for t in terms))
    return out


def parse_entries(body: str) -> list[dict]:
    if not isinstance(body, str) or len(body) > 1_000_000:
        raise ValueError("CSV must be at most 1,000,000 characters")
    try:
        reader = csv.DictReader(io.StringIO(body.lstrip("\ufeff")), strict=True)
        if reader.fieldnames != FIELDS:
            raise ValueError("CSV header must be: " + ",".join(FIELDS))
        rows = []
        for row in reader:
            if len(rows) >= 500:
                raise ValueError("review at most 500 entries at a time")
            if None in row or any(row.get(field) is None for field in FIELDS):
                raise ValueError(f"entry {len(rows) + 1}: incorrect number of columns; quote narratives containing commas")
            item = {k: row[k].strip() for k in FIELDS}
            if not item["timekeeper"] or len(item["timekeeper"]) > 100 or not item["narrative"] or len(item["narrative"]) > 10000:
                raise ValueError(f"entry {len(rows) + 1}: timekeeper and narrative are required (maximum 100 and 10,000 characters)")
            if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", item["date"]):
                raise ValueError(f"entry {len(rows) + 1}: date must use YYYY-MM-DD")
            date.fromisoformat(item["date"])
            try:
                hours = float(item["hours"])
            except ValueError as exc:
                raise ValueError(f"entry {len(rows) + 1}: hours must be numeric") from exc
            if not math.isfinite(hours) or not 0 < hours <= 24:
                raise ValueError(f"entry {len(rows) + 1}: hours must be greater than zero and at most 24")
            if any(len(item[field]) > 20 for field in ["task_code", "activity_code"]):
                raise ValueError("billing codes must be at most 20 characters")
            rows.append({**item, "hours": hours, "task_code": item["task_code"].upper(), "activity_code": item["activity_code"].upper(), "row": len(rows) + 2})
    except csv.Error as exc:
        raise ValueError("Malformed CSV; use double quotes around narratives containing commas or line breaks") from exc
    if not rows:
        raise ValueError("add at least one time entry")
    return rows


def audit_entries(entries: list[dict], policy: dict) -> dict:
    results = []
    vague = re.compile(r"\b(?:worked? on (?:the )?(?:case|matter|file)|attention to (?:the )?(?:case|matter)|miscellaneous|general work|various tasks)\b", re.I)
    action = re.compile(r"^\s*(?:review\w*|draft\w*|research\w*|call\w*|email\w*|prepar\w*|analy[sz]\w*|attend\w*|meet\w*|discuss\w*|revis\w*)\s+\S", re.I)
    for entry in entries:
        flags = []
        def flag(code, description, next_step):
            flags.append({"code": code, "description": description, "next_step": next_step})
        for field, allowed, required in [("task_code", TASK_CODES, policy["require_task_code"]), ("activity_code", ACTIVITY_CODES, policy["require_activity_code"])]:
            code = entry[field]
            if not code and required:
                flag("missing_" + field, f"Required {field.replace('_', ' ')} is missing", "Choose the client's approved code for the work actually performed.")
            elif code and code not in allowed:
                flag("unsupported_" + field, f"{code} is outside the selected ABA 1997 code set", "Confirm the client uses this code set; JURIX does not validate other UTBMS sets or custom codes.")
        narrative = entry["narrative"]
        if vague.search(narrative) or len(re.findall(r"\b\w+\b", narrative)) < policy["min_narrative_words"]:
            flag("vague_narrative", "Narrative may not identify the work and its purpose", "Describe the specific task, subject and purpose using facts supplied by the timekeeper.")
        segments = re.split(r";|\b(?:and then|then|and)\b|(?<=[.!?])\s+", narrative, flags=re.I)
        if policy["flag_block_billing"] and sum(bool(action.match(s)) for s in segments) >= 2:
            flag("possible_block_billing", "Multiple task descriptions share one hours total", "Ask the timekeeper to split actual tasks and allocate the recorded hours; do not invent an allocation.")
        if entry["hours"] > policy["max_entry_hours"]:
            flag("hours_threshold", f"{entry['hours']:g} hours exceeds the chosen {policy['max_entry_hours']:g}-hour entry threshold", "Verify the recorded time and the client's guidelines; a threshold flag is not an overbilling determination.")
        matched = next((term for term in policy["routine_terms"] if re.search(r"(?<!\w)" + re.escape(term) + r"(?!\w)", narrative, re.I)), None)
        if matched and entry["hours"] > policy["routine_max_hours"]:
            flag("routine_threshold", f"Routine phrase '{matched}' exceeds the chosen {policy['routine_max_hours']:g}-hour threshold", "Confirm task complexity and client guidance before changing the entry.")
        results.append({**entry, "flags": flags})
    return {"entries": results, "policy": policy, "code_set": "ABA Litigation and Activity (1997)", "sources": CODE_SOURCES,
            "total_hours": round(sum(e["hours"] for e in entries), 4), "flagged_entries": sum(bool(e["flags"]) for e in results),
            "flag_count": sum(len(e["flags"]) for e in results), "method": "Deterministic pre-bill checks; billing partner verification required. No findings does not certify code-to-narrative accuracy or invoice acceptance."}
