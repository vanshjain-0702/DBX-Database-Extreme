"""Commercial MSA checklist. Findings are scored from clause text, not from a fixed story."""

from __future__ import annotations

import re
from typing import Any, Dict, List, Optional, Sequence

PLAYBOOK_NAME = "Commercial MSA v7"

PLAYBOOK: List[Dict[str, str]] = [
    {
        "key": "indemnity",
        "item": "Liability cap",
        "playbook": "Floor 12 months fees",
        "query": "supplier aggregate liability cap fees paid months preceding the claim indemnity",
    },
    {
        "key": "renewal",
        "item": "Auto-renewal",
        "playbook": "Prefer 30-day notice",
        "query": "agreement renew automatically written notice days non-renewal term",
    },
    {
        "key": "law",
        "item": "Governing law",
        "playbook": "Delaware accepted",
        "query": "governed by the laws of the state of delaware jurisdiction",
    },
    {
        "key": "breach",
        "item": "Breach notice",
        "playbook": "72-hour notice required",
        "query": "security incident breach notify hours notice notices writing customer data",
    },
]

_WORDS = {
    "one": 1,
    "two": 2,
    "three": 3,
    "four": 4,
    "five": 5,
    "six": 6,
    "seven": 7,
    "eight": 8,
    "nine": 9,
    "ten": 10,
    "twelve": 12,
    "twenty-four": 24,
    "thirty": 30,
    "forty-eight": 48,
    "sixty": 60,
    "seventy-two": 72,
    "ninety": 90,
}
_NUM = "|".join(sorted(_WORDS, key=len, reverse=True))
_STOP = {
    "a", "an", "and", "are", "as", "at", "be", "by", "for", "from", "if", "in",
    "is", "it", "its", "of", "on", "or", "that", "the", "this", "to", "under",
    "with", "shall", "any", "such", "not", "other", "than", "agreement",
}


def _tokens(text: str) -> set:
    out = set()
    for word in re.findall(r"[a-z0-9]+", text.lower()):
        if word in _STOP or len(word) <= 1:
            continue
        out.add(word)
        if word.endswith("s") and len(word) > 4:
            out.add(word[:-1])
    return out


def overlap(query: str, text: str) -> float:
    q = _tokens(query)
    if not q:
        return 0.0
    return len(q & _tokens(text)) / len(q)


def _number(raw: str) -> Optional[int]:
    raw = raw.lower().strip()
    if raw.isdigit():
        return int(raw)
    return _WORDS.get(raw)


def find_unit(text: str, unit: str) -> List[int]:
    pat = re.compile(
        rf"({_NUM}|\d+)\s*(?:\(\s*(\d+)\s*\))?\s*{unit}",
        re.IGNORECASE,
    )
    found: List[int] = []
    for match in pat.finditer(text):
        if match.group(2):
            found.append(int(match.group(2)))
            continue
        number = _number(match.group(1))
        if number is not None:
            found.append(number)
    return found


def _sentences(text: str) -> List[str]:
    parts = re.split(r"(?<=[.!?])\s+", text.strip())
    return [p.strip() for p in parts if p.strip()]


def _matching_sentences(text: str, pattern: str) -> List[str]:
    rx = re.compile(pattern, re.IGNORECASE)
    hits = [s for s in _sentences(text) if rx.search(s)]
    return hits or ([text.strip()] if text.strip() else [])


def section_for(chunks: Sequence[Dict[str, Any]], chosen_id: str, quote: str) -> str:
    """Section mark that belongs to the cited sentence, carried across chunks."""
    last = "—"
    for chunk in chunks:
        text = str(chunk.get("text") or "")
        marks = list(re.finditer(r"§\s*(\d+(?:\.\d+)?)", text))
        if str(chunk.get("id")) != str(chosen_id):
            if marks:
                last = f"§{marks[-1].group(1)}"
            continue
        if quote:
            in_quote = re.search(r"§\s*(\d+(?:\.\d+)?)", quote)
            if in_quote:
                return f"§{in_quote.group(1)}"
            idx = text.find(quote[:60])
            if idx >= 0:
                prior = [m for m in marks if m.start() <= idx]
                if prior:
                    return f"§{prior[-1].group(1)}"
        if marks:
            return f"§{marks[0].group(1)}"
        return last
    return "—"


