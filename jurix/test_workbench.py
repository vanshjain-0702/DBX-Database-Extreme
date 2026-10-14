"""Evidence, review persistence and account boundaries without a live DBX node."""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from service import JurixService
from records import Records
from store import FirmStore
from review import normalize_policy
from playbook import evaluate, score_breach, score_law
from samples import SAMPLE_DPA, SAMPLE_MSA


class Memory:
    def __init__(self):
        self.rows = {}
        self.client = self

    def remember(self, key, text, vector=None):
        self.rows[key] = text

    def set(self, key, text):
        pass

    def get(self, key):
        return self.rows.get(key)

    def recall(self, vector, top_k=4):
        return []


class Bridge:
    def __init__(self):
        self.memories = {}
        self.provisioned_names = []

    def provision_client(self, name):
        self.provisioned_names.append(name)
        tenant = f"tenant-{len(self.memories)}"
        self.memories[tenant] = Memory()
        return {"tenant_id": tenant, "key_id": "test", "secret": "test"}

    def memory_for(self, tenant, key, secret):
        return self.memories[tenant]


@pytest.fixture
def setup(tmp_path):
    svc = JurixService(FirmStore(str(tmp_path / "firm.json")), Records(str(tmp_path / "records.json")), Bridge())
    svc.register("alice@firm.test", "password123", "Test Firm")
    svc.register("bob@other.test", "password123", "Other Firm")
    matter = svc.create_matter("alice@firm.test", "Atlas", "Contract review")
    svc.ingest("alice@firm.test", matter["id"], "Commercial.txt", SAMPLE_MSA)
    return svc, matter["id"]


def test_document_matrix_does_not_hide_missing_clauses(setup):
    svc, cid = setup
    svc.ingest("alice@firm.test", cid, "Privacy.txt", SAMPLE_DPA)
    result = svc.audit("alice@firm.test", cid)
    assert result["review_total"] == 16
    assert result["reviewed"] == 0
    rows = result["document_runs"]
    first = {item["key"]: item for item in rows[0]["items"]}
    second = {item["key"]: item for item in rows[1]["items"]}
    assert first["breach"]["state"] == "missing"
    assert second["breach"]["state"] == "clear"
    assert first["breach"]["quote"] == first["breach"]["chunk_id"] == ""
    assert first["breach"]["id"] != second["breach"]["id"]
    assert next(i for i in result["items"] if i["key"] == "breach")["state"] == "missing"
    for row in rows:
        chunks = svc.read_document("alice@firm.test", cid, row["doc_id"])["chunks"]
        for item in row["items"]:
            if item["quote"]:
                source = next(chunk for chunk in chunks if chunk["id"] == item["chunk_id"])
                assert item["quote"] in source["text"]


def test_policy_thresholds_change_results_and_stale_existing_run(setup):
    svc, cid = setup
    svc.audit("alice@firm.test", cid)
    policy = normalize_policy({"liability_floor_months": 3, "renewal_notice_days": 90, "accepted_laws": ["California"], "enabled_checks": ["indemnity", "renewal", "law"]})
    assert svc.set_playbook("alice@firm.test", cid, policy)["stale"] is True
    result = svc.audit("alice@firm.test", cid)
    assert result["stale"] is False
    states = {i["key"]: i["state"] for i in result["items"]}
    assert states == {"indemnity": "clear", "renewal": "clear", "law": "medium"}
    assert result["review_total"] == 3


def test_reviewer_decision_persists_on_same_evidence_and_is_exported(setup):
    svc, cid = setup
    result = svc.audit("alice@firm.test", cid)
    item = result["document_runs"][0]["items"][0]
    svc.review_finding("alice@firm.test", cid, item["id"], "accepted_risk", "Approved exception for this transaction.")
    rerun = svc.audit("alice@firm.test", cid)
    assert rerun["reviewed"] == 1
    assert rerun["document_runs"][0]["items"][0]["review"]["status"] == "accepted_risk"
    restored = JurixService(FirmStore(str(svc.store.path)), Records(str(svc.records.path)), svc.bridge)
    assert restored.detail("alice@firm.test", cid)["reviewed"] == 1
    memo = restored.export_memo("alice@firm.test", cid)["body"]
    assert "accepted_risk" in memo and "Approved exception" in memo
    assert "Decision history" in memo and "first-pass text checks" in memo


