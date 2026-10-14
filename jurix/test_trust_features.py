"""Numeric firm rules, evidence history, pre-bill review and account isolation."""
from __future__ import annotations

import importlib.util
import io
import csv
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from test_workbench import setup  # noqa: F401 — shared isolated service fixture
from custom_rules import normalize_rules, evaluate_rules, source_anchor
from review import normalize_policy
from billing import parse_entries, audit_entries, normalize_billing_policy, FIELDS
from service import JurixService
from records import Records
from store import FirmStore


def rule(kind="nda_term", **changes):
    return normalize_rules([{
        "id": "nda_max" if kind == "nda_term" else "cap_floor", "name": "Firm rule", "kind": kind,
        "threshold": 24 if kind == "nda_term" else 50000, "preferred_text": "This Agreement expires after two years.",
        **changes,
    }])[0]


def scored(text, kind="nda_term", **changes):
    return evaluate_rules([{"id": "c1", "index": 0, "doc_name": "NDA.txt", "text": text}], [rule(kind, **changes)])[0]


@pytest.mark.parametrize("text,state", [
    ("The term of this Agreement is two years.", "clear"),
    ("This Agreement remains in effect for 24 months.", "clear"),
    ("The term of this Agreement is three years.", "high"),
    ("The initial term of this Agreement is two years and renews automatically.", "medium"),
    ("Confidentiality obligations survive for five years.", "missing"),
    ("The term of the Agreement is perpetual.", "medium"),
    ("The Agreement term is two years. It renews automatically.", "medium"),
    ("The payment term is two months.", "medium"),
    ("The Agreement term is two (3) years.", "medium"),
    ("The Agreement term is twenty four months.", "medium"),
    ("The Agreement term is 1.5 years.", "medium"),
    ("The Agreement term is not two years.", "medium"),
    ("The agreement covers a pilot project.", "missing"),
])
def test_numeric_nda_rule_requires_an_explicit_supported_term(text, state):
    assert scored(text)["state"] == state


@pytest.mark.parametrize("text,state", [
    ("The aggregate liability cap is $25,000.", "high"),
    ("The aggregate liability cap is USD 50,000.", "clear"),
    ("Total liability shall not exceed 60,000 USD.", "clear"),
    ("Liability is limited to INR 50,000.", "medium"),
    ("Liability is capped at fees paid in the previous twelve months.", "medium"),
    ("Liability shall not exceed $50,000 except for indemnities.", "medium"),
    ("Liability shall not exceed $50,000 per claim.", "medium"),
    ("Liability shall not exceed $2 million.", "medium"),
    ("Liability shall not exceed $50,000 CAD.", "medium"),
    ("Liability is not capped at $50,000.", "medium"),
    ("Invoices must be paid up to $25,000.", "missing"),
])
def test_currency_rules_do_not_infer_exchange_rates_or_scope(text, state):
    assert scored(text, "liability_amount")["state"] == state


def test_conflicting_provision_and_scope_are_visible():
    items = evaluate_rules([
        {"id": "c1", "index": 0, "doc_name": "NDA.txt", "text": "The Agreement term is two years."},
        {"id": "c2", "index": 1, "doc_name": "NDA.txt", "text": "For other services, the Agreement term is five years."},
    ], [rule(applies_to="nda")])
    assert items[0]["state"] == "high" and items[0]["chunk_id"] == "c2"
    assert evaluate_rules([{"id": "c1", "index": 0, "doc_name": "MSA.txt", "text": "The Agreement term is three years."}], [rule(applies_to="nda")]) == []
    absent = scored("Services will be provided.")
    assert absent["quote"] == absent["chunk_id"] == ""
    assert source_anchor(absent, [], "Services will be provided.") is None
    assert absent["authority"] is None


@pytest.mark.parametrize("changes", [
    {"threshold": True}, {"threshold": 121}, {"threshold": 0}, {"currency": "JPY"},
    {"kind": "arbitrary_ai_rule"}, {"id": "../bad"}, {"applies_to": "court_order"},
    {"authority_title": "Invalid link", "authority_url": "javascript:alert(1)"},
    {"authority_title": "Invalid link", "authority_url": "https://user:password@example.com"},
    {"authority_url": "https://example.com"},
])
def test_invalid_rule_inputs_are_rejected(changes):
    with pytest.raises(ValueError):
        rule(**changes)


def test_custom_only_policy_and_no_applicable_rules(setup):
    svc, cid = setup
    policy = normalize_policy({"enabled_checks": [], "custom_rules": [rule(applies_to="nda")]})
    svc.set_playbook("alice@firm.test", cid, policy)
    with pytest.raises(ValueError, match="No selected rules apply"):
        svc.audit("alice@firm.test", cid)


