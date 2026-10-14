"""Encrypted SQLite persistence and tamper-evident decision history.

SQLite WAL and BEGIN IMMEDIATE protect updates across processes. AES-GCM protects
payloads; an HMAC chain detects changes to retained history. The chain is not an
external notarization: a database rollback with the same keys requires an external
checkpoint to detect, and someone holding both keys can rewrite evidence.
"""
from __future__ import annotations

import base64
import getpass
import hashlib
import hmac
import json
import os
import secrets
import sqlite3
import subprocess
import threading
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Callable, Optional

from cryptography.hazmat.primitives.ciphers.aead import AESGCM

try:
    from .records import Records as LegacyRecords
except ImportError:
    from records import Records as LegacyRecords

MAX_BLOB_BYTES = 20 * 1024 * 1024
MAX_OBJECT_BYTES = 80 * 1024 * 1024


def _json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("utf-8")


def _decode_key(value: Any, name: str) -> bytes:
    if isinstance(value, bytes):
        key = value
    else:
        try:
            key = base64.b64decode(str(value), altchars=b"-_", validate=True)
        except (ValueError, TypeError) as exc:
            raise ValueError(f"{name} must be a base64 encoded 32-byte key") from exc
    if len(key) != 32 or len(set(key)) < 12:
        raise ValueError(f"{name} must contain 32 high-entropy bytes")
    return key


def _protect_key_file(path: Path) -> None:
    if os.name != "nt":
        path.chmod(0o600)
        return
    account = os.environ.get("USERDOMAIN", "") + "\\" + os.environ.get("USERNAME", getpass.getuser())
    account = account.lstrip("\\")
    result = subprocess.run(
        ["icacls", str(path), "/inheritance:r", "/grant:r", f"{account}:(F)", "*S-1-5-18:(F)"],
        capture_output=True, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
    )
    if result.returncode:
        raise RuntimeError("could not restrict permissions of development key file")


