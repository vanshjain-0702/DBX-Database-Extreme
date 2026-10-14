"""LLM rule checks with verified quotations and explicit provider provenance."""
from __future__ import annotations

import hashlib
import re

PROMPT_VERSION = "jurix-rule-review-v1"
SCHEMA = {"type": "object", "properties": {
    "state": {"type": "string", "enum": ["high", "medium", "clear", "missing"]},
    "finding": {"type": "string"}, "quote": {"type": "string"},
    "start": {"type": "integer"}, "end": {"type": "integer"}},
    "required": ["state", "finding", "quote", "start", "end"], "additionalProperties": False}


def evaluate_semantic(doc, chunks, rules, providers, start=0):
    items = []
    for rule in rules:
        if rule["kind"] != "semantic":
            continue
        if rule["applies_to"] == "nda" and not re.search(r"\bnda\b|non[\s-]*disclosure agreement|confidentiality agreement", doc["name"] + "\n" + doc["text"], re.I):
            continue
        result = providers.structured("firm_rule", SCHEMA,
            "Check only the supplied firm rule against the complete supplied document. Use high for an apparent rule conflict, medium for uncertainty, clear only when a precise passage supports the rule, and missing when no supporting provision exists. Copy one exact, contiguous source passage and its Unicode code-point offsets. Do not provide a general legal verdict. For missing, quote is empty and both offsets are zero.",
            {"rule": rule["instructions"], "source": doc["text"]})
        state, quote = result.get("state"), result.get("quote")
        if state not in {"high", "medium", "clear", "missing"} or not isinstance(quote, str) or not isinstance(result.get("finding"), str) or len(result["finding"]) > 4000:
            raise ValueError("AI returned an invalid finding; no audit was saved")
        anchor = None
        if state == "missing":
            if quote or result.get("start") != 0 or result.get("end") != 0:
                raise ValueError("AI missing finding contained unverified evidence; no audit was saved")
        else:
            left, right = result.get("start"), result.get("end")
            if type(left) is not int or type(right) is not int or not 0 <= left < right <= len(doc["text"]) or not quote or doc["text"][left:right] != quote:
                raise ValueError("AI quotation did not match the source; no audit was saved")
            anchor = {"start": left, "end": right, "line_start": doc["text"].count("\n", 0, left)+1, "line_end": doc["text"].count("\n", 0, right)+1}
        chunk = next((c for c in chunks if quote and quote in c["text"]), {})
        items.append({"key": "custom:" + rule["id"], "number": f"{start + len(items) + 1:02d}", "item": rule["name"], "playbook": rule["instructions"], "state": state, "finding": result["finding"], "quote": quote,
            "mark": "", "lead": "Firm AI rule", "tail": result["finding"], "absent": state == "missing", "chunk_id": chunk.get("id", ""), "chunk_index": chunk.get("index", 0), "section": "Source passage", "citation": "Not located in document" if not anchor else f"Lines {anchor['line_start']}–{anchor['line_end']}", "doc_name": doc["name"], "via": "read", "anchor": anchor,
            "proposed_text": rule["preferred_text"] if state != "clear" else "", "authority": {"title": rule["authority_title"], "url": rule["authority_url"], "provenance": "firm_supplied"} if rule["authority_url"] else None,
            "method": "LLM assessment against an explicit firm rule; quoted source verified", "provenance": {"provider": "OpenAI", "model": providers.model, "prompt_version": PROMPT_VERSION, "source_sha256": hashlib.sha256(doc["text"].encode()).hexdigest()}})
    return items