def test_exact_anchors_preserve_repeated_and_overlapping_passages():
    original = "Preamble\n\nThe Agreement term is three years.\n\nThe Agreement term is three years."
    chunks = [{"id": "c1", "index": 0, "text": "The Agreement term is three years."}, {"id": "c2", "index": 1, "text": "The Agreement term is three years."}]
    anchor = source_anchor({"chunk_id": "c2", "quote": chunks[1]["text"]}, chunks, original)
    assert anchor["start"] == original.rindex(chunks[1]["text"])
    assert anchor["line_start"] == 5
    assert original[anchor["start"]:anchor["end"]] == chunks[1]["text"]
    assert source_anchor({"chunk_id": "c1", "quote": "Fabricated quotation"}, chunks, original) is None


def test_source_coordinates_count_unicode_characters():
    original = "📄 आदेश\n\nThe Agreement term is three years."
    chunk = {"id": "c1", "index": 0, "text": "The Agreement term is three years."}
    anchor = source_anchor({"chunk_id": "c1", "quote": chunk["text"]}, [chunk], original)
    assert original[anchor["start"]:anchor["end"]] == chunk["text"]
    assert anchor["line_start"] == 3


def test_firm_playbook_versions_are_isolated_copies(setup):
    svc, cid = setup
    policy = normalize_policy({"name": "Firm NDA standard", "enabled_checks": [], "custom_rules": [rule()]})
    first = svc.save_firm_playbook("alice@firm.test", policy)
    second = svc.save_firm_playbook("alice@firm.test", {**policy, "custom_rules": [rule(threshold=12)]})
    assert first["version"] == 1 and second["version"] == 2
    assert first["id"] != second["id"]
    assert svc.firm_playbooks("bob@other.test")["playbooks"] == []
    svc.set_playbook("alice@firm.test", cid, first["policy"])
    assert svc.matter_playbook("alice@firm.test", cid)["custom_rules"][0]["threshold"] == 24


def test_clause_decisions_retain_exact_evidence_and_invalidate_on_changes(setup):
    svc, cid = setup
    original = "NON-DISCLOSURE AGREEMENT\n\n§1 The term of this Agreement is three years."
    doc = svc.ingest("alice@firm.test", cid, "NDA.txt", original)
    policy = normalize_policy({"enabled_checks": [], "custom_rules": [rule(applies_to="nda", authority_title="Firm rule reference", authority_url="https://example.com/firm-policy")]})
    svc.set_playbook("alice@firm.test", cid, policy)
    run = svc.audit("alice@firm.test", cid)
    item = next(d["items"][0] for d in run["document_runs"] if d["doc_id"] == doc["id"])
    assert original[item["anchor"]["start"]:item["anchor"]["end"]] == item["quote"]
    with pytest.raises(ValueError, match="rationale"):
        svc.review_finding("alice@firm.test", cid, item["id"], "open", "", "accepted")
    with pytest.raises(ValueError, match="accept at least one"):
        svc.export_proposals("alice@firm.test", cid)
    svc.review_finding("alice@firm.test", cid, item["id"], "in_review", "Approved wording; integration pending", "accepted")
    body = svc.export_proposals("alice@firm.test", cid)["body"]
    assert "Original passage" in body and item["quote"] in body and "Firm preferred text" in body
    assert svc.read_document("alice@firm.test", cid, doc["id"])["text"] == original
    event = svc.detail("alice@firm.test", cid)["activity"][-1]
    assert event["evidence"]["quote"] == item["quote"] and event["policy"] == policy
    assert event["fingerprint"] == item["fingerprint"] and event["suggestion_decision"] == "accepted"
    svc.audit("alice@firm.test", cid)
    assert svc.detail("alice@firm.test", cid)["document_runs"][-1]["items"][0]["review"]["suggestion_decision"] == "accepted"
    new_policy = {**policy, "custom_rules": [rule(threshold=12, applies_to="nda")]}
    svc.set_playbook("alice@firm.test", cid, new_policy)
    with pytest.raises(ValueError, match="current audit"):
        svc.export_proposals("alice@firm.test", cid)
    rerun = svc.audit("alice@firm.test", cid)
    assert rerun["document_runs"][-1]["items"][0]["review"]["suggestion_decision"] == "pending"
    assert rerun["activity"][-1]["evidence"]["quote"] == item["quote"]


def csv_body(narrative="Reviewed indemnity provisions for supplier negotiation", **changes):
    row = {"date": "2026-10-02", "timekeeper": "Associate", "hours": "0.5", "task_code": "L120", "activity_code": "A104", "narrative": narrative, **changes}
    stream = io.StringIO()
    writer = csv.DictWriter(stream, fieldnames=FIELDS)
    writer.writeheader(); writer.writerow(row)
    return stream.getvalue()


