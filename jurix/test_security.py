"""Production persistence tests use isolated databases, clocks and injected keys."""
from __future__ import annotations

import base64
import hashlib
import json
import os
import secrets
import sqlite3
import subprocess
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from persistence import Database, EncryptedRecords
from security import SecureFirmStore
from migrate import migrate
from review import normalize_policy

PASSWORD = "correct horse battery staple"


@pytest.fixture
def setup_secure(tmp_path, monkeypatch):
    monkeypatch.setenv("JURIX_ENV", "development")
    now = [1000.0]
    database = Database(str(tmp_path / "secure.sqlite3"), secrets.token_bytes(32), secrets.token_bytes(32), lambda: now[0])
    store = SecureFirmStore(database=database, clock=lambda: now[0], session_ttl=60, invitation_ttl=120)
    return database, store, EncryptedRecords(database=database), now


def _register(store, email="admin@firm.test", firm="Example Firm"):
    return store.register(email, PASSWORD, firm)


def test_sessions_expire_revoke_and_never_store_bearer_tokens(setup_secure):
    database, store, records, now = setup_secure
    _register(store)
    token = store.login("admin@firm.test", PASSWORD)
    user = store.user_for_token(token)
    assert user["role"] == "admin"
    assert "password_hash" not in user
    with database.connection() as conn:
        assert conn.execute("SELECT token_hash FROM sessions").fetchone()[0] == hashlib.sha256(token.encode()).hexdigest()
    now[0] += 60
    assert store.user_for_token(token) is None
    token = store.login("admin@firm.test", PASSWORD)
    assert store.revoke_token(token)
    assert not store.revoke_token(token)
    assert store.user_for_token(token) is None
    token = store.login("admin@firm.test", PASSWORD)
    assert store.revoke_all_sessions("admin@firm.test") >= 1
    assert store.user_for_token(token) is None
    assert token.encode() not in database.path.read_bytes()


def test_wrong_firm_code_never_creates_session(setup_secure):
    database, store, _, _ = setup_secure
    _register(store)
    store.set_firm_code("admin@firm.test", "AB-1234")
    with pytest.raises(ValueError, match="firm code"):
        store.login("admin@firm.test", PASSWORD, "WRONG")
    with database.connection() as conn:
        assert conn.execute("SELECT COUNT(*) FROM sessions").fetchone()[0] == 0
    assert store.user_for_token(store.login("admin@firm.test", PASSWORD, "ab-1234"))


def test_login_throttle_survives_reconstruction_and_expires(setup_secure):
    database, store, _, now = setup_secure
    _register(store)
    for _ in range(10):
        with pytest.raises(ValueError, match="invalid credentials"):
            store.login("admin@firm.test", "wrong password")
    reloaded = SecureFirmStore(database=database, clock=lambda: now[0])
    with pytest.raises(ValueError, match="too many failed"):
        reloaded.login("admin@firm.test", PASSWORD)
    with database.connection() as conn:
        assert conn.execute("SELECT COUNT(*) FROM sessions").fetchone()[0] == 0
    now[0] += 900
    assert reloaded.user_for_token(reloaded.login("admin@firm.test", PASSWORD))


def test_password_change_revokes_existing_sessions(setup_secure):
    _, store, _, _ = setup_secure
    _register(store)
    token = store.login("admin@firm.test", PASSWORD)
    store.change_password("admin@firm.test", PASSWORD, "a brand new long password")
    assert store.user_for_token(token) is None
    with pytest.raises(ValueError, match="invalid credentials"):
        store.login("admin@firm.test", PASSWORD)
    assert store.user_for_token(store.login("admin@firm.test", "a brand new long password"))


