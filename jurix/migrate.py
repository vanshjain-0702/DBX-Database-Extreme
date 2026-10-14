"""Explicit, validated, idempotent migration of the old Jurix JSON stores.

Dry runs read source files only. Applied migrations retain the original files and
create an encrypted SQLite backup before importing in one SQL transaction. Legacy
session bearer tokens are deliberately never copied to the new database.
"""
from __future__ import annotations

import argparse
import hashlib
import hmac
import json
from pathlib import Path
from typing import Any, Optional

try:
    from .persistence import Database, EncryptedRecords, _json
    from .security import normalize_email
except ImportError:
    from persistence import Database, EncryptedRecords, _json
    from security import normalize_email


def read_sources(identity_path: str, records_path: str) -> tuple[dict, dict, str]:
    identity_raw = Path(identity_path).read_bytes()
    records_raw = Path(records_path).read_bytes()
    identity = json.loads(identity_raw)
    records = json.loads(records_raw)
    if not isinstance(identity, dict) or not isinstance(records, dict):
        raise ValueError("migration sources must be JSON objects")
    for category in ("users", "sessions", "clients"):
        if not isinstance(identity.get(category, {}), dict):
            raise ValueError("identity migration source is malformed")
    state = EncryptedRecords.empty_state()
    state.update(records)
    for category, rows in state.items():
        if not isinstance(rows, list if category == "exports" else dict):
            raise ValueError(f"records category {category} is malformed")
    users = identity.get("users", {})
    if len({normalize_email(email) for email in users}) != len(users):
        raise ValueError("migration contains duplicate normalized user accounts")
    normalized_users = {normalize_email(email) for email in users}
    for email, row in users.items():
        if not isinstance(row, dict) or normalize_email(row.get("email", email)) != normalize_email(email):
            raise ValueError("migration account email does not match its key")
        stored = row.get("password_hash", "")
        parts = stored.split("$") if isinstance(stored, str) else []
        if not ((len(parts) == 2 and len(parts[0]) == 32 and len(parts[1]) == 64)
                or (len(parts) == 4 and parts[0] == "pbkdf2_sha256" and int(parts[1]) >= 600_000 and len(parts[3]) == 64)):
            raise ValueError("migration account has an unsupported password hash")
        try:
            bytes.fromhex(parts[-1])
            bytes.fromhex(parts[-2])
        except ValueError as exc:
            raise ValueError("migration account password hash is malformed") from exc
    clients = identity.get("clients", {})
    for client_id, row in clients.items():
        if not isinstance(row, dict) or row.get("id") != client_id:
            raise ValueError("migration client id does not match its key")
        if normalize_email(row.get("owner_email", "")) not in normalized_users:
            raise ValueError("migration client owner has no account")
        if not all(isinstance(row.get(key), str) and row[key] for key in ("tenant_id", "key_id", "secret", "name")):
            raise ValueError("migration client is missing provider credentials")
    for category in ("matters", "documents", "runs", "notes", "reviews", "billing_runs"):
        if any(client_id not in clients for client_id in state[category]):
            raise ValueError(f"migration {category} refers to an unknown matter")
    # Activity and exports may survive offboarding; their matter ids are retained.
    for owner in set(state["firms"]) | set(state["firm_playbooks"]):
        if normalize_email(owner) not in normalized_users:
            raise ValueError("migration firm metadata owner has no account")
    for export in state["exports"]:
        if not isinstance(export, dict) or normalize_email(export.get("owner", "")) not in normalized_users or not export.get("id"):
            raise ValueError("migration export is malformed")
    for client_id, docs in state["documents"].items():
        if not isinstance(docs, list) or len({doc.get("id") for doc in docs}) != len(docs):
            raise ValueError("migration contains duplicate or malformed document ids")
        for doc in docs:
            if not isinstance(doc.get("id"), str) or not isinstance(doc.get("text"), str) or not isinstance(doc.get("chunk_ids"), list):
                raise ValueError("migration document is malformed")
    for client_id, events in state["activity"].items():
        if not isinstance(events, list) or any(not isinstance(event, dict) or not event.get("id") for event in events):
            raise ValueError("migration activity evidence is malformed")
    # Canonical content makes repeated runs idempotent even after formatting changes.
    fingerprint = hashlib.sha256(_json([identity, records])).hexdigest()
    return identity, state, fingerprint


