"""Exercise browser API contracts with isolated encrypted state and fake DBX."""
from __future__ import annotations

import importlib.util
from pathlib import Path
import re
import secrets
import sys

import pytest
from fastapi.testclient import TestClient

sys.path.insert(0, str(Path(__file__).resolve().parent))
from persistence import Database, EncryptedRecords
from security import SecureFirmStore
from service import JurixService
from test_workbench import Bridge
from regulations import RegulatoryWatches
from worker import run_cycle

PASSWORD = "a strong test password"


@pytest.fixture
def api_setup(tmp_path, monkeypatch):
    database = Database(str(tmp_path / "contracts.sqlite3"), secrets.token_bytes(32), secrets.token_bytes(32))
    store = SecureFirmStore(database=database)
    service = JurixService(store, EncryptedRecords(database=database), Bridge())
    service.register("admin@firm.test", PASSWORD, "Contract Test Firm")
    matter = service.create_matter("admin@firm.test", "Acme", "Services agreement")
    service.dbx_status = lambda: {"ok": True}
    spec = importlib.util.spec_from_file_location("jurix_contract_api", Path(__file__).parent / "app.py")
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    module.svc = service
    with TestClient(module.app) as client:
        yield client, service, matter["id"], module
    service.providers.http.close()


def sign_in(client, email="admin@firm.test"):
    client.cookies.clear()
    result = client.post("/api/login", json={"email": email, "password": PASSWORD})
    assert result.status_code == 200
    assert "token" not in result.json()
    assert "HttpOnly" in result.headers.get_list("set-cookie")[0]
    return {"X-CSRF-Token": client.cookies["jurix_csrf"]}


def test_cookie_auth_csrf_logout_readiness_and_error_headers(api_setup, monkeypatch):
    client, service, cid, module = api_setup
    assert client.get("/api/health").json() == {"status": "ok", "product": "jurix"}
    assert client.get("/api/readiness").status_code == 401
    csrf = sign_in(client)
    assert client.get("/api/session").json()["authenticated"]
    assert client.get("/api/readiness").json()["dbx"]["ok"]
    assert client.post(f"/api/matters/{cid}/audit").status_code == 403
    assert client.post(f"/api/matters/{cid}/audit", headers={"Authorization": "Basic invalid"}).status_code == 403
    assert client.post(f"/api/matters/{cid}/audit", headers={**csrf, "Origin": "https://foreign.test"}).status_code == 403
    service.dbx_status = lambda: {"ok": False, "detail": "SECRET_PRIVATE_URL"}
    readiness = client.get("/api/readiness")
    assert not readiness.json()["dbx"]["ok"] and "SECRET" not in readiness.text
    def broken(*args):
        raise RuntimeError("SECRET_PROVIDER_CREDENTIAL")
    monkeypatch.setattr(service, "audit", broken)
    failed = client.post(f"/api/matters/{cid}/audit", headers=csrf)
    assert failed.status_code == 502 and "SECRET" not in failed.text
    assert failed.headers["x-content-type-options"] == "nosniff"
    assert "script-src 'self'" in failed.headers["content-security-policy"]
    token = client.cookies["jurix_session"]
    assert client.post("/api/logout", headers=csrf).status_code == 200
    assert not client.get("/api/session").json()["authenticated"]
    assert service.store.user_for_token(token) is None
    missing = client.get("/api/no-such-endpoint")
    assert missing.status_code == 404 and missing.headers["content-type"].startswith("application/json")


def test_invited_registration_roles_and_partial_membership_updates(api_setup):
    client, service, cid, module = api_setup
    csrf = sign_in(client)
    invitation = client.post("/api/firm/invitations", headers=csrf, json={"email": "viewer@firm.test", "role": "viewer"})
    assert invitation.status_code == 200
    client.cookies.clear()
    registered = client.post("/api/register", json={"email": "viewer@firm.test", "password": PASSWORD, "invite_token": invitation.json()["token"]})
    assert registered.status_code == 200 and registered.json()["user"]["role"] == "viewer"
    viewer_csrf = {"X-CSRF-Token": client.cookies["jurix_csrf"]}
    assert client.get("/api/matters").json()["matters"][0]["id"] == cid
    assert client.post(f"/api/matters/{cid}/audit", headers=viewer_csrf).status_code == 403
    assert client.put(f"/api/matters/{cid}/processing", headers=viewer_csrf, json={"enabled": True}).status_code == 403
    assert client.post("/api/firm/invitations", headers=viewer_csrf, json={"email": "other@firm.test"}).status_code == 403
    csrf = sign_in(client)
    route = "/api/firm/members/viewer@firm.test"
    assert client.patch(route, headers=csrf, json={"active": False}).json()["active"] is False
    changed = client.patch(route, headers=csrf, json={"role": "reviewer"})
    assert changed.status_code == 200 and changed.json()["active"] is False
    assert client.patch("/api/firm/members/admin@firm.test", headers=csrf, json={"active": False}).status_code == 400