def test_shared_firm_and_role_enforcement_with_last_admin_protection(setup_secure):
    _, store, _, _ = setup_secure
    _register(store)
    _register(store, "colleague@firm.test", "Personal firm")
    personal_token = store.login("colleague@firm.test", PASSWORD)
    invite = store.invite_member("admin@firm.test", "colleague@firm.test", "viewer")
    store.accept_invite("colleague@firm.test", invite["token"])
    assert store.user_for_token(personal_token) is None
    assert store.workspace_owner("admin@firm.test") == store.workspace_owner("colleague@firm.test")
    client = store.add_client("admin@firm.test", "Acme", "tenant-1", "key-1", "private provider key")
    assert store.list_clients("colleague@firm.test") == [client]
    assert store.get_client("colleague@firm.test", client["id"])["secret"] == "private provider key"
    with pytest.raises(PermissionError):
        store.add_client("colleague@firm.test", "Bad", "tenant-2", "key-2", "secret")
    with pytest.raises(PermissionError):
        store.invite_member("colleague@firm.test", "outside@firm.test")
    with pytest.raises(ValueError, match="active admin"):
        store.set_role("admin@firm.test", "admin@firm.test", "viewer")
    store.set_role("admin@firm.test", "colleague@firm.test", "admin")
    token = store.login("colleague@firm.test", PASSWORD)
    store.set_role("admin@firm.test", "colleague@firm.test", "admin", active=False)
    assert store.user_for_token(token) is None
    assert store.public_user("colleague@firm.test") is None
    assert store.list_members("admin@firm.test")["members"][1]["active"] is False


def test_invites_expire_are_scoped_and_single_use(setup_secure):
    _, store, _, now = setup_secure
    _register(store)
    _register(store, "member@firm.test")
    _register(store, "intruder@firm.test")
    invite = store.invite_member("admin@firm.test", "member@firm.test", "reviewer")
    with pytest.raises(ValueError):
        store.accept_invite("intruder@firm.test", invite["token"])
    now[0] += 120
    with pytest.raises(ValueError, match="expired"):
        store.accept_invite("member@firm.test", invite["token"])
    invite = store.invite_member("admin@firm.test", "member@firm.test")
    assert store.revoke_invite("admin@firm.test", invite["token"])
    with pytest.raises(ValueError):
        store.accept_invite("member@firm.test", invite["token"])
    invite = store.invite_member("admin@firm.test", "member@firm.test")
    store.accept_invite("member@firm.test", invite["token"])
    with pytest.raises(ValueError):
        store.accept_invite("member@firm.test", invite["token"])


def test_joining_cannot_orphan_matters(setup_secure):
    _, store, _, _ = setup_secure
    _register(store)
    _register(store, "owner@other.test")
    store.add_client("owner@other.test", "Matter", "tenant", "key", "secret")
    invite = store.invite_member("admin@firm.test", "owner@other.test")
    before = store.workspace_owner("owner@other.test")
    with pytest.raises(ValueError, match="offboard"):
        store.accept_invite("owner@other.test", invite["token"])
    assert store.workspace_owner("owner@other.test") == before


def test_closed_registration_allows_only_first_admin_or_invited_accounts(setup_secure, monkeypatch):
    database, store, _, _ = setup_secure
    monkeypatch.setenv("JURIX_ENV", "production")
    monkeypatch.setenv("JURIX_REGISTRATION_OPEN", "false")
    _register(store)
    with pytest.raises(PermissionError, match="registration is closed"):
        _register(store, "other@firm.test")
    invite = store.invite_member("admin@firm.test", "member@firm.test", "reviewer")
    member = store.register("member@firm.test", PASSWORD, "Ignored firm", invite_token=invite["token"])
    assert member["role"] == "reviewer"
    assert member["firm_id"] == store.public_user("admin@firm.test")["firm_id"]
    assert member["firm_name"] == "Example Firm"
    with database.connection() as conn:
        assert conn.execute("SELECT COUNT(*) FROM firms").fetchone()[0] == 1
    assert store.account_count() == 2
    assert store.firm_admin_emails()[0]["owner"] == store.workspace_owner("admin@firm.test")


def test_firm_isolation_public_secrets_and_encrypted_at_rest(setup_secure):
    database, store, records, _ = setup_secure
    _register(store)
    _register(store, "other@other.test")
    secret = "uniquely-sensitive-provider-secret-348f"
    original = "privileged-original-document-92f08abdd827"
    client = store.add_client("admin@firm.test", "Sensitive Client", "tenant", "key", secret)
    assert store.get_client("other@other.test", client["id"]) is None
    assert store.list_clients("other@other.test") == []
    assert "secret" not in client and "key_id" not in client
    records.init_matter(client["id"], "Privileged matter", "admin@firm.test")
    doc = records.add_document(client["id"], "NDA.txt", original, ["chunk1"])
    assert records.documents(client["id"])[0]["id"] == doc["id"]
    owner = store.workspace_owner("admin@firm.test")
    database.put_object(owner, "watch", "one", {"title": original})
    assert database.get_object(store.workspace_owner("other@other.test"), "watch", "one") is None
    blob_id = database.save_blob(owner, "NDA.pdf", original.encode(), "application/pdf")
    assert database.open_blob(owner, blob_id)["data"] == original.encode()
    with pytest.raises(KeyError):
        database.open_blob(store.workspace_owner("other@other.test"), blob_id)
    with pytest.raises(ValueError, match="20 MB"):
        database.save_blob(owner, "large.pdf", b"0" * (20 * 1024 * 1024 + 1))
    with database.connection() as conn:
        conn.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    for file in database.path.parent.glob("secure.sqlite3*"):
        data = file.read_bytes()
        assert secret.encode() not in data and original.encode() not in data and b"admin@firm.test" not in data


