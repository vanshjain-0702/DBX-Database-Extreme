"""Encrypted firm identity, expiring sessions, and firm membership authorization."""
from __future__ import annotations

import hashlib
import hmac
import os
import re
import secrets
import time
from typing import Any, Callable, Optional

try:
    from .persistence import Database
except ImportError:
    from persistence import Database

PASSWORD_ITERATIONS = 600_000
ROLES = frozenset({"admin", "reviewer", "viewer"})


def normalize_email(email: str) -> str:
    value = str(email).strip().lower()
    if len(value) > 254 or not re.fullmatch(r"[^\s@]+@[^\s@]+\.[^\s@]+", value):
        raise ValueError("valid email address is required")
    return value


def hash_password(password: str) -> str:
    if not isinstance(password, str) or len(password) < 12 or len(password) > 1024:
        raise ValueError("password must contain 12 to 1024 characters")
    salt = secrets.token_bytes(24).hex()
    digest = hashlib.pbkdf2_hmac("sha256", password.encode(), bytes.fromhex(salt), PASSWORD_ITERATIONS).hex()
    return f"pbkdf2_sha256${PASSWORD_ITERATIONS}${salt}${digest}"


def verify_password(password: str, stored: str) -> bool:
    if not isinstance(password, str) or len(password) > 1024:
        return False
    try:
        parts = stored.split("$")
        if len(parts) == 4 and parts[0] == "pbkdf2_sha256":
            iterations = int(parts[1])
            if iterations < PASSWORD_ITERATIONS or iterations > 2_000_000:
                return False
            salt = bytes.fromhex(parts[2])
            digest = parts[3]
        elif len(parts) == 2:  # Explicitly migrated legacy accounts; upgraded on login.
            salt = parts[0].encode()
            digest = parts[1]
            iterations = 120_000
        else:
            return False
        check = hashlib.pbkdf2_hmac("sha256", password.encode(), salt, iterations).hex()
        return hmac.compare_digest(check, digest)
    except (ValueError, TypeError, AttributeError):
        return False


