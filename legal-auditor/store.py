"""Local firm registry — maps users and clients to DBX tenant ids.

This is Legal Auditor state, not DBX state. Client corpora live only in
their sealed DBX tenants.
"""

from __future__ import annotations

import json
import os
import secrets
import time
from filelock import FileLock
from hashlib import pbkdf2_hmac
from pathlib import Path
from typing import Any, Dict, List, Optional


def _hash_password(password: str, salt: Optional[str] = None) -> str:
    if salt is None:
        salt = secrets.token_hex(16)
    digest = pbkdf2_hmac(
        "sha256", password.encode("utf-8"), salt.encode("utf-8"), 120_000
    ).hex()
    return f"{salt}${digest}"


def _verify_password(password: str, stored: str) -> bool:
    salt, _, digest = stored.partition("$")
    if not salt or not digest:
        return False
    check = pbkdf2_hmac(
        "sha256", password.encode("utf-8"), salt.encode("utf-8"), 120_000
    ).hex()
    return secrets.compare_digest(check, digest)


class FirmStore:
    def __init__(self, path: Optional[str] = None) -> None:
        default = Path(__file__).resolve().parent / "data" / "firm_store.json"
        self.path = Path(path or os.environ.get("LA_STORE_PATH", str(default)))
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = FileLock(str(self.path) + ".lock")
        self._data: Dict[str, Any] = {"users": {}, "sessions": {}, "clients": {}}
        self._load()

    def _load(self) -> None:
        if self.path.exists():
            self._data = json.loads(self.path.read_text(encoding="utf-8"))

    def _save(self) -> None:
        tmp = self.path.with_suffix(".tmp")
        tmp.write_text(json.dumps(self._data, indent=2), encoding="utf-8")
        tmp.replace(self.path)

    def register(self, email: str, password: str, firm_name: str) -> Dict[str, Any]:
        email = email.strip().lower()
        with self._lock:
            self._load()
            if email in self._data["users"]:
                raise ValueError("user already exists")
            user = {
                "email": email,
                "firm_name": firm_name.strip() or email,
                "password_hash": _hash_password(password),
                "created_at": time.time(),
            }
            self._data["users"][email] = user
            self._save()
            return {"email": email, "firm_name": user["firm_name"]}

    def login(self, email: str, password: str) -> str:
        email = email.strip().lower()
        with self._lock:
            self._load()
            user = self._data["users"].get(email)
            if not user or not _verify_password(password, user["password_hash"]):
                raise ValueError("invalid credentials")
            token = secrets.token_urlsafe(32)
            self._data["sessions"][token] = {
                "email": email,
                "created_at": time.time(),
            }
            self._save()
            return token

    def public_user(self, email: str) -> Optional[Dict[str, Any]]:
        email = email.strip().lower()
        with self._lock:
            self._load()
            user = self._data["users"].get(email)
            if not user:
                return None
            return {"email": user["email"], "firm_name": user["firm_name"]}

    def user_for_token(self, token: str) -> Optional[Dict[str, Any]]:
        with self._lock:
            self._load()
            session = self._data["sessions"].get(token)
            if not session:
                return None
            return self._data["users"].get(session["email"])

    def add_client(
        self,
        owner_email: str,
        client_name: str,
        tenant_id: str,
        key_id: str,
        secret: str,
    ) -> Dict[str, Any]:
        with self._lock:
            self._load()
            client_id = secrets.token_hex(8)
            record = {
                "id": client_id,
                "owner_email": owner_email,
                "name": client_name,
                "tenant_id": tenant_id,
                "key_id": key_id,
                "secret": secret,
                "created_at": time.time(),
            }
            self._data["clients"][client_id] = record
            self._save()
            return {
                "id": client_id,
                "name": client_name,
                "tenant_id": tenant_id,
                "created_at": record["created_at"],
            }

    def list_clients(self, owner_email: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            out = []
            for c in self._data["clients"].values():
                if c["owner_email"] == owner_email:
                    out.append(
                        {
                            "id": c["id"],
                            "name": c["name"],
                            "tenant_id": c["tenant_id"],
                            "created_at": c["created_at"],
                        }
                    )
            return sorted(out, key=lambda x: x["created_at"])

    def get_client(self, owner_email: str, client_id: str) -> Optional[Dict[str, Any]]:
        with self._lock:
            self._load()
            c = self._data["clients"].get(client_id)
            if not c or c["owner_email"] != owner_email:
                return None
            return dict(c)

    def remove_client(
        self, owner_email: str, client_id: str
    ) -> Optional[Dict[str, Any]]:
        with self._lock:
            self._load()
            c = self._data["clients"].get(client_id)
            if not c or c["owner_email"] != owner_email:
                return None
            del self._data["clients"][client_id]
            self._save()
            return dict(c)