def _decision(records, client_id="matter1"):
    records.init_matter(client_id, "Matter", "admin@firm.test")
    records.add_document(client_id, "doc.txt", "source", ["c1"])
    doc_ids = [doc["id"] for doc in records.documents(client_id)]
    run = records.save_run(client_id, {"policy": normalize_policy(), "document_snapshot": doc_ids})
    records.set_review(client_id, {"id": "finding1", "fingerprint": "abc", "quote": "source"}, "accepted", "Evidence checked", "admin@firm.test", run["ran_at"])


def test_audit_chain_retains_offboarded_evidence_and_rejects_changes(setup_secure):
    database, _, records, _ = setup_secure
    _decision(records)
    assert records.verify_audit()["events"] == 1
    history = records.activity("matter1")
    records.drop_matter("matter1")
    assert records.activity("matter1") == history
    assert records.verify_audit()["valid"]
    with database.connection() as conn:
        with pytest.raises(sqlite3.IntegrityError, match="append-only"):
            conn.execute("DELETE FROM activity_evidence")
    with records._lock:
        records._load()
        records._data["activity"]["matter1"][0]["note"] = "altered"
        with pytest.raises(ValueError, match="rewritten"):
            records._save()


def test_audit_and_authenticated_ciphertext_detect_database_tamper(setup_secure):
    database, _, records, _ = setup_secure
    _decision(records)
    with database.connection() as conn:
        conn.execute("DROP TRIGGER activity_no_update")
        conn.execute("UPDATE activity_evidence SET mac=?", ("f" * 64,))
    with pytest.raises(ValueError, match="chain failed"):
        records.activity("matter1")
    with database.connection() as conn:
        row = conn.execute("SELECT payload FROM objects WHERE owner='_system'").fetchone()
        payload = bytearray(row[0])
        payload[-1] ^= 1
        conn.execute("UPDATE objects SET payload=? WHERE owner='_system'", (bytes(payload),))
    with pytest.raises(ValueError, match="authentication"):
        records.documents("matter1")


def test_cross_process_updates_do_not_lose_notes(setup_secure):
    database, _, records, _ = setup_secure
    records.init_matter("parallel", "Matter", "reviewer")
    code = """import sys,base64
sys.path.insert(0,sys.argv[1])
from persistence import Database,EncryptedRecords
db=Database(sys.argv[2],base64.b64decode(sys.argv[3]),base64.b64decode(sys.argv[4]))
r=EncryptedRecords(database=db)
for i in range(15): r.add_note('parallel','finding',sys.argv[5]+'-'+str(i),'reviewer')
"""
    args = [str(Path(__file__).resolve().parent), str(database.path), base64.b64encode(database.encryption_key).decode(), base64.b64encode(database.audit_key).decode()]
    processes = [subprocess.Popen([sys.executable, "-c", code, *args, str(i)], stdout=subprocess.PIPE, stderr=subprocess.PIPE) for i in range(3)]
    for process in processes:
        stdout, stderr = process.communicate(timeout=90)
        assert process.returncode == 0, stderr.decode()
    notes = records.notes("parallel")
    assert len(notes) == 45
    assert len({note["text"] for note in notes}) == 45