def test_policy_change_invalidates_prior_decision_but_preserves_history(setup):
    svc, cid = setup
    row = svc.audit("alice@firm.test", cid)["document_runs"][0]["items"][0]
    svc.review_finding("alice@firm.test", cid, row["id"], "resolved", "Counterparty confirmed scope.")
    svc.set_playbook("alice@firm.test", cid, normalize_policy({"liability_floor_months": 6}))
    with pytest.raises(ValueError, match="run the audit again"):
        svc.review_finding("alice@firm.test", cid, row["id"], "resolved", "Old result")
    result = svc.audit("alice@firm.test", cid)
    assert result["reviewed"] == 0
    assert len(result["activity"]) == 1


def test_new_document_requires_rerun_and_rationale_is_required(setup):
    svc, cid = setup
    row = svc.audit("alice@firm.test", cid)["document_runs"][0]["items"][0]
    with pytest.raises(ValueError, match="rationale"):
        svc.review_finding("alice@firm.test", cid, row["id"], "resolved", " ")
    svc.ingest("alice@firm.test", cid, "Addendum.txt", "Changes to services.")
    with pytest.raises(ValueError, match="run the audit again"):
        svc.review_finding("alice@firm.test", cid, row["id"], "in_review", "Checking")
    assert "STALE" in svc.memorandum("alice@firm.test", cid)


def test_other_account_cannot_change_reviews_policy_notes_or_read_exports(setup):
    svc, cid = setup
    row = svc.audit("alice@firm.test", cid)["document_runs"][0]["items"][0]
    exported = svc.export_memo("alice@firm.test", cid)["export"]["id"]
    calls = [
        lambda: svc.set_playbook("bob@other.test", cid, normalize_policy()),
        lambda: svc.review_finding("bob@other.test", cid, row["id"], "resolved", "Done"),
        lambda: svc.add_note("bob@other.test", cid, row["id"], "Note"),
        lambda: svc.get_export("bob@other.test", exported),
        lambda: svc.read_document("bob@other.test", cid),
    ]
    for call in calls:
        with pytest.raises(KeyError):
            call()


def test_same_client_name_gets_distinct_provisioning_identity(setup):
    svc, cid = setup
    svc.create_matter("bob@other.test", "Atlas", "Other contract review")
    assert svc.bridge.provisioned_names[0] != svc.bridge.provisioned_names[1]
    assert all(len(name) <= 37 for name in svc.bridge.provisioned_names)


def test_document_added_during_audit_is_marked_stale(setup, monkeypatch):
    svc, cid = setup
    original = svc._recall
    added = False

    def recall(client, query, top_k=4):
        nonlocal added
        if not added:
            added = True
            svc.ingest("alice@firm.test", cid, "During audit.txt", "An additional document.")
        return original(client, query, top_k)

    monkeypatch.setattr(svc, "_recall", recall)
    result = svc.audit("alice@firm.test", cid)
    assert result["stale"] is True
    assert len(result["document_runs"]) == 1
    item = result["document_runs"][0]["items"][0]
    with pytest.raises(ValueError, match="run the audit again"):
        svc.review_finding("alice@firm.test", cid, item["id"], "in_review", "")


