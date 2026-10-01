"""Commercial MSA checklist. Findings are scored from clause text, not from a fixed story."""

from __future__ import annotations

import re
from typing import Any, Dict, List, Optional, Sequence

PLAYBOOK_NAME = "Commercial MSA v7"

PLAYBOOK: List[Dict[str, str]] = [
    {
        "key": "indemnity",
        "item": "Indemnity cap",
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


def score_indemnity(text: str) -> Dict[str, Any]:
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
    if months_n < 12:
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


def score_renewal(text: str) -> Dict[str, Any]:
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
    if days_n > 30:
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


def score_law(text: str) -> Dict[str, Any]:
    if re.search(r"delaware", text, re.IGNORECASE):
        quote = focus_quote(text, ["delaware", "governed"])
        return _result(
            state="clear",
            finding="Delaware",
            quote=quote,
            mark="Delaware",
            lead="Playbook accepts",
            tail="matches",
            absent=False,
        )
    match = re.search(
        r"laws of (?:the )?(?:State of )?([A-Z][A-Za-z]+)",
        text,
    )
    if match:
        place = match.group(1)
        quote = focus_quote(text, [place, "governed"])
        return _result(
            state="medium",
            finding=place,
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


def score_breach(text: str) -> Dict[str, Any]:
    security = bool(re.search(r"breach|security incident|security|incident", text, re.IGNORECASE))
    hours = find_unit(text, "hours?")
    if hours and security:
        hours_n = min(hours)
        quote = focus_quote(text, ["hours", "incident", "breach"])
        mark = str(hours_n) if str(hours_n) in quote else ""
        if hours_n <= 72:
            return _result(
                state="clear",
                finding=f"{hours_n}-hour notice",
                quote=quote,
                mark=mark or f"{hours_n} hours",
                lead="Playbook requires",
                tail=f"{hours_n}-hour notice meets 72",
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
) -> List[Dict[str, Any]]:
    items: List[Dict[str, Any]] = []
    for index, spec in enumerate(PLAYBOOK, start=1):
        key = spec["key"]
        chosen = select_chunk(spec["query"], chunks, recalls.get(key) or [])
        scored = _SCORERS[key](str(chosen.get("text") or ""))
        chunk_index = int(chosen.get("index") or 0)
        label = section_for(chunks, str(chosen.get("id") or ""), str(scored.get("quote") or ""))
        items.append(
            {
                "key": key,
                "number": f"{index:02d}",
                "item": spec["item"],
                "playbook": spec["playbook"],
                "query": spec["query"],
                "state": scored["state"],
                "finding": scored["finding"],
                "quote": scored["quote"],
                "mark": scored["mark"],
                "lead": scored["lead"],
                "tail": scored["tail"],
                "absent": scored["absent"],
                "chunk_id": chosen.get("id") or "",
                "chunk_index": chunk_index,
                "section": label,
                "citation": f"{label} · chunk {chunk_index}",
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