def _legacy_sources(tmp_path):
    salt = "a" * 32
    password_hash = salt + "$" + hashlib.pbkdf2_hmac("sha256", PASSWORD.encode(), salt.encode(), 120000).hex()
    identity = {"users": {"old@firm.test": {"email": "old@firm.test", "firm_name": "Old Firm", "password_hash": password_hash, "created_at": 10}},
                "sessions": {"legacy-bearer-token": {"email": "old@firm.test", "created_at": 11}},
                "clients": {"old-matter": {"id": "old-matter", "owner_email": "old@firm.test", "name": "Acme", "tenant_id": "tenant-original", "key_id": "provider-key", "secret": "provider-secret", "created_at": 12}}}
    records = EncryptedRecords.empty_state()
    records.update({"firms": {"old@firm.test": {"email": "old@firm.test", "firm_name": "Old Firm", "code": "OF-1234"}},
                    "matters": {"old-matter": {"matter": "Migration", "created_at": 12}},
                    "documents": {"old-matter": [{"id": "old-doc", "name": "old.txt", "text": "old privileged source", "chunk_ids": ["old-chunk"], "chunk_count": 1, "ingested_at": 13}]},
                    "runs": {"old-matter": {"id": "old-run", "ran_at": 14}},
                    "activity": {"old-matter": [{"id": "old-event", "note": "retained evidence"}]},
                    "exports": [{"id": "old-export", "owner": "old@firm.test", "body": "confidential memo", "created_at": 15}]})
    identity_path = tmp_path / "legacy_identity.json"
    records_path = tmp_path / "legacy_records.json"
    identity_path.write_text(json.dumps(identity), encoding="utf-8")
    records_path.write_text(json.dumps(records), encoding="utf-8")
    return identity_path, records_path


def test_migration_dry_run_idempotency_ids_credentials_and_revoked_sessions(setup_secure, tmp_path):
    database, store, records, _ = setup_secure
    identity_path, records_path = _legacy_sources(tmp_path)
    original_sources = [identity_path.read_bytes(), records_path.read_bytes()]
    report = migrate(str(identity_path), str(records_path), dry_run=True)
    assert report["valid"] and report["legacy_sessions_revoked"] == 1
    assert store.account_count() == 0
    result = migrate(str(identity_path), str(records_path), database)
    assert Path(result["backup"]).exists()
    assert records.documents("old-matter")[0]["id"] == "old-doc"
    assert records.run("old-matter")["id"] == "old-run"
    assert store.get_client("old@firm.test", "old-matter")["tenant_id"] == "tenant-original"
    assert records.exports(store.workspace_owner("old@firm.test"))[0]["id"] == "old-export"
    assert store.user_for_token("legacy-bearer-token") is None
    assert records.verify_audit()["events"] == 1
    token = store.login("old@firm.test", PASSWORD, "OF-1234")
    assert store.user_for_token(token)["firm_name"] == "Old Firm"
    with database.connection() as conn:
        uid = database.identity_id("old@firm.test")
        payload = conn.execute("SELECT payload FROM users WHERE user_id=?", (uid,)).fetchone()[0]
        assert database.decrypt(payload, "user:" + uid)["password_hash"].startswith("pbkdf2_sha256$600000$")
    assert migrate(str(identity_path), str(records_path), database)["already_applied"]
    assert [identity_path.read_bytes(), records_path.read_bytes()] == original_sources


def test_migration_validation_leaves_target_untouched(setup_secure, tmp_path):
    database, store, _, _ = setup_secure
    identity_path, records_path = _legacy_sources(tmp_path)
    data = json.loads(identity_path.read_text())
    data["clients"]["old-matter"]["owner_email"] = "missing@firm.test"
    identity_path.write_text(json.dumps(data))
    with pytest.raises(ValueError, match="owner has no account"):
        migrate(str(identity_path), str(records_path), database)
    assert store.account_count() == 0


def test_production_requires_independent_secure_keys_and_database_path(tmp_path, monkeypatch):
    monkeypatch.setenv("JURIX_ENV", "production")
    for name in ("JURIX_ENCRYPTION_KEY", "JURIX_AUDIT_KEY", "JURIX_DATABASE_PATH"):
        monkeypatch.delenv(name, raising=False)
    with pytest.raises(ValueError, match="DATABASE_PATH"):
        Database()
    with pytest.raises(ValueError, match="required in production"):
        Database(str(tmp_path / "production.sqlite3"))
    with pytest.raises(ValueError, match="high-entropy"):
        Database(str(tmp_path / "production.sqlite3"), b"x" * 32, secrets.token_bytes(32))
    key = secrets.token_bytes(32)
    with pytest.raises(ValueError, match="distinct"):
        Database(str(tmp_path / "production.sqlite3"), key, key)
    good = Database(str(tmp_path / "production.sqlite3"), key, secrets.token_bytes(32))
    assert good.path.exists()