def test_document_import_source_version_compare_and_account_boundaries(api_setup):
    client, service, cid, module = api_setup
    csrf = sign_in(client)
    route = f"/api/matters/{cid}/documents"
    source = b"The term of this Agreement is two years."
    uploaded = client.post(route + "/upload", headers=csrf, files={"file": ("agreement.txt", source, "text/plain")})
    assert uploaded.status_code == 200
    draft = uploaded.json()
    extraction = route + "/extractions/" + draft["id"]
    assert "blob_id" not in client.get(extraction).json()
    original = client.get(extraction + "/original")
    assert original.content == source and "attachment" in original.headers["content-disposition"]
    committed = client.post(extraction + "/commit", headers=csrf, json={"doc_name": "agreement.txt", "text": draft["text"]})
    assert committed.status_code == 200
    first = committed.json()["id"]
    assert client.post(extraction + "/commit", headers=csrf, json={"doc_name": "agreement.txt", "text": draft["text"]}).status_code == 400
    revised = client.post(route, headers=csrf, json={"doc_name": "agreement v2.txt", "text": "The term of this Agreement is three years.", "supersedes": first})
    assert revised.status_code == 200
    second = revised.json()["id"]
    matter = client.get(f"/api/matters/{cid}").json()
    assert matter["documents"][-1]["version"] == 2
    assert matter["documents"][-1]["supersedes"] == first
    assert client.get(route + f"/{second}/compare", params={"other_id": first}).json()["changes"]
    assert client.post(route, headers=csrf, json={"doc_name": "v3.txt", "text": "A version", "supersedes": first}).status_code == 400
    assert client.post(route + f"/{second}/redline", headers=csrf).status_code == 400
    service.register("other@firm.test", PASSWORD, "Other Firm")
    sign_in(client, "other@firm.test")
    assert client.get(extraction).status_code == 404
    assert client.get(extraction + "/original").status_code == 404
    assert client.get(route + f"/{second}/compare", params={"other_id": first}).status_code == 404


def test_capabilities_and_external_processing_are_truthful(api_setup, monkeypatch):
    client, service, cid, module = api_setup
    csrf = sign_in(client)
    monkeypatch.setattr(service.providers, "capabilities", lambda: {key: {"available": False} for key in ("llm", "ocr", "translation", "dictation")})
    assert not any(row["available"] for row in client.get("/api/capabilities").json().values())
    doc = service.ingest("admin@firm.test", cid, "source.txt", "The Agreement term is two years.")
    route = f"/api/matters/{cid}/documents/{doc['id']}/translate"
    assert client.post(route, headers=csrf, json={"target_language": "French"}).status_code == 403
    assert client.put(f"/api/matters/{cid}/processing", headers=csrf, json={"enabled": True}).status_code == 200
    assert client.post(route, headers=csrf, json={"target_language": "French"}).status_code == 503
    policy = service.matter_playbook("admin@firm.test", cid)
    policy["custom_rules"] = [{"id": "ai", "name": "AI rule", "kind": "semantic", "instructions": "Check obligations", "threshold": 1}]
    assert client.put(f"/api/matters/{cid}/playbook", headers=csrf, json=policy).status_code == 200


def test_regulatory_unknown_dates_changes_and_scheduled_worker(api_setup, monkeypatch):
    client, service, cid, module = api_setup
    csrf = sign_in(client)
    service.ingest("admin@firm.test", cid, "privacy.txt", "The contract requires confidentiality and privacy obligations.")
    service.audit("admin@firm.test", cid)
    current = ["privacy baseline"]
    import hashlib
    import time
    def publication(url):
        return {"text": current[0], "sha256": hashlib.sha256(current[0].encode()).hexdigest(), "url": url, "fetched_at": time.time()}
    watches = RegulatoryWatches(service.database, service.store, service.records, publication)
    monkeypatch.setattr(service, "regulatory_watches", lambda: watches)
    body = {"name": "Privacy legislation", "source_url": "https://www.legislation.gov.uk/ukpga/2018/12", "jurisdiction": "UK", "effective_date": None, "keywords": ["privacy"]}
    created = client.post("/api/regulations", headers=csrf, json=body)
    assert created.status_code == 200 and created.json()["effective_date"] is None
    wid = created.json()["id"]
    assert client.post(f"/api/regulations/{wid}/check", headers=csrf).status_code == 200
    current[0] = "privacy revised publication"
    run_cycle(service, interval=0)
    state = client.get("/api/regulations").json()
    assert len(state["watches"][0]["versions"]) == 2
    assert state["alerts"][0]["matter_id"] == cid
    alert = "/api/regulations/alerts/" + state["alerts"][0]["id"]
    assert client.patch(alert, headers=csrf, json={"status": "dismissed", "note": ""}).status_code == 400
    assert client.patch(alert, headers=csrf, json={"status": "reviewed", "note": "Counsel checked applicability"}).status_code == 200
    assert client.post("/api/regulations", headers=csrf, json={**body, "effective_date": "not-a-date"}).status_code == 400
    assert client.post("/api/regulations", headers=csrf, json={**body, "keywords": []}).status_code == 422