@pytest.mark.parametrize("narrative,changes,code", [
    ("Worked on case", {}, "vague_narrative"),
    ("Reviewed witness statements; drafted a motion response", {}, "possible_block_billing"),
    ("Reviewed witness statement and drafted response to opposing counsel", {}, "possible_block_billing"),
    ("Scheduling client meeting for next week", {"hours": "2"}, "routine_threshold"),
    ("Reviewed lengthy trial record for appeal strategy", {"hours": "9"}, "hours_threshold"),
    ("Reviewed witness statement for case preparation", {"task_code": "L999"}, "unsupported_task_code"),
    ("Reviewed witness statement for case preparation", {"activity_code": "A999"}, "unsupported_activity_code"),
    ("Reviewed witness statement for case preparation", {"task_code": ""}, "missing_task_code"),
])
def test_billing_flags_are_explainable(narrative, changes, code):
    result = audit_entries(parse_entries(csv_body(narrative, **changes)), normalize_billing_policy())
    assert code in [f["code"] for f in result["entries"][0]["flags"]]
    assert result["flagged_entries"] == 1
    assert all(f["next_step"] for f in result["entries"][0]["flags"])


def test_billing_respects_client_guidelines_and_does_not_treat_every_and_as_block_billing():
    policy = normalize_billing_policy({"require_task_code": False, "require_activity_code": False, "routine_terms": []})
    body = csv_body("Reviewed and analyzed the indemnity provision for negotiation", task_code="", activity_code="")
    assert audit_entries(parse_entries(body), policy)["flagged_entries"] == 0
    result = audit_entries(parse_entries(csv_body("Reviewed witness statement and drafted motion response")), normalize_billing_policy({"flag_block_billing": False}))
    assert result["flagged_entries"] == 0
    # csv.reader preserves commas, embedded newlines and UTF-8 narrative text.
    assert parse_entries(csv_body("Reviewed आदेश,\nconfirmed statutory references"))[0]["narrative"] == "Reviewed आदेश,\nconfirmed statutory references"


@pytest.mark.parametrize("changes", [{"hours": "NaN"}, {"hours": "inf"}, {"hours": "0"}, {"hours": "25"}, {"date": "2026-02-30"}, {"timekeeper": ""}, {"narrative": ""}])
def test_malformed_billing_entries_are_not_saved(changes):
    with pytest.raises(ValueError):
        parse_entries(csv_body(**changes))


def test_billing_snapshot_persistence_exports_and_foreign_account_boundaries(setup):
    svc, cid = setup
    run = svc.audit_billing("alice@firm.test", cid, csv_body("Worked on case"), {"name": "Atlas guidelines", "max_entry_hours": 6})
    restored = JurixService(FirmStore(str(svc.store.path)), Records(str(svc.records.path)), svc.bridge)
    assert restored.billing_history("alice@firm.test", cid)["runs"][0]["id"] == run["id"]
    exported = restored.export_billing("alice@firm.test", cid, run["id"])
    assert "Atlas guidelines" in exported["body"] and "Worked on case" in exported["body"]
    assert run["author"] in exported["body"] and "specific task" in exported["body"]
    for call in [
        lambda: svc.audit_billing("bob@other.test", cid, csv_body(), {}),
        lambda: svc.billing_history("bob@other.test", cid),
        lambda: svc.export_billing("bob@other.test", cid, run["id"]),
        lambda: svc.get_export("bob@other.test", exported["export"]["id"]),
    ]:
        with pytest.raises(KeyError): call()


def test_http_exposes_firm_library_billing_and_proposals_with_auth(setup, monkeypatch):
    from fastapi.testclient import TestClient
    svc, cid = setup
    spec = importlib.util.spec_from_file_location("jurix_trust_api", Path(__file__).parent / "app.py")
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "svc", svc)
    headers = {"Authorization": "Bearer " + svc.login("alice@firm.test", "password123")["token"]}
    foreign = {"Authorization": "Bearer " + svc.login("bob@other.test", "password123")["token"]}
    with TestClient(module.app) as http:
        assert http.get("/api/firm/playbooks").status_code == 401
        policy = normalize_policy({"custom_rules": [rule()]})
        assert http.post("/api/firm/playbooks", headers=headers, json=policy).status_code == 200
        assert http.get("/api/firm/playbooks", headers=foreign).json()["playbooks"] == []
        route = f"/api/matters/{cid}/billing/audit"
        assert http.post(route, json={"csv_text": csv_body()}).status_code == 401
        assert http.post(route, headers=foreign, json={"csv_text": csv_body()}).status_code == 404
        assert http.post(route, headers=headers, json={"csv_text": csv_body(), "policy": {"max_entry_hours": True}}).status_code == 422
        assert http.post(route, headers=headers, json={"csv_text": csv_body(hours="NaN")}).status_code == 400
        response = http.post(route, headers=headers, json={"csv_text": csv_body("Worked on case")})
        assert response.status_code == 200 and response.json()["flagged_entries"] == 1
        runid = response.json()["id"]
        assert http.get(f"/api/matters/{cid}/billing", headers=foreign).status_code == 404
        report = http.post(f"/api/matters/{cid}/billing/{runid}/export", headers=headers)
        assert report.status_code == 200
        assert http.get(f"/api/exports/{report.json()['export']['id']}", headers=headers).status_code == 200
        assert http.post(f"/api/matters/{cid}/export-proposals", headers=foreign).status_code == 404
        assert http.post(f"/api/matters/{cid}/export-proposals", headers=headers).status_code == 400