def section_label(text: str) -> str:
    match = re.search(r"§\s*(\d+(?:\.\d+)?)", text)
    if match:
        return f"§{match.group(1)}"
    match = re.search(r"\b(\d+\.\d+)\b", text)
    if match:
        return f"§{match.group(1)}"
    match = re.search(r"Article\s+(\d+)", text, re.IGNORECASE)
    if match:
        return f"Article {match.group(1)}"
    return "—"


def focus_quote(text: str, needles: Sequence[str]) -> str:
    sentences = _sentences(text)
    lowered = [n.lower() for n in needles if n]
    ranked = []
    for sentence in sentences:
        folded = sentence.lower()
        hits = sum(1 for needle in lowered if needle in folded)
        if hits:
            ranked.append((hits, len(sentence), sentence))
    if ranked:
        ranked.sort(key=lambda row: (row[0], row[1]), reverse=True)
        return ranked[0][2]
    folded = text.strip()
    return folded[:700]


def _result(
    *,
    state: str,
    finding: str,
    quote: str,
    mark: str,
    lead: str,
    tail: str,
    absent: bool,
) -> Dict[str, Any]:
    return {
        "state": state,
        "finding": finding,
        "quote": quote,
        "mark": mark,
        "lead": lead,
        "tail": tail,
        "absent": absent,
    }


def score_indemnity(text: str, floor_months: int = 12) -> Dict[str, Any]:
    scoped = " ".join(_matching_sentences(text, r"liabilit|indemn|fees"))
    if not re.search(r"liabilit|indemn", text, re.IGNORECASE):
        return _result(
            state="missing",
            finding="Clause absent",
            quote=focus_quote(text, ["liability", "indemn"]),
            mark="",
            lead="Playbook floor",
            tail="contract silent on a liability cap",
            absent=True,
        )
    months = find_unit(scoped, "months?")
    quote = focus_quote(scoped, ["months", "liability", "fees"])
    if not months:
        return _result(
            state="medium",
            finding="Cap not stated in months",
            quote=quote,
            mark="",
            lead="Playbook floor",
            tail="no month cap stated",
            absent=False,
        )
    months_n = min(months)
    mark = ""
    word = next((w for w, n in _WORDS.items() if n == months_n), "")
    if word and word in quote.lower():
        mark = word + " months" if "month" in quote.lower() else word
    elif str(months_n) in quote:
        mark = str(months_n)
    if months_n < floor_months:
        return _result(
            state="high",
            finding=f"Cap is {months_n} months",
            quote=quote,
            mark=mark or f"{months_n} months",
            lead="Playbook floor",
            tail=f"contract gives {months_n} months",
            absent=False,
        )
    return _result(
        state="clear",
        finding=f"{months_n} months",
        quote=quote,
        mark=mark or f"{months_n} months",
        lead="Playbook floor",
        tail=f"{months_n} months meets the floor",
        absent=False,
    )