def _report(identity: dict, records: dict, fingerprint: str) -> dict[str, Any]:
    return {"source_hash": fingerprint, "accounts": len(identity.get("users", {})),
            "matters": len(identity.get("clients", {})),
            "documents": sum(len(rows) for rows in records["documents"].values()),
            "runs": len(records["runs"]),
            "legacy_sessions_revoked": len(identity.get("sessions", {})),
            "activity_events": sum(len(rows) for rows in records["activity"].values())}


def migrate(identity_path: str, records_path: str, database: Optional[Database] = None,
            dry_run: bool = False, backup_path: Optional[str] = None) -> dict[str, Any]:
    identity, state, fingerprint = read_sources(identity_path, records_path)
    report = _report(identity, state, fingerprint)
    if dry_run:
        return {**report, "dry_run": True, "valid": True}
    database = database or Database()
    with database.connection() as conn:
        if conn.execute("SELECT 1 FROM migrations WHERE source_hash=?", (fingerprint,)).fetchone():
            return {**report, "already_applied": True}
    backup_path = backup_path or str(database.path) + ".pre-migration-" + str(int(database.clock())) + "-" + fingerprint[:8] + ".sqlite3"
    backup = database.backup(backup_path)
    with database.transaction() as conn:
        # A concurrent migration can complete while the backup is being made.
        if conn.execute("SELECT 1 FROM migrations WHERE source_hash=?", (fingerprint,)).fetchone():
            return {**report, "already_applied": True, "backup": backup}
        old_records = database._get_object(conn, "_system", "records", "state", {})
        if any(old_records.values()) or conn.execute("SELECT 1 FROM users LIMIT 1").fetchone():
            raise ValueError("migration target must contain no accounts or working records")
        owners: dict[str, str] = {}
        for raw_email, old in identity.get("users", {}).items():
            email = normalize_email(raw_email)
            user_id = database.identity_id(email)
            firm_id = hmac.new(database.audit_key, b"migration-firm:" + email.encode(), hashlib.sha256).hexdigest()[:32]
            owners[email] = "firm:" + firm_id
            firm_meta = state["firms"].get(raw_email, {})
            firm = {"firm_name": firm_meta.get("firm_name") or old.get("firm_name") or email,
                    "code": firm_meta.get("code") or "", "created_at": old.get("created_at", database.clock())}
            user = {**old, "email": email}
            conn.execute("INSERT INTO users VALUES (?,?)", (user_id, database.encrypt(user, "user:" + user_id)))
            conn.execute("INSERT INTO firms VALUES (?,?)", (firm_id, database.encrypt(firm, "firm:" + firm_id)))
            conn.execute("INSERT INTO memberships VALUES (?,?,?,1)", (user_id, firm_id, "admin"))
        for client_id, old in identity.get("clients", {}).items():
            firm_id = owners[normalize_email(old["owner_email"])].removeprefix("firm:")
            row = {**old, "firm_id": firm_id}
            conn.execute("INSERT INTO clients VALUES (?,?,?)", (client_id, firm_id, database.encrypt(row, "client:" + client_id)))
        state["firms"] = {owners[normalize_email(email)]: row for email, row in state["firms"].items()}
        state["firm_playbooks"] = {owners[normalize_email(email)]: rows for email, rows in state["firm_playbooks"].items()}
        for row in state["exports"]:
            row["owner"] = owners[normalize_email(row["owner"])]
        database._sync_activity(conn, state["activity"])
        database._put_object(conn, "_system", "records", "state", state)
        # Roundtrip and referential validation occur before the commit.
        if database._get_object(conn, "_system", "records", "state") != state:
            raise ValueError("migration records roundtrip validation failed")
        if conn.execute("PRAGMA foreign_key_check").fetchone():
            raise ValueError("migration identity referential validation failed")
        conn.execute("INSERT INTO migrations VALUES (?,?)", (fingerprint, database.clock()))
    database.verify_audit()
    return {**report, "applied": True, "backup": backup}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--identity", required=True, help="legacy firm_store.json")
    parser.add_argument("--records", required=True, help="legacy jurix_records.json")
    parser.add_argument("--database", help="encrypted SQLite destination (or JURIX_DATABASE_PATH)")
    parser.add_argument("--backup", help="new encrypted pre-migration backup path")
    parser.add_argument("--dry-run", action="store_true", help="validate source files without writing")
    args = parser.parse_args()
    database = None if args.dry_run else Database(args.database)
    result = migrate(args.identity, args.records, database, args.dry_run, args.backup)
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
