"""Checklist scoring without a live DBX node."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from embed import embed
from playbook import evaluate, tally
from samples import SAMPLE_DPA, SAMPLE_MSA


def _chunks(text: str, doc_name: str):
    # One chunk per paragraph so each clause can be cited on its own.
    parts = [p.strip() for p in text.split("\n\n") if p.strip()]
    return [
        {"id": f"c{i}", "text": part, "index": i, "doc_name": doc_name}
        for i, part in enumerate(parts)
    ]


def _cosine(a, b):
    return sum(x * y for x, y in zip(a, b))


def test_feature_hash_puts_the_query_on_the_clause():
    cap = "Supplier's aggregate liability shall not exceed the fees paid in the three months preceding the claim."
    law = "This Agreement is governed by the laws of the State of Delaware."
    query = embed("supplier aggregate liability cap fees paid months preceding the claim indemnity")
    assert _cosine(query, embed(cap)) > _cosine(query, embed(law))


def test_msa_checklist_matches_the_playbook():
    items = evaluate(_chunks(SAMPLE_MSA, "Harborline_MSA_v3.txt"), {})
    by_key = {item["key"]: item for item in items}
    assert by_key["indemnity"]["state"] == "high"
    assert "3 months" in by_key["indemnity"]["finding"]
    assert by_key["renewal"]["state"] == "medium"
    assert "90" in by_key["renewal"]["finding"]
    assert "ninety" in by_key["renewal"]["quote"].lower()
    assert by_key["renewal"]["section"] == "§12.1"
    assert by_key["law"]["state"] == "clear"
    assert by_key["law"]["finding"] == "Delaware"
    assert by_key["breach"]["state"] == "missing"
    assert by_key["breach"]["absent"] is True
    counts = tally(items)
    assert counts["open"] == 3
    assert counts["clear"] == 1


def test_dpa_checklist_is_a_different_matter():
    items = evaluate(_chunks(SAMPLE_DPA, "Northwind_DPA.txt"), {})
    by_key = {item["key"]: item for item in items}
    assert by_key["indemnity"]["state"] == "clear"
    assert by_key["renewal"]["state"] == "clear"
    assert by_key["law"]["state"] == "medium"
    assert by_key["law"]["finding"] == "California"
    assert by_key["breach"]["state"] == "clear"
    assert "three months preceding" not in " ".join(item["quote"].lower() for item in items)