class SecureFirmStore:
    def __init__(self, path: Optional[str] = None, database: Optional[Database] = None,
                 encryption_key: Any = None, audit_key: Any = None,
                 clock: Callable[[], float] = time.time, session_ttl: float = 43200,
                 invitation_ttl: float = 172800) -> None:
        if session_ttl <= 0 or session_ttl > 7 * 86400:
            raise ValueError("session lifetime must be positive and no longer than seven days")
        if invitation_ttl <= 0 or invitation_ttl > 7 * 86400:
            raise ValueError("invitation lifetime must be positive and no longer than seven days")
        self.database = database or Database(path, encryption_key, audit_key, clock)
        self.path = self.database.path
        self.clock = clock
        self.session_ttl = session_ttl
        self.invitation_ttl = invitation_ttl

    @staticmethod
    def _token_hash(token: str) -> str:
        return hashlib.sha256(token.encode("utf-8")).hexdigest()

    def _user(self, conn, user_id: str) -> Optional[dict[str, Any]]:
        row = conn.execute("SELECT payload FROM users WHERE user_id=?", (user_id,)).fetchone()
        return self.database.decrypt(row[0], "user:" + user_id) if row else None

    def _firm(self, conn, firm_id: str) -> dict[str, Any]:
        row = conn.execute("SELECT payload FROM firms WHERE firm_id=?", (firm_id,)).fetchone()
        if not row:
            raise ValueError("firm not found")
        return self.database.decrypt(row[0], "firm:" + firm_id)

    def _membership(self, conn, email: str, roles=None):
        user_id = self.database.identity_id(normalize_email(email))
        row = conn.execute("SELECT * FROM memberships WHERE user_id=? AND active=1", (user_id,)).fetchone()
        if not row or (roles is not None and row["role"] not in roles):
            raise PermissionError("this action is not permitted for your firm role")
        return row

    def _public(self, conn, user_id: str) -> Optional[dict[str, Any]]:
        user = self._user(conn, user_id)
        membership = conn.execute("SELECT * FROM memberships WHERE user_id=? AND active=1", (user_id,)).fetchone()
        if not user or not membership:
            return None
        firm = self._firm(conn, membership["firm_id"])
        return {"email": user["email"], "firm_name": firm["firm_name"], "firm_id": membership["firm_id"],
                "role": membership["role"], "created_at": user.get("created_at")}

    def register(self, email: str, password: str, firm_name: str, invite_token: str = "") -> dict[str, Any]:
        email = normalize_email(email)
        password_hash = hash_password(password)
        firm_name = str(firm_name).strip() or email
        if len(firm_name) > 200:
            raise ValueError("firm name must contain at most 200 characters")
        user_id = self.database.identity_id(email)
        firm_id = secrets.token_hex(16)
        user = {"email": email, "password_hash": password_hash, "created_at": self.clock()}
        firm = {"firm_name": firm_name, "code": "", "created_at": self.clock()}
        with self.database.transaction() as conn:
            invitation = None
            role = "admin"
            if invite_token:
                digest = self._token_hash(invite_token)
                invitation = conn.execute("SELECT * FROM invitations WHERE token_hash=? AND user_id=? AND used=0 AND revoked=0 AND expires_at>?", (digest, user_id, self.clock())).fetchone()
                if not invitation:
                    raise ValueError("invitation is invalid or expired")
                details = self.database.decrypt(invitation["payload"], "invite:" + digest)
                inviter = self._membership(conn, details["invited_by"], {"admin"})
                if inviter["firm_id"] != invitation["firm_id"]:
                    raise ValueError("invitation is no longer valid")
                firm_id, role = invitation["firm_id"], details["role"]
            registration_open = os.environ.get("JURIX_REGISTRATION_OPEN", os.environ.get("REGISTRATION_OPEN", "false")).lower() in {"1", "true", "yes"}
            if not invitation and os.environ.get("JURIX_ENV", "development").lower() == "production" and not registration_open:
                if conn.execute("SELECT COUNT(*) FROM users").fetchone()[0]:
                    raise PermissionError("public registration is closed; contact your firm admin")
            if self._user(conn, user_id):
                raise ValueError("user already exists")
            conn.execute("INSERT INTO users VALUES (?,?)", (user_id, self.database.encrypt(user, "user:" + user_id)))
            if not invitation:
                conn.execute("INSERT INTO firms VALUES (?,?)", (firm_id, self.database.encrypt(firm, "firm:" + firm_id)))
            else:
                conn.execute("UPDATE invitations SET used=1 WHERE token_hash=?", (invitation["token_hash"],))
            conn.execute("INSERT INTO memberships VALUES (?,?,?,1)", (user_id, firm_id, role))
            return self._public(conn, user_id)

    def public_user(self, email: str) -> Optional[dict[str, Any]]:
        with self.database.connection() as conn:
            return self._public(conn, self.database.identity_id(normalize_email(email)))

    def get_user(self, email: str) -> Optional[dict[str, Any]]:
        return self.public_user(email)

    def account_count(self) -> int:
        with self.database.connection() as conn:
            return conn.execute("SELECT COUNT(*) FROM users").fetchone()[0]

    def firm_admin_emails(self) -> list[dict[str, str]]:
        with self.database.connection() as conn:
            return [{"email": self._user(conn, row["user_id"])["email"], "firm_id": row["firm_id"], "owner": "firm:" + row["firm_id"]}
                    for row in conn.execute("SELECT user_id,firm_id FROM memberships WHERE role='admin' AND active=1 ORDER BY firm_id,user_id")]

    def set_firm_code(self, email: str, code: str) -> None:
        with self.database.transaction() as conn:
            member = self._membership(conn, email, {"admin"})
            firm = self._firm(conn, member["firm_id"])
            firm["code"] = str(code).strip().upper()
            conn.execute("UPDATE firms SET payload=? WHERE firm_id=?", (self.database.encrypt(firm, "firm:" + member["firm_id"]), member["firm_id"]))

    def firm_code(self, email: str) -> str:
        with self.database.connection() as conn:
            member = self._membership(conn, email)
            return self._firm(conn, member["firm_id"]).get("code", "")

    def login(self, email: str, password: str, code: str = "") -> str:
        email = normalize_email(email)
        user_id = self.database.identity_id(email)
        # Check and mint in the same transaction; failed codes leave no sessions.
        with self.database.transaction() as conn:
            now = self.clock()
            attempts = conn.execute("SELECT * FROM login_failures WHERE user_id=?", (user_id,)).fetchone()
            if attempts and attempts["blocked_until"] > now:
                raise ValueError("too many failed login attempts; try again in 15 minutes")
            error = None
            user = self._user(conn, user_id)
            if not user or not verify_password(password, user.get("password_hash", "")):
                error = ValueError("invalid credentials")
            else:
                member = self._membership(conn, email)
                expected = self._firm(conn, member["firm_id"]).get("code", "")
                if code.strip() and (not expected or not hmac.compare_digest(code.strip().upper().encode(), expected.upper().encode())):
                    error = ValueError("firm code does not match this account")
            if error is not None:
                window = attempts["window_started"] if attempts and now - attempts["window_started"] < 900 else now
                failures = attempts["failures"] + 1 if attempts and window == attempts["window_started"] else 1
                blocked_until = now + 900 if failures >= 10 else 0
                conn.execute("INSERT INTO login_failures VALUES (?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET window_started=excluded.window_started,failures=excluded.failures,blocked_until=excluded.blocked_until", (user_id, window, failures, blocked_until))
                conn.execute("DELETE FROM login_failures WHERE window_started<? AND blocked_until<?", (now - 86400, now))
            else:
                if not user["password_hash"].startswith("pbkdf2_sha256$"):
                    # Legacy passwords retain their old length only until the account
                    # explicitly changes its password; successful login upgrades work.
                    salt = secrets.token_bytes(24).hex()
                    digest = hashlib.pbkdf2_hmac("sha256", password.encode(), bytes.fromhex(salt), PASSWORD_ITERATIONS).hex()
                    user["password_hash"] = f"pbkdf2_sha256${PASSWORD_ITERATIONS}${salt}${digest}"
                    conn.execute("UPDATE users SET payload=? WHERE user_id=?", (self.database.encrypt(user, "user:" + user_id), user_id))
                conn.execute("DELETE FROM login_failures WHERE user_id=?", (user_id,))
                token = secrets.token_urlsafe(32)
                conn.execute("INSERT INTO sessions VALUES (?,?,?,?,0)", (self._token_hash(token), user_id, now, now + self.session_ttl))
        if error is not None:
            raise error
        return token

    def user_for_token(self, token: str) -> Optional[dict[str, Any]]:
        if not isinstance(token, str) or len(token) > 512:
            return None
        with self.database.connection() as conn:
            row = conn.execute("SELECT user_id FROM sessions WHERE token_hash=? AND revoked=0 AND expires_at>?", (self._token_hash(token), self.clock())).fetchone()
            return self._public(conn, row[0]) if row else None

    def revoke_token(self, token: str) -> bool:
        with self.database.transaction() as conn:
            return bool(conn.execute("UPDATE sessions SET revoked=1 WHERE token_hash=? AND revoked=0", (self._token_hash(token),)).rowcount)

    def revoke_all_sessions(self, email: str) -> int:
        with self.database.transaction() as conn:
            return conn.execute("UPDATE sessions SET revoked=1 WHERE user_id=? AND revoked=0", (self.database.identity_id(normalize_email(email)),)).rowcount

    def change_password(self, email: str, current_password: str, new_password: str) -> None:
        password_hash = hash_password(new_password)
        user_id = self.database.identity_id(normalize_email(email))
        with self.database.transaction() as conn:
            user = self._user(conn, user_id)
            if not user or not verify_password(current_password, user.get("password_hash", "")):
                raise ValueError("invalid credentials")
            user["password_hash"] = password_hash
            conn.execute("UPDATE users SET payload=? WHERE user_id=?", (self.database.encrypt(user, "user:" + user_id), user_id))
            conn.execute("UPDATE sessions SET revoked=1 WHERE user_id=?", (user_id,))

    def workspace_owner(self, email: str) -> str:
        with self.database.connection() as conn:
            return "firm:" + self._membership(conn, email)["firm_id"]

    def require_role(self, email: str, *roles: str) -> dict[str, Any]:
        if not roles or any(role not in ROLES for role in roles):
            raise ValueError("at least one valid role is required")
        with self.database.connection() as conn:
            member = self._membership(conn, email, set(roles))
            return self._public(conn, member["user_id"])

    def add_client(self, owner_email: str, client_name: str, tenant_id: str, key_id: str, secret: str) -> dict[str, Any]:
        with self.database.transaction() as conn:
            member = self._membership(conn, owner_email, {"admin", "reviewer"})
            client_id = secrets.token_hex(8)
            row = {"id": client_id, "owner_email": normalize_email(owner_email), "firm_id": member["firm_id"],
                   "name": client_name, "tenant_id": tenant_id, "key_id": key_id, "secret": secret, "created_at": self.clock()}
            conn.execute("INSERT INTO clients VALUES (?,?,?)", (client_id, member["firm_id"], self.database.encrypt(row, "client:" + client_id)))
            return self._public_client(row)

    @staticmethod
    def _public_client(row: dict[str, Any]) -> dict[str, Any]:
        return {key: row[key] for key in ("id", "name", "tenant_id", "created_at")}

    def list_clients(self, owner_email: str) -> list[dict[str, Any]]:
        with self.database.connection() as conn:
            member = self._membership(conn, owner_email)
            rows = [self._public_client(self.database.decrypt(row["payload"], "client:" + row["client_id"]))
                    for row in conn.execute("SELECT client_id,payload FROM clients WHERE firm_id=?", (member["firm_id"],))]
            return sorted(rows, key=lambda row: row["created_at"])

    def get_client(self, owner_email: str, client_id: str) -> Optional[dict[str, Any]]:
        """Private provider credentials for the service; never expose this as JSON."""
        with self.database.connection() as conn:
            member = self._membership(conn, owner_email)
            row = conn.execute("SELECT payload FROM clients WHERE client_id=? AND firm_id=?", (client_id, member["firm_id"])).fetchone()
            return self.database.decrypt(row[0], "client:" + client_id) if row else None

    def remove_client(self, owner_email: str, client_id: str) -> Optional[dict[str, Any]]:
        with self.database.transaction() as conn:
            member = self._membership(conn, owner_email, {"admin"})
            row = conn.execute("SELECT payload FROM clients WHERE client_id=? AND firm_id=?", (client_id, member["firm_id"])).fetchone()
            if not row:
                return None
            conn.execute("DELETE FROM clients WHERE client_id=?", (client_id,))
            return self.database.decrypt(row[0], "client:" + client_id)

    def invite_member(self, actor: str, email: str, role: str = "reviewer") -> dict[str, Any]:
        if role not in ROLES:
            raise ValueError("invalid firm role")
        email = normalize_email(email)
        user_id = self.database.identity_id(email)
        token = secrets.token_urlsafe(32)
        expires_at = self.clock() + self.invitation_ttl
        with self.database.transaction() as conn:
            member = self._membership(conn, actor, {"admin"})
            current = conn.execute("SELECT * FROM memberships WHERE user_id=?", (user_id,)).fetchone()
            if current and current["firm_id"] == member["firm_id"] and current["active"]:
                raise ValueError("user is already a firm member")
            conn.execute("UPDATE invitations SET revoked=1 WHERE user_id=? AND firm_id=? AND used=0", (user_id, member["firm_id"]))
            digest = self._token_hash(token)
            payload = {"email": email, "role": role, "invited_by": normalize_email(actor), "created_at": self.clock()}
            conn.execute("INSERT INTO invitations VALUES (?,?,?,?,?,0,0)", (digest, member["firm_id"], user_id, self.database.encrypt(payload, "invite:" + digest), expires_at))
        return {"token": token, "expires_at": expires_at, "email": email, "role": role}

    def accept_invite(self, email: str, token: str) -> dict[str, Any]:
        email = normalize_email(email)
        user_id = self.database.identity_id(email)
        digest = self._token_hash(token)
        with self.database.transaction() as conn:
            member = self._membership(conn, email)
            row = conn.execute("SELECT * FROM invitations WHERE token_hash=? AND user_id=? AND used=0 AND revoked=0 AND expires_at>?", (digest, user_id, self.clock())).fetchone()
            if not row:
                raise ValueError("invitation is invalid or expired")
            invite = self.database.decrypt(row["payload"], "invite:" + digest)
            inviter = self._membership(conn, invite["invited_by"], {"admin"})
            if inviter["firm_id"] != row["firm_id"]:
                raise ValueError("invitation is no longer valid")
            if member["firm_id"] != row["firm_id"]:
                if conn.execute("SELECT 1 FROM clients WHERE firm_id=? LIMIT 1", (member["firm_id"],)).fetchone():
                    raise ValueError("offboard existing matters before joining another firm")
                other_members = conn.execute("SELECT COUNT(*) FROM memberships WHERE firm_id=? AND user_id<>? AND active=1", (member["firm_id"], user_id)).fetchone()[0]
                if other_members:
                    self._guard_last_admin(conn, member, invite["role"], False)
            conn.execute("UPDATE memberships SET firm_id=?,role=?,active=1 WHERE user_id=?", (row["firm_id"], invite["role"], user_id))
            conn.execute("UPDATE invitations SET used=1 WHERE token_hash=?", (digest,))
            # Revoke sessions issued for the previous workspace. Caller signs in
            # again so no cached client view is authorized for a different firm.
            conn.execute("UPDATE sessions SET revoked=1 WHERE user_id=?", (user_id,))
            return self._public(conn, user_id)

    @staticmethod
    def _guard_last_admin(conn, member, new_role: str, active: bool) -> None:
        if member["role"] == "admin" and member["active"] and (new_role != "admin" or not active):
            count = conn.execute("SELECT COUNT(*) FROM memberships WHERE firm_id=? AND role='admin' AND active=1", (member["firm_id"],)).fetchone()[0]
            if count <= 1:
                raise ValueError("firm must retain at least one active admin")

    def set_role(self, actor: str, email: str, role: str, active: bool = True) -> dict[str, Any]:
        if role not in ROLES or not isinstance(active, bool):
            raise ValueError("invalid firm role or membership state")
        user_id = self.database.identity_id(normalize_email(email))
        with self.database.transaction() as conn:
            admin = self._membership(conn, actor, {"admin"})
            member = conn.execute("SELECT * FROM memberships WHERE user_id=? AND firm_id=?", (user_id, admin["firm_id"])).fetchone()
            if not member:
                raise KeyError("firm member not found")
            self._guard_last_admin(conn, member, role, active)
            conn.execute("UPDATE memberships SET role=?,active=? WHERE user_id=?", (role, int(active), user_id))
            conn.execute("UPDATE sessions SET revoked=1 WHERE user_id=?", (user_id,))
            return {"email": normalize_email(email), "role": role, "active": active, "firm_id": admin["firm_id"]}

    def remove_member(self, actor: str, email: str) -> dict[str, Any]:
        with self.database.connection() as conn:
            admin = self._membership(conn, actor, {"admin"})
            row = conn.execute("SELECT role FROM memberships WHERE user_id=? AND firm_id=?", (self.database.identity_id(normalize_email(email)), admin["firm_id"])).fetchone()
            if not row:
                raise KeyError("firm member not found")
            role = row[0]
        return self.set_role(actor, email, role, active=False)

    def revoke_invite(self, actor: str, token: str) -> bool:
        with self.database.transaction() as conn:
            member = self._membership(conn, actor, {"admin"})
            return bool(conn.execute("UPDATE invitations SET revoked=1 WHERE token_hash=? AND firm_id=? AND used=0 AND revoked=0", (self._token_hash(token), member["firm_id"])).rowcount)

    def list_members(self, email: str) -> dict[str, list[dict[str, Any]]]:
        with self.database.connection() as conn:
            member = self._membership(conn, email)
            members = []
            for row in conn.execute("SELECT * FROM memberships WHERE firm_id=?", (member["firm_id"],)):
                user = self._user(conn, row["user_id"])
                members.append({"email": user["email"], "role": row["role"], "active": bool(row["active"]), "created_at": user.get("created_at")})
            invitations = []
            if member["role"] == "admin":
                for row in conn.execute("SELECT * FROM invitations WHERE firm_id=? AND revoked=0 AND used=0 AND expires_at>?", (member["firm_id"], self.clock())):
                    invite = self.database.decrypt(row["payload"], "invite:" + row["token_hash"])
                    invitations.append({**invite, "expires_at": row["expires_at"]})
            return {"members": sorted(members, key=lambda row: row["email"]), "invitations": invitations}


FirmStore = SecureFirmStore