def score_renewal(text: str, notice_days: int = 30) -> Dict[str, Any]:
    if not re.search(r"renew", text, re.IGNORECASE):
        return _result(
            state="missing",
            finding="Clause absent",
            quote=focus_quote(text, ["renew", "term"]),
            mark="",
            lead="Playbook prefers",
            tail="contract silent on renewal",
            absent=True,
        )
    scoped = " ".join(_matching_sentences(text, r"renew|non-renewal"))
    days = find_unit(scoped, "days?")
    quote = focus_quote(scoped, ["days", "renew"])
    if not days:
        return _result(
            state="medium",
            finding="Notice window not stated",
            quote=quote,
            mark="",
            lead="Playbook prefers",
            tail="no day count stated",
            absent=False,
        )
    days_n = max(days) if re.search(r"renew", scoped, re.IGNORECASE) else min(days)
    # Prefer the number attached to renewal notice, not a cure period in another sentence.
    renew_sentences = _matching_sentences(text, r"renew")
    renew_days = find_unit(" ".join(renew_sentences), "days?")
    if renew_days:
        days_n = max(renew_days)
        quote = focus_quote(" ".join(renew_sentences), ["days", "renew"])
    mark = f"{days_n} days" if str(days_n) in quote or str(days_n) in text else ""
    if not mark:
        for word, number in _WORDS.items():
            if number == days_n and word in quote.lower():
                mark = word
                break
    if days_n > notice_days:
        return _result(
            state="medium",
            finding=f"{days_n}-day notice",
            quote=quote,
            mark=mark or str(days_n),
            lead="Playbook prefers",
            tail=f"contract requires {days_n} days",
            absent=False,
        )
    return _result(
        state="clear",
        finding=f"{days_n}-day notice",
        quote=quote,
        mark=mark or str(days_n),
        lead="Playbook prefers",
        tail=f"{days_n}-day notice matches",
        absent=False,
    )


def score_law(text: str, accepted_laws: Sequence[str] = ("Delaware",)) -> Dict[str, Any]:
    scoped = " ".join(s for s in _sentences(text) if re.search(r"governed|governing|laws of", s, re.IGNORECASE))
    law_pattern = r"laws of (?:the )?(?:State of )?([A-Za-z][A-Za-z ]*?)(?:[.;,]|$| without | excluding | and )"
    places = list(dict.fromkeys(place.strip() for place in re.findall(law_pattern, scoped, re.IGNORECASE)))
    accepted = next((law for law in accepted_laws if len(places) == 1 and law.casefold() == places[0].casefold()), None)
    if accepted:
        quote = focus_quote(scoped, [accepted, "governed"])
        return _result(
            state="clear",
            finding=accepted,
            quote=quote,
            mark=accepted,
            lead="Playbook accepts",
            tail="matches",
            absent=False,
        )
    match = re.search(
        law_pattern,
        scoped, re.IGNORECASE,
    )
    if match:
        place = match.group(1).strip()
        quote = focus_quote(text, [place, "governed"])
        return _result(
            state="medium",
            finding=place if len(places) <= 1 else "Multiple governing laws require review",
            quote=quote,
            mark=place,
            lead="Playbook accepts",
            tail=f"contract says {place}",
            absent=False,
        )
    if re.search(r"govern", text, re.IGNORECASE):
        return _result(
            state="medium",
            finding="Law not identified",
            quote=focus_quote(text, ["governed", "laws"]),
            mark="",
            lead="Playbook accepts",
            tail="forum not named",
            absent=False,
        )
    return _result(
        state="missing",
        finding="Clause absent",
        quote=focus_quote(text, ["law", "govern"]),
        mark="",
        lead="Playbook accepts",
        tail="contract silent on governing law",
        absent=True,
    )


def score_breach(text: str, notice_hours: int = 72) -> Dict[str, Any]:
    text = " ".join(s for s in _sentences(text) if re.search(r"breach|security incident|security|incident", s, re.IGNORECASE))
    security = bool(re.search(r"breach|security incident|security|incident", text, re.IGNORECASE))
    hours = find_unit(text, "hours?")
    if hours and security:
        hours_n = max(hours)
        quote = focus_quote(text, ["hours", "incident", "breach"])
        mark = str(hours_n) if str(hours_n) in quote else ""
        if hours_n <= notice_hours:
            return _result(
                state="clear",
                finding=f"{hours_n}-hour notice",
                quote=quote,
                mark=mark or f"{hours_n} hours",
                lead="Playbook requires",
                tail=f"{hours_n}-hour notice meets {notice_hours}",
                absent=False,
            )
        return _result(
            state="medium",
            finding=f"{hours_n}-hour notice",
            quote=quote,
            mark=mark or f"{hours_n} hours",
            lead="Playbook requires",
            tail=f"contract allows {hours_n} hours",
            absent=False,
        )
    quote = focus_quote(text, ["notice", "notices", "security", "breach"])
    return _result(
        state="missing",
        finding="Clause absent",
        quote=quote,
        mark="",
        lead="Playbook requires",
        tail="contract silent",
        absent=True,
    )


