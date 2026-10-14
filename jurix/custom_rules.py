"""Explicit numeric firm rules. Ambiguous wording always needs counsel review."""
from __future__ import annotations

import re
from typing import Any
from urllib.parse import urlsplit

KINDS = {"nda_term", "liability_amount", "semantic"}
CURRENCIES = {"USD", "INR", "EUR", "GBP"}


def normalize_rules(raw: Any) -> list[dict]:
    if not isinstance(raw, list) or len(raw) > 20:
        raise ValueError("custom_rules must be a list of at most 20 rules")
    out, ids = [], set()
    for value in raw:
        if not isinstance(value, dict):
            raise ValueError("each custom rule must be an object")
        rule = {**value}
        rid = rule.get("id", "")
        if not isinstance(rid, str) or not re.fullmatch(r"[a-z0-9_-]{1,40}", rid) or rid in ids:
            raise ValueError("custom rules need unique lowercase IDs (1–40 letters, digits, _ or -)")
        ids.add(rid)
        for field, limit in [("name", 100), ("preferred_text", 4000), ("authority_title", 200), ("authority_url", 2000)]:
            text = rule.get(field, "")
            if not isinstance(text, str) or len(text) > limit or (field == "name" and not text.strip()):
                raise ValueError(f"invalid custom rule {field}")
            rule[field] = text.strip()
        if rule.get("kind") not in KINDS or rule.get("applies_to", "all") not in {"all", "nda"}:
            raise ValueError("unsupported rule type or document scope")
        instructions = rule.get("instructions", "")
        if not isinstance(instructions, str) or len(instructions) > 4000 or (rule["kind"] == "semantic" and not instructions.strip()):
            raise ValueError("Semantic rules require instructions of 1–4,000 characters")
        maximum = 120 if rule["kind"] == "nda_term" else 1_000_000_000
        threshold = rule.get("threshold")
        if isinstance(threshold, bool) or not isinstance(threshold, int) or not 1 <= threshold <= maximum:
            raise ValueError(f"rule threshold must be an integer from 1 to {maximum}")
        currency = rule.get("currency", "USD")
        if currency not in CURRENCIES:
            raise ValueError("supported currencies: USD, INR, EUR, GBP")
        url = rule["authority_url"]
        parts = urlsplit(url)
        if url and (parts.scheme != "https" or not parts.hostname or parts.username or parts.password or any(ord(c) < 33 for c in url)):
            raise ValueError("authority links must be HTTPS URLs without embedded credentials")
        if bool(url) != bool(rule["authority_title"]):
            raise ValueError("provide both an authority title and its URL")
        normalized = {key: rule[key] for key in ["id", "name", "kind", "threshold", "preferred_text", "authority_title", "authority_url"]} | {
            "currency": currency, "applies_to": rule.get("applies_to", "all"),
        }
        if rule["kind"] == "semantic":
            normalized["instructions"] = instructions.strip()
        out.append(normalized)
    return out