def test_every_frontend_api_path_has_a_backend_route(api_setup):
    client, service, cid, module = api_setup
    def normalize(path):
        path = path.split("?", 1)[0]
        path = re.sub(r"\$\{[^}]+\}", "{}", path)
        return re.sub(r"\{[^}]*\}", "{}", path)
    exposed = {normalize(route.path) for route in module.app.routes if route.path.startswith("/api/") and route.path != "/api/{path:path}"}
    used = set()
    for source in (Path(__file__).parent / "frontend" / "src").rglob("*.ts*"):
        for match in re.finditer(r"['\"`](/api/[^'\"`]+)['\"`]", source.read_text(encoding="utf-8")):
            used.add(normalize(match[1]))
    assert len(used) >= 25
    assert not used - exposed, f"Frontend paths without API routes: {sorted(used - exposed)}"


def test_translation_and_dictation_drafts_require_consent_and_preserve_source(api_setup, monkeypatch):
    client, service, cid, module = api_setup
    csrf = sign_in(client)
    doc = service.ingest("admin@firm.test", cid, "source.txt", "The term is two years.")
    monkeypatch.setattr(service.providers, "capabilities", lambda: {key: {"available": True} for key in ("llm", "ocr", "translation", "dictation")})
    monkeypatch.setattr(service.providers, "translate", lambda text, target, glossary: {"text": "Le terme est de deux ans.", "warnings": []})
    monkeypatch.setattr(service.providers, "transcribe", lambda *args: {"transcript": "Check the term", "structured_note": "Verify the contract term.", "warnings": []})
    assert client.put(f"/api/matters/{cid}/processing", headers=csrf, json={"enabled": True}).status_code == 200
    draft = client.post(f"/api/matters/{cid}/documents/{doc['id']}/translate", headers=csrf, json={"target_language": "French", "glossary": []})
    assert draft.status_code == 200 and draft.json()["status"] == "needs_review"
    assert draft.json()["translated_from"] == doc["id"] and draft.json()["has_original"] is False
    assert service.records.documents(cid)[0]["text"] == "The term is two years."
    assert len(service.records.documents(cid)) == 1
    audio = {"file": ("note.webm", b"test audio", "audio/webm")}
    note = client.post(f"/api/matters/{cid}/dictation", headers=csrf, files=audio, data={"lexicon": "Firm vocabulary", "language": "en"})
    assert note.status_code == 200 and note.json()["transcript"] == "Check the term"
    assert not service.database.list_objects(service.owner("admin@firm.test"), "archive")
    assert client.put(f"/api/matters/{cid}/processing", headers=csrf, json={"enabled": False}).status_code == 200
    assert client.post(f"/api/matters/{cid}/dictation", headers=csrf, files=audio).status_code == 403


def test_word_export_contains_only_accepted_anchored_changes(api_setup):
    import io
    from zipfile import ZipFile
    from review import normalize_policy
    client, service, cid, module = api_setup
    csrf = sign_in(client)
    source = "The term of this Agreement is three years."
    doc = service.ingest("admin@firm.test", cid, "NDA.txt", source)
    policy = normalize_policy({"enabled_checks": [], "custom_rules": [{"id": "term", "name": "Maximum NDA term", "kind": "nda_term", "threshold": 24, "preferred_text": "The Agreement expires after two years."}]})
    service.set_playbook("admin@firm.test", cid, policy)
    audit = service.audit("admin@firm.test", cid)
    finding = audit["document_runs"][0]["items"][0]
    service.review_finding("admin@firm.test", cid, finding["id"], "resolved", "Approved replacement wording", "accepted")
    result = client.post(f"/api/matters/{cid}/documents/{doc['id']}/redline", headers=csrf)
    assert result.status_code == 200 and result.headers["content-type"].startswith("application/vnd.openxmlformats")
    with ZipFile(io.BytesIO(result.content)) as archive:
        changes = archive.read("word/document.xml").decode()
    assert "<w:del " in changes and "<w:ins " in changes
    assert source in changes and "expires after two years" in changes
    assert service.records.documents(cid)[0]["text"] == source