def _local_keys(path: Path) -> tuple[bytes, bytes]:
    key_path = Path(str(path) + ".keys.json")
    # FileLock handles the small race between O_EXCL creation and complete writing.
    from filelock import FileLock
    with FileLock(str(key_path) + ".lock", timeout=30):
        if not key_path.exists():
            values = {"encryption": base64.b64encode(secrets.token_bytes(32)).decode(),
                      "audit": base64.b64encode(secrets.token_bytes(32)).decode()}
            fd = os.open(str(key_path), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            try:
                with os.fdopen(fd, "wb") as stream:
                    _protect_key_file(key_path)
                    stream.write(_json(values))
                    stream.flush()
                    os.fsync(stream.fileno())
                _protect_key_file(key_path)
            except BaseException:
                key_path.unlink(missing_ok=True)
                raise
        if key_path.is_symlink():
            raise ValueError("development key file must not be a symbolic link")
        if os.name != "nt" and key_path.stat().st_mode & 0o077:
            raise ValueError("development key file permissions must be 0600")
        # Reassert the explicit Windows ACL for existing development key files.
        if os.name == "nt":
            _protect_key_file(key_path)
        values = json.loads(key_path.read_text(encoding="utf-8"))
        return _decode_key(values["encryption"], "encryption key"), _decode_key(values["audit"], "audit key")


class Database:
    """One shared encrypted local database. Keys may be injected for isolated tests."""
    def __init__(self, path: Optional[str] = None, encryption_key: Any = None,
                 audit_key: Any = None, clock: Callable[[], float] = time.time) -> None:
        production = os.environ.get("JURIX_ENV", "development").lower() == "production"
        configured_path = path or os.environ.get("JURIX_DATABASE_PATH")
        if production and not configured_path:
            raise ValueError("JURIX_DATABASE_PATH is required in production")
        self.path = Path(configured_path or Path(__file__).resolve().parent / "data" / "jurix.sqlite3")
        if production and not self.path.is_absolute():
            raise ValueError("JURIX_DATABASE_PATH must be absolute in production")
        self.path = self.path.resolve()
        self.path.parent.mkdir(parents=True, exist_ok=True)
        enc = encryption_key if encryption_key is not None else os.environ.get("JURIX_ENCRYPTION_KEY")
        audit = audit_key if audit_key is not None else os.environ.get("JURIX_AUDIT_KEY")
        if (enc is None) != (audit is None):
            raise ValueError("configure both JURIX_ENCRYPTION_KEY and JURIX_AUDIT_KEY")
        if enc is None:
            if production:
                raise ValueError("JURIX_ENCRYPTION_KEY and JURIX_AUDIT_KEY are required in production")
            enc, audit = _local_keys(self.path)
        self.encryption_key = _decode_key(enc, "JURIX_ENCRYPTION_KEY")
        self.audit_key = _decode_key(audit, "JURIX_AUDIT_KEY")
        if hmac.compare_digest(self.encryption_key, self.audit_key):
            raise ValueError("encryption and audit keys must be distinct")
        self.clock = clock
        self._cipher = AESGCM(self.encryption_key)
        with self.connection() as conn:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.executescript("""
                CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value BLOB NOT NULL);
                CREATE TABLE IF NOT EXISTS objects (
                    owner TEXT NOT NULL, category TEXT NOT NULL, object_key TEXT NOT NULL,
                    payload BLOB NOT NULL, PRIMARY KEY(owner, category, object_key));
                CREATE TABLE IF NOT EXISTS activity_evidence (
                    seq INTEGER PRIMARY KEY, client_id TEXT NOT NULL, payload BLOB NOT NULL,
                    previous_mac TEXT NOT NULL, mac TEXT NOT NULL);
                CREATE TRIGGER IF NOT EXISTS activity_no_update BEFORE UPDATE ON activity_evidence
                    BEGIN SELECT RAISE(ABORT, 'activity evidence is append-only'); END;
                CREATE TRIGGER IF NOT EXISTS activity_no_delete BEFORE DELETE ON activity_evidence
                    BEGIN SELECT RAISE(ABORT, 'activity evidence is append-only'); END;
                CREATE TABLE IF NOT EXISTS users (user_id TEXT PRIMARY KEY, payload BLOB NOT NULL);
                CREATE TABLE IF NOT EXISTS firms (firm_id TEXT PRIMARY KEY, payload BLOB NOT NULL);
                CREATE TABLE IF NOT EXISTS memberships (
                    user_id TEXT PRIMARY KEY REFERENCES users(user_id),
                    firm_id TEXT NOT NULL REFERENCES firms(firm_id), role TEXT NOT NULL,
                    active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0,1)),
                    CHECK(role IN ('admin','reviewer','viewer')));
                CREATE TABLE IF NOT EXISTS sessions (
                    token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(user_id),
                    issued_at REAL NOT NULL, expires_at REAL NOT NULL, revoked INTEGER NOT NULL DEFAULT 0);
                CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
                CREATE TABLE IF NOT EXISTS login_failures (
                    user_id TEXT PRIMARY KEY, window_started REAL NOT NULL,
                    failures INTEGER NOT NULL, blocked_until REAL NOT NULL);
                CREATE TABLE IF NOT EXISTS clients (
                    client_id TEXT PRIMARY KEY, firm_id TEXT NOT NULL REFERENCES firms(firm_id),
                    payload BLOB NOT NULL);
                CREATE INDEX IF NOT EXISTS clients_firm ON clients(firm_id);
                CREATE TABLE IF NOT EXISTS invitations (
                    token_hash TEXT PRIMARY KEY, firm_id TEXT NOT NULL REFERENCES firms(firm_id),
                    user_id TEXT NOT NULL, payload BLOB NOT NULL, expires_at REAL NOT NULL,
                    revoked INTEGER NOT NULL DEFAULT 0, used INTEGER NOT NULL DEFAULT 0);
                CREATE TABLE IF NOT EXISTS migrations (source_hash TEXT PRIMARY KEY, imported_at REAL NOT NULL);
            """)
            conn.execute("BEGIN IMMEDIATE")
            try:
                row = conn.execute("SELECT value FROM metadata WHERE key='key_check'").fetchone()
                if row:
                    if self.decrypt(row[0], "key-check") != {"format": "jurix-sqlite-v1"}:
                        raise ValueError("invalid database key check")
                    check = conn.execute("SELECT value FROM metadata WHERE key='audit_key_check'").fetchone()
                    expected = hmac.new(self.audit_key, b"jurix-audit-key-v1", hashlib.sha256).hexdigest().encode()
                    if not check or not hmac.compare_digest(bytes(check[0]), expected):
                        raise ValueError("incorrect audit key for this database")
                else:
                    conn.execute("INSERT INTO metadata VALUES ('key_check',?)", (self.encrypt({"format": "jurix-sqlite-v1"}, "key-check"),))
                    conn.execute("INSERT INTO metadata VALUES ('audit_key_check',?)", (hmac.new(self.audit_key, b"jurix-audit-key-v1", hashlib.sha256).hexdigest().encode(),))
                conn.commit()
            except BaseException:
                conn.rollback()
                raise

    @contextmanager
    def connection(self):
        conn = sqlite3.connect(str(self.path), timeout=30, isolation_level=None)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA foreign_keys=ON")
        conn.execute("PRAGMA busy_timeout=30000")
        conn.execute("PRAGMA synchronous=FULL")
        try:
            yield conn
        finally:
            conn.close()

    @contextmanager
    def transaction(self):
        with self.connection() as conn:
            conn.execute("BEGIN IMMEDIATE")
            try:
                yield conn
                conn.commit()
            except BaseException:
                conn.rollback()
                raise

    def encrypt(self, value: Any, context: str) -> bytes:
        nonce = secrets.token_bytes(12)
        return nonce + self._cipher.encrypt(nonce, _json(value), context.encode("utf-8"))

    def decrypt(self, payload: bytes, context: str) -> Any:
        try:
            return json.loads(self._cipher.decrypt(payload[:12], payload[12:], context.encode("utf-8")))
        except Exception as exc:
            raise ValueError("encrypted database payload failed authentication") from exc

    def identity_id(self, email: str) -> str:
        return hmac.new(self.audit_key, b"identity:" + email.strip().lower().encode(), hashlib.sha256).hexdigest()

    @staticmethod
    def _object_context(owner: str, category: str, key: str) -> str:
        return "object:" + json.dumps([owner, category, key], separators=(",", ":"))

    def _get_object(self, conn, owner: str, category: str, key: str, default=None):
        row = conn.execute("SELECT payload FROM objects WHERE owner=? AND category=? AND object_key=?", (owner, category, key)).fetchone()
        return self.decrypt(row[0], self._object_context(owner, category, key)) if row else default

    def _put_object(self, conn, owner: str, category: str, key: str, value: Any) -> None:
        payload = self.encrypt(value, self._object_context(owner, category, key))
        # Limit feature state and archives per workspace. The records adapter is
        # separately bounded by application upload and matter policies.
        if owner != "_system":
            used = conn.execute("SELECT COALESCE(SUM(length(payload)),0) FROM objects WHERE owner=? AND NOT(category=? AND object_key=?)", (owner, category, key)).fetchone()[0]
            if used + len(payload) > MAX_OBJECT_BYTES:
                raise ValueError("firm object storage limit reached (80 MB)")
        conn.execute("INSERT INTO objects VALUES (?,?,?,?) ON CONFLICT(owner,category,object_key) DO UPDATE SET payload=excluded.payload",
                     (owner, category, key, payload))

    def get_object(self, owner: str, category: str, key: str, default=None):
        with self.connection() as conn:
            return self._get_object(conn, owner, category, key, default)

    def put_object(self, owner: str, category: str, key: str, value: Any) -> Any:
        with self.transaction() as conn:
            self._put_object(conn, owner, category, key, value)
        return value

    def list_objects(self, owner: str, category: str) -> list[Any]:
        with self.connection() as conn:
            return [self.decrypt(row["payload"], self._object_context(owner, category, row["object_key"]))
                    for row in conn.execute("SELECT object_key,payload FROM objects WHERE owner=? AND category=? ORDER BY object_key", (owner, category))]

    def delete_object(self, owner: str, category: str, key: str) -> bool:
        with self.transaction() as conn:
            return bool(conn.execute("DELETE FROM objects WHERE owner=? AND category=? AND object_key=?", (owner, category, key)).rowcount)

    def save_blob(self, owner: str, filename: str, data: bytes, content_type: str = "application/octet-stream") -> str:
        if not isinstance(data, bytes) or len(data) > MAX_BLOB_BYTES:
            raise ValueError("original document must contain at most 20 MB")
        blob_id = secrets.token_urlsafe(24)
        self.put_object(owner, "archive", blob_id, {
            "id": blob_id, "filename": Path(filename).name, "content_type": content_type,
            "size": len(data), "sha256": hashlib.sha256(data).hexdigest(), "created_at": self.clock(),
            "data": base64.b64encode(data).decode("ascii"),
        })
        return blob_id

    def open_blob(self, owner: str, blob_id: str) -> dict[str, Any]:
        row = self.get_object(owner, "archive", blob_id)
        if row is None:
            raise KeyError("original document not found")
        row["data"] = base64.b64decode(row["data"], validate=True)
        if len(row["data"]) > MAX_BLOB_BYTES or hashlib.sha256(row["data"]).hexdigest() != row["sha256"]:
            raise ValueError("original document failed validation")
        return row

    def _audit_rows(self, conn) -> tuple[dict[str, list[dict]], str, int]:
        grouped: dict[str, list[dict]] = {}
        previous = "0" * 64
        seq = 0
        for row in conn.execute("SELECT * FROM activity_evidence ORDER BY seq"):
            if row["seq"] != seq + 1 or row["previous_mac"] != previous:
                raise ValueError("activity evidence chain failed verification")
            payload = self.decrypt(row["payload"], f"activity:{row['seq']}:{row['client_id']}")
            expected = hmac.new(self.audit_key, _json([row["seq"], row["client_id"], previous, payload]), hashlib.sha256).hexdigest()
            if not hmac.compare_digest(expected, row["mac"]):
                raise ValueError("activity evidence chain failed verification")
            grouped.setdefault(row["client_id"], []).append(payload)
            previous, seq = row["mac"], row["seq"]
        head = conn.execute("SELECT value FROM metadata WHERE key='activity_head'").fetchone()
        if head:
            checkpoint = self.decrypt(head[0], "activity-head")
            if checkpoint != {"seq": seq, "mac": previous}:
                raise ValueError("activity evidence head failed verification")
        elif seq:
            raise ValueError("activity evidence head missing")
        return grouped, previous, seq

    def _sync_activity(self, conn, activity: dict[str, list[dict]]) -> None:
        existing, previous, seq = self._audit_rows(conn)
        for client_id, rows in existing.items():
            if activity.get(client_id, [])[:len(rows)] != rows:
                raise ValueError("activity evidence cannot be removed or rewritten")
        for client_id, rows in activity.items():
            for event in rows[len(existing.get(client_id, [])):]:
                seq += 1
                mac = hmac.new(self.audit_key, _json([seq, client_id, previous, event]), hashlib.sha256).hexdigest()
                conn.execute("INSERT INTO activity_evidence VALUES (?,?,?,?,?)", (seq, client_id, self.encrypt(event, f"activity:{seq}:{client_id}"), previous, mac))
                previous = mac
        conn.execute("INSERT INTO metadata VALUES ('activity_head',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
                     (self.encrypt({"seq": seq, "mac": previous}, "activity-head"),))

    def verify_audit(self) -> dict[str, Any]:
        with self.connection() as conn:
            conn.execute("BEGIN")
            grouped, head, count = self._audit_rows(conn)
            state = self._get_object(conn, "_system", "records", "state", {})
            if grouped != {key: value for key, value in state.get("activity", {}).items() if value}:
                raise ValueError("activity evidence does not match records")
            return {"valid": True, "events": count, "head": head}

    def backup(self, destination: str) -> str:
        target = Path(destination).resolve()
        if target == self.path or target.exists():
            raise ValueError("backup destination must be a new file")
        target.parent.mkdir(parents=True, exist_ok=True)
        with self.connection() as conn:
            backup = sqlite3.connect(str(target))
            try:
                conn.backup(backup)
                if backup.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
                    raise ValueError("database backup failed integrity validation")
            finally:
                backup.close()
        return str(target)


class _RecordsTransaction:
    """Adapt the complete Records interface to one serialized SQL transaction."""
    def __init__(self, records: "EncryptedRecords") -> None:
        self.records = records
        self.lock = threading.RLock()
        self.local = threading.local()

    def __enter__(self):
        self.lock.acquire()
        try:
            if getattr(self.local, "depth", 0) == 0:
                self.local.context = self.records.database.transaction()
                self.local.conn = self.local.context.__enter__()
            self.local.depth = getattr(self.local, "depth", 0) + 1
            return self
        except BaseException:
            self.lock.release()
            raise

    def __exit__(self, exc_type, exc, traceback):
        try:
            self.local.depth -= 1
            if self.local.depth == 0:
                self.local.context.__exit__(exc_type, exc, traceback)
                del self.local.conn
        finally:
            self.lock.release()


class EncryptedRecords(LegacyRecords):
    """Records-compatible encrypted adapter; no automatic JSON import.

    A single encrypted state payload keeps all existing Records methods atomic.
    Decision evidence additionally lives in an append-only authenticated journal.
    """
    def __init__(self, path: Optional[str] = None, database: Optional[Database] = None,
                 encryption_key: Any = None, audit_key: Any = None) -> None:
        self.database = database or Database(path, encryption_key, audit_key)
        self.path = self.database.path
        self._lock = _RecordsTransaction(self)
        self._data = self.empty_state()
        with self._lock:
            self._load()

    @staticmethod
    def empty_state() -> dict[str, Any]:
        return {"firms": {}, "matters": {}, "documents": {}, "runs": {}, "notes": {}, "exports": [],
                "reviews": {}, "activity": {}, "firm_playbooks": {}, "billing_runs": {}}

    def _load(self) -> None:
        conn = self._lock.local.conn
        state = self.empty_state()
        state.update(self.database._get_object(conn, "_system", "records", "state", {}))
        journal, _, _ = self.database._audit_rows(conn)
        if journal != {key: value for key, value in state.get("activity", {}).items() if value}:
            raise ValueError("activity evidence does not match records")
        self._data = state

    def _save(self) -> None:
        conn = self._lock.local.conn
        self.database._sync_activity(conn, self._data["activity"])
        self.database._put_object(conn, "_system", "records", "state", self._data)

    def drop_matter(self, client_id: str) -> None:
        with self._lock:
            self._load()
            for category in ["matters", "documents", "runs", "notes", "reviews", "billing_runs"]:
                self._data[category].pop(client_id, None)
            # Decision evidence is retained after offboarding.
            self._save()

    def verify_audit(self) -> dict[str, Any]:
        return self.database.verify_audit()


Records = EncryptedRecords