def test_api_enforces_auth_validation_and_reviewer_workflow(setup, monkeypatch):
    import importlib.util
    from fastapi.testclient import TestClient

    svc, cid = setup
    spec = importlib.util.spec_from_file_location("jurix_api_under_test", Path(__file__).parent / "app.py")
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "svc", svc)
    token = svc.login("alice@firm.test", "password123")["token"]
    other = svc.login("bob@other.test", "password123")["token"]
    headers = {"Authorization": "Bearer " + token}
    foreign = {"Authorization": "Bearer " + other}
    with TestClient(module.app) as http:
        assert http.get(f"/api/matters/{cid}/playbook").status_code == 401
        assert http.get(f"/api/matters/{cid}/playbook", headers=foreign).status_code == 404
        policy = normalize_policy()
        policy["liability_floor_months"] = True
        assert http.put(f"/api/matters/{cid}/playbook", headers=headers, json=policy).status_code == 422
        run = http.post(f"/api/matters/{cid}/audit", headers=headers)
        assert run.status_code == 200
        item = run.json()["document_runs"][0]["items"][0]
        route = f"/api/matters/{cid}/findings/{item['id']}/review"
        assert http.patch(route, headers=headers, json={"status": "resolved", "note": ""}).status_code == 400
        assert http.patch(route, headers=foreign, json={"status": "resolved", "note": "Checked"}).status_code == 404
        decision = http.patch(route, headers=headers, json={"status": "accepted_risk", "note": "Approved exception"})
        assert decision.status_code == 200 and decision.json()["reviewed"] == 1
        exported = http.post(f"/api/matters/{cid}/export", headers=headers).json()
        download = http.get(f"/api/exports/{exported['export']['id']}", headers=headers)
        assert download.status_code == 200 and "Approved exception" in download.text
        assert "attachment" in download.headers["content-disposition"]


def test_unrelated_deadline_or_jurisdiction_does_not_clear_check():
    assert score_breach("Invoices must be paid within 24 hours. Security incidents must be notified within 120 hours.")["state"] == "medium"
    assert score_law("Supplier is based in Delaware. This Agreement is governed by the laws of California.")["state"] == "medium"
    assert score_law("This Agreement is governed by the laws of New York.", ["New York"])["state"] == "clear"


def test_retrieved_clear_clause_does_not_hide_a_conflicting_passage():
    chunks = [
        {"id": "clear", "index": 0, "text": "Supplier aggregate liability is capped at fees paid in the twelve months preceding the claim.", "doc_name": "terms.txt"},
        {"id": "risk", "index": 1, "text": "For service interruption, liability is capped at fees paid in the three months preceding the claim.", "doc_name": "terms.txt"},
    ]
    result = evaluate(chunks, {"indemnity": [{"id": "clear"}]}, normalize_policy({"enabled_checks": ["indemnity"]}))[0]
    assert result["state"] == "high"
    assert result["chunk_id"] == "risk"


def test_new_presence_checks_always_require_counsel_review():
    chunks = [{"id": "1", "index": 0, "text": "Confidential information shall not be disclosed. A party may terminate for breach. Assignment needs consent. Delete customer data after termination.", "doc_name": "terms.txt"}]
    result = evaluate(chunks, {}, normalize_policy({"enabled_checks": ["confidentiality", "termination", "assignment", "data_return"]}))
    assert all(item["state"] == "medium" for item in result)


def test_health_checks_current_connectivity_even_with_cached_login(setup, monkeypatch):
    from types import SimpleNamespace
    svc, cid = setup
    svc.bridge.control_url = "http://127.0.0.1:8000"
    svc.bridge.resp_host = "127.0.0.1"
    svc.bridge.resp_port = 6380
    svc.bridge.plane = SimpleNamespace(token="test-token")
    svc.bridge._logged_in = True
    svc.bridge.ensure_login = lambda: None

    def unavailable(*args, **kwargs):
        raise OSError("node unavailable")

    monkeypatch.setattr("service.urlopen", unavailable)
    assert svc.dbx_status()["ok"] is False
    assert svc.bridge._logged_in is False


@pytest.mark.parametrize("policy", [
    {"liability_floor_months": True}, {"renewal_notice_days": 0},
    {"accepted_laws": [""]}, {"enabled_checks": ["unsupported"]}, {"enabled_checks": []},
])
def test_invalid_policies_are_rejected(policy):
    with pytest.raises(ValueError):
        normalize_policy(policy)