_SCORERS = {
    "indemnity": score_indemnity,
    "renewal": score_renewal,
    "law": score_law,
    "breach": score_breach,
}

EXTRA_CHECKS = [
    {"key": "confidentiality", "item": "Confidentiality", "playbook": "Confidentiality duties require review", "query": "confidential confidentiality disclosure information survive"},
    {"key": "termination", "item": "Termination rights", "playbook": "Termination and cure rights require review", "query": "termination terminate cure material breach notice"},
    {"key": "assignment", "item": "Assignment", "playbook": "Assignment consent requires review", "query": "assign assignment transfer consent change control"},
    {"key": "data_return", "item": "Data return and deletion", "playbook": "Exit data handling requires review", "query": "return delete deletion destroy customer personal data termination"},
]
ALL_CHECKS = PLAYBOOK + EXTRA_CHECKS
_PRESENCE_PATTERNS = {
    "confidentiality": r"\bconfidential(?:ity)?\b|non-disclosure",
    "termination": r"\bterminat(?:e|ion|ing)\b",
    "assignment": r"\bassign(?:ment|ed)?\b",
    "data_return": r"\b(?:return|delet(?:e|ion)|destroy)\b.{0,100}\b(?:data|information|records)\b|\b(?:data|information|records)\b.{0,100}\b(?:return|delet(?:e|ion)|destroy)\b",
}


def _presence_score(key: str, text: str) -> Dict[str, Any]:
    pattern = _PRESENCE_PATTERNS[key]
    found = re.search(pattern, text, re.IGNORECASE | re.DOTALL)
    return _result(
        state="medium" if found else "missing",
        finding="Matching language located; scope requires review" if found else "No matching provision located",
        quote=focus_quote(text, [found.group(0)]) if found else "",
        mark="", lead="Reviewer must confirm", tail="Scope and enforceability require counsel review", absent=not bool(found),
    )


def select_chunk(
    query: str,
    chunks: Sequence[Dict[str, Any]],
    hits: Sequence[Dict[str, Any]],
) -> Dict[str, Any]:
    by_id = {str(c["id"]): c for c in chunks}
    vector_choice = None
    for hit in hits:
        chosen = by_id.get(str(hit.get("id")))
        if chosen is not None:
            vector_choice = chosen
            break
    ranked = sorted(chunks, key=lambda c: overlap(query, str(c.get("text") or "")), reverse=True)
    lexical = ranked[0] if ranked else None
    if vector_choice is not None and overlap(query, str(vector_choice.get("text") or "")) >= 0.2:
        return {**vector_choice, "via": "vector"}
    if lexical is not None and overlap(query, str(lexical.get("text") or "")) > 0:
        return {**lexical, "via": "vector" if vector_choice and lexical.get("id") == vector_choice.get("id") else "read"}
    if vector_choice is not None:
        return {**vector_choice, "via": "vector"}
    if lexical is not None:
        return {**lexical, "via": "read"}
    return {"id": "", "text": "", "doc_name": "", "index": 0, "via": "none"}


