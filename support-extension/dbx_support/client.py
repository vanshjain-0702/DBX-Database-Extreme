"""Small stdlib-only client for DBX's tenant-scoped support API."""

from __future__ import annotations

import json
import math
import ssl
from dataclasses import dataclass, field
from http.client import HTTPException
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit, urlunsplit
from urllib.parse import quote
from urllib.request import HTTPRedirectHandler, HTTPSHandler, Request, build_opener

from .schema import TENANT_FIELDS, USAGE_FIELDS, safe_tenant_id, snapshot_problem


class SupportConnectionError(RuntimeError):
    """A safe-to-display connection or API error."""

    def __init__(self, message: str, *, retryable: bool = False) -> None:
        super().__init__(message)
        self.retryable = retryable


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # A dedicated support capability must never follow a redirect to an
        # operator endpoint or a different origin.
        return None


def _json_object(pairs: list[tuple[str, object]]) -> dict:
    result: dict = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def _reject_constant(_value: str) -> None:
    raise ValueError("non-finite JSON number")


@dataclass
class DBXSupportClient:
    base_url: str
    timeout: float = 10.0
    token: str = field(default="", repr=False, compare=False)
    support_read_token: str = field(default="", repr=False, compare=False)
    support_mode: bool = False
    support_wake_token: str = field(default="", repr=False, compare=False)

    def __post_init__(self) -> None:
        if any(ord(char) <= 32 or ord(char) == 127 for char in self.base_url):
            raise SupportConnectionError("DBX URL contains invalid whitespace or control characters")
        try:
            parsed = urlsplit(self.base_url)
            _ = parsed.port
        except ValueError:
            raise SupportConnectionError("DBX URL has an invalid host or port") from None
        if parsed.scheme not in ("http", "https") or not parsed.hostname:
            raise SupportConnectionError("DBX URL must start with http:// or https://")
        if parsed.username is not None or parsed.password is not None:
            raise SupportConnectionError("Do not put credentials in the DBX URL")
        if parsed.path not in ("", "/") or parsed.query or parsed.fragment:
            raise SupportConnectionError("DBX URL must be the control-plane origin only")
        if parsed.scheme == "http" and parsed.hostname not in ("localhost", "127.0.0.1", "::1"):
            raise SupportConnectionError(
                "Plain HTTP is allowed only for loopback DBX; use HTTPS for remote nodes"
            )
        self.base_url = urlunsplit((parsed.scheme, parsed.netloc, "", "", ""))
        if not math.isfinite(self.timeout) or self.timeout <= 0:
            raise SupportConnectionError("DBX request timeout must be finite and positive")

    def login(self, username: str, password: str) -> None:
        response = self._request(
            "/api/login",
            method="POST",
            payload={"username": username, "password": password},
            authenticated=False,
        )
        token = response.get("token") if isinstance(response, dict) else None
        if not isinstance(token, str) or not token:
            raise SupportConnectionError("DBX login did not return an operator token")
        self.token = token

    def use_support_capability(self, read_token: str, wake_token: str = "") -> None:
        if len(read_token) < 32:
            raise SupportConnectionError("DBX support read capability must be at least 32 characters")
        if wake_token and len(wake_token) < 32:
            raise SupportConnectionError("DBX support wake capability must be at least 32 characters")
        if wake_token and read_token == wake_token:
            raise SupportConnectionError("DBX support read and wake capabilities must differ")
        if any(ord(char) <= 32 or ord(char) >= 127 for char in read_token + wake_token):
            raise SupportConnectionError("DBX support capabilities must be printable ASCII without whitespace")
        self.support_read_token = read_token
        self.support_wake_token = wake_token
        self.support_mode = True

    def snapshot(self) -> dict:
        if self.support_mode and not self.support_read_token:
            raise SupportConnectionError("Configure a DBX support read capability before requesting diagnostics")
        if not self.support_mode and not self.token:
            raise SupportConnectionError("Connect to DBX before requesting diagnostics")
        if self.support_mode:
            response = self._request("/api/support/v1/snapshot", bearer_token=self.support_read_token)
            if not isinstance(response, dict):
                raise SupportConnectionError("DBX returned an unexpected support snapshot")
            tenants = response
            usage = response
        else:
            tenants = self._request("/api/tenants")
            usage = self._request("/api/usage")
        snapshot = {
            "tenants": self._list_response(tenants, "tenants"),
            "usage": self._list_response(usage, "usage"),
        }
        if snapshot_problem(snapshot):
            raise SupportConnectionError("DBX returned malformed support telemetry; recovery is held")
        return snapshot

    def wake_tenant(self, tenant_id: str) -> object:
        """Wake exactly one tenant using the dedicated support capability.

        Callers must apply the support repair policy before using this method.
        This method deliberately exposes no generic path or mutation helper.
        """
        if not safe_tenant_id(tenant_id):
            raise SupportConnectionError("Tenant ID is not safe for a scoped wake request")
        if not self.support_mode or not self.support_wake_token:
            raise SupportConnectionError("A separate DBX support wake capability is required")
        return self._request(
            f"/api/support/v1/tenants/{quote(tenant_id, safe='')}/wake",
            method="POST",
            bearer_token=self.support_wake_token,
        )

    def _request(
        self,
        path: str,
        *,
        method: str = "GET",
        payload: dict | None = None,
        authenticated: bool = True,
        bearer_token: str | None = None,
    ) -> object:
        headers = {"Accept": "application/json"}
        body = None
        if payload is not None:
            headers["Content-Type"] = "application/json"
            body = json.dumps(payload).encode("utf-8")
        if authenticated:
            token = bearer_token if bearer_token is not None else self.token
            if not token:
                raise SupportConnectionError("DBX session is not authenticated")
            headers["Authorization"] = f"Bearer {token}"
        request = Request(self.base_url + path, data=body, headers=headers, method=method)
        try:
            # urllib's default TLS context verifies the server certificate.
            opener = build_opener(_NoRedirect(), HTTPSHandler(context=ssl.create_default_context()))
            with opener.open(request, timeout=self.timeout) as response:
                length_header = response.headers.get("Content-Length")
                expected_length = None
                if length_header is not None:
                    try:
                        expected_length = int(length_header)
                    except ValueError:
                        raise SupportConnectionError("DBX returned an invalid response length") from None
                    if not 0 <= expected_length <= 2 * 1024 * 1024:
                        raise SupportConnectionError("DBX response exceeded the 2 MiB safety limit")
                raw = response.read(2 * 1024 * 1024 + 1)
                if expected_length is not None and len(raw) != expected_length:
                    raise SupportConnectionError("DBX response was truncated", retryable=True)
        except HTTPError as exc:
            exc.close()
            if exc.code in (401, 403):
                message = "DBX rejected the support capability or tenant scope"
            else:
                message = f"DBX control API returned HTTP {exc.code} for {path}"
            raise SupportConnectionError(
                message,
                retryable=exc.code in (408, 425, 429, 500, 502, 503, 504),
            ) from None
        except (URLError, TimeoutError, OSError, HTTPException) as exc:
            # Do not include request headers, token, or password in diagnostics.
            reason = getattr(exc, "reason", None)
            detail = type(reason or exc).__name__
            retryable = not isinstance(reason or exc, ssl.SSLError)
            raise SupportConnectionError(
                f"Cannot reach DBX control API ({detail})", retryable=retryable
            ) from None
        if len(raw) > 2 * 1024 * 1024:
            raise SupportConnectionError(f"DBX response from {path} exceeded the 2 MiB safety limit")
        try:
            return json.loads(raw.decode("utf-8"), object_pairs_hook=_json_object,
                              parse_constant=_reject_constant)
        except (UnicodeDecodeError, ValueError, RecursionError):
            raise SupportConnectionError(f"DBX returned invalid JSON for {path}") from None

    @staticmethod
    def _list_response(payload: object, name: str) -> list[dict]:
        if isinstance(payload, list):
            rows = payload
        elif isinstance(payload, dict) and isinstance(payload.get(name), list):
            rows = payload[name]
        else:
            raise SupportConnectionError(f"DBX returned an unexpected {name} response")
        if any(not isinstance(row, dict) for row in rows):
            raise SupportConnectionError(f"DBX returned an invalid row in {name}")
        allowed = TENANT_FIELDS if name == "tenants" else USAGE_FIELDS
        # Project onto a reviewed schema. New DBX API fields cannot silently
        # flow into specialists, learning state, or diagnostic output.
        return [{key: value for key, value in row.items() if key in allowed} for row in rows]