def _score(text: str, rule: dict) -> tuple[str, str, str]:
    from playbook import _sentences, _NUM, _number
    sentences = _sentences(text)
    if rule["kind"] == "nda_term":
        relevant = [s for s in sentences if re.search(r"\bterm\b|\bexpire|\bduration|remains? in (?:force|effect)|\brenew", s, re.I)]
        if not relevant:
            return "missing", "No agreement term located", ""
        values = []
        for sentence in relevant:
            # Survival, renewals and compound terms cannot establish the entire NDA term.
            ambiguous = re.search(r"surviv|renew|initial|confidentiality obligations|trade secret|indefinite|perpetual|unless|except|payment term|invoice|subject to|notwithstanding|\bnot\b", sentence, re.I)
            amounts = list(re.finditer(rf"\b({_NUM}|\d+)\s*(?:\(\s*(\d+)\s*\))?\s*(years?|months?)\b", sentence, re.I))
            if ambiguous or len(amounts) != 1:
                values.append(("medium", "Term wording needs manual verification", sentence))
                continue
            match = amounts[0]
            prefix = sentence[:match.start()].lower()
            if re.search(r"\d\.$|\b(?:twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety|hundred|thousand)\s+$", prefix):
                values.append(("medium", "Unsupported compound or decimal term value", sentence))
                continue
            if match[2] and _number(match[1]) != int(match[2]):
                values.append(("medium", "Conflicting written and numeric term values", sentence))
                continue
            amount = int(match[2]) if match[2] else _number(match[1])
            months = amount * (12 if match[3].lower().startswith("year") else 1)
            state = "high" if months > rule["threshold"] else "clear"
            values.append((state, f"Agreement term: {months} months; firm maximum: {rule['threshold']}", sentence))
    else:
        relevant = [s for s in sentences if re.search(r"\bliabilit\w*\b", s, re.I) and re.search(r"\bcap\w*\b|not exceed|limited to|limit of", s, re.I)]
        if not relevant:
            return "missing", "No fixed monetary liability cap located", ""
        values = []
        money = r"(?:(USD|INR|EUR|GBP|US\$|\$|₹|€|£)\s*(\d[\d,]*(?:\.\d{1,2})?)|(\d[\d,]*(?:\.\d{1,2})?)\s*(USD|INR|EUR|GBP))"
        mapping = {"$": "USD", "US$": "USD", "₹": "INR", "€": "EUR", "£": "GBP"}
        for sentence in relevant:
            hits = list(re.finditer(money, sentence, re.I))
            if len(hits) != 1 or re.search(r"fees|greater|lesser|except|exclud|unless|million|billion|thousand|lakh|crore|per claim|per event|per incident|subject to|notwithstanding|not (?:capped|limited)|does not|no (?:cap|limit)|\b(?:CAD|AUD|NZD|SGD|HKD)\b|[CA]\$", sentence, re.I):
                values.append(("medium", "Cap basis, amount or exceptions need manual verification", sentence))
                continue
            hit = hits[0]
            currency = (hit[1] or hit[4]).upper()
            currency = mapping.get(currency, currency)
            amount = float((hit[2] or hit[3]).replace(",", ""))
            if currency != rule["currency"]:
                values.append(("medium", f"Cap is in {currency}; rule is in {rule['currency']}. No currency conversion applied", sentence))
            else:
                state = "high" if amount < rule["threshold"] else "clear"
                values.append((state, f"Fixed cap: {currency} {amount:,.2f}; firm minimum: {rule['threshold']:,}", sentence))
    return max(values, key=lambda row: {"clear": 0, "medium": 1, "high": 2}[row[0]])


def evaluate_rules(chunks: list[dict], rules: list[dict], start: int = 0) -> list[dict]:
    from playbook import section_for
    full = "\n".join(str(c.get("text", "")) for c in chunks)
    name = chunks[0].get("doc_name", "") if chunks else ""
    is_nda = bool(re.search(r"\bNDA\b|non[\s-]*disclosure agreement|confidentiality agreement", name + "\n" + full, re.I))
    items = []
    for rule in rules:
        if rule["kind"] == "semantic":
            continue
        if rule["applies_to"] == "nda" and not is_nda:
            continue
        scored = [(chunk, _score(str(chunk.get("text", "")), rule)) for chunk in chunks]
        located = [(c, r) for c, r in scored if r[0] != "missing"]
        chunk, result = max(located, key=lambda row: {"clear": 0, "medium": 1, "high": 2}[row[1][0]]) if located else ({}, ("missing", "No matching provision located", ""))
        state, finding, quote = result
        standard = f"Agreement term ≤ {rule['threshold']} months" if rule["kind"] == "nda_term" else f"Fixed liability cap ≥ {rule['currency']} {rule['threshold']:,}"
        section = section_for(chunks, str(chunk.get("id", "")), quote)
        items.append({
            "key": "custom:" + rule["id"], "number": f"{start + len(items) + 1:02d}", "item": rule["name"],
            "playbook": standard, "state": state, "finding": finding, "quote": quote,
            "mark": "", "lead": "Firm rule", "tail": finding, "absent": state == "missing",
            "chunk_id": chunk.get("id", ""), "chunk_index": chunk.get("index", 0),
            "section": section, "citation": "Not located in document" if not quote else section,
            "doc_name": name, "via": "read", "proposed_text": rule["preferred_text"] if state != "clear" else "",
            "authority": {"title": rule["authority_title"], "url": rule["authority_url"], "provenance": "firm_supplied"} if rule["authority_url"] else None,
        })
    return items


def source_anchor(item: dict, chunks: list[dict], original: str) -> dict | None:
    """Verify an exact substring before exposing line/character coordinates."""
    if not item.get("quote") or not item.get("chunk_id"):
        return None
    cursor = 0
    for chunk in sorted(chunks, key=lambda c: c["index"]):
        text = str(chunk["text"])
        position = original.find(text, cursor)
        if position < 0:
            return None
        if chunk["id"] == item["chunk_id"]:
            local = text.find(item["quote"])
            if local < 0:
                return None
            start = position + local
            end = start + len(item["quote"])
            return {"start": start, "end": end, "line_start": original.count("\n", 0, start) + 1, "line_end": original.count("\n", 0, end) + 1}
        cursor = position + 1  # Windows overlap, including repeated passage text.
    return None