def evaluate(
    chunks: Sequence[Dict[str, Any]],
    recalls: Dict[str, Sequence[Dict[str, Any]]],
    policy: Optional[Dict[str, Any]] = None,
) -> List[Dict[str, Any]]:
    items: List[Dict[str, Any]] = []
    checks = [s for s in ALL_CHECKS if s["key"] in policy["enabled_checks"]] if policy else PLAYBOOK
    for index, spec in enumerate(checks, start=1):
        key = spec["key"]
        query = spec["query"]
        standard = spec["playbook"]
        if policy:
            if key == "law":
                query = "governed governing laws jurisdiction " + " ".join(policy["accepted_laws"])
                standard = "Accepted law: " + ", ".join(policy["accepted_laws"])
            elif key == "indemnity":
                standard = f"Floor {policy['liability_floor_months']} months fees"
            elif key == "renewal":
                standard = f"Prefer {policy['renewal_notice_days']}-day notice"
            elif key == "breach":
                standard = f"{policy['breach_notice_hours']}-hour notice required"
        # Prefer a matching provision before relying on broad semantic recall.
        patterns = {"indemnity": r"liabilit|indemn", "renewal": r"renew", "law": r"governed|governing|laws of", "breach": r"security|breach|incident", **_PRESENCE_PATTERNS}
        matching = [c for c in chunks
            if re.search(patterns[key], str(c.get("text") or ""), re.IGNORECASE | re.DOTALL)
            and not re.fullmatch(r"Article\s+\d+\s*[—:-][^\n.!?]{0,100}", str(c.get("text") or "").strip(), re.IGNORECASE)]
        candidates = matching or chunks
        chosen = select_chunk(query, candidates, recalls.get(key) or [])

        def score_text(text: str) -> Dict[str, Any]:
            if key not in _SCORERS:
                return _presence_score(key, text)
            if policy and key == "indemnity":
                return score_indemnity(text, policy["liability_floor_months"])
            if policy and key == "renewal":
                return score_renewal(text, policy["renewal_notice_days"])
            if policy and key == "law":
                return score_law(text, policy["accepted_laws"])
            if policy and key == "breach":
                return score_breach(text, policy["breach_notice_hours"])
            return _SCORERS[key](text)

        scored = score_text(str(chosen.get("text") or ""))
        # A retrieved compliant clause must not hide a conflicting provision in
        # another passage of this document. Prefer the worst located result.
        located = [(chunk, score_text(str(chunk.get("text") or ""))) for chunk in matching]
        located = [(chunk, result) for chunk, result in located if not result["absent"]]
        if located:
            ranks = {"clear": 0, "medium": 1, "high": 2}
            worst_chunk, scored = max(located, key=lambda row: (ranks[row[1]["state"]], overlap(query, str(row[0].get("text") or ""))))
            via = "vector" if any(hit.get("id") == worst_chunk["id"] for hit in recalls.get(key, [])) else "read"
            chosen = {**worst_chunk, "via": via}
        # An absent provision has no supporting quotation or source citation.
        if scored["absent"]:
            scored["quote"] = ""
        chunk_index = int(chosen.get("index") or 0)
        label = section_for(chunks, str(chosen.get("id") or ""), str(scored.get("quote") or ""))
        items.append(
            {
                "key": key,
                "number": f"{index:02d}",
                "item": spec["item"],
                "playbook": standard,
                "query": query,
                "state": scored["state"],
                "finding": scored["finding"],
                "quote": scored["quote"],
                "mark": scored["mark"],
                "lead": scored["lead"],
                "tail": scored["tail"],
                "absent": scored["absent"],
                "chunk_id": "" if scored["absent"] else chosen.get("id") or "",
                "chunk_index": chunk_index,
                "section": label,
                "citation": "Not located in document" if scored["absent"] else f"{label} · chunk {chunk_index}",
                "doc_name": chosen.get("doc_name") or "",
                "doc_id": chosen.get("doc_id") or "",
                "via": chosen.get("via") or "none",
            }
        )
    return items


def tally(items: Sequence[Dict[str, Any]]) -> Dict[str, int]:
    counts = {"high": 0, "medium": 0, "clear": 0, "missing": 0}
    for item in items:
        state = str(item.get("state") or "")
        if state in counts:
            counts[state] += 1
    counts["open"] = counts["high"] + counts["medium"] + counts["missing"]
    return counts
