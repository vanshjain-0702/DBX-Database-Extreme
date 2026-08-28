from typing import Any, Dict, List, Optional, Tuple, Union, cast
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import json

import redis


class DBXError(Exception):
    """RESP or control-plane failure with the engine's message, not a stack dump."""


class DBXClient:
    """RESP client for the public DBX ingress.

    The first command on :6380 must be AUTH tenantID:keyID secret.
    Point this at the orchestrator ingress, not a loopback tenant port.
    """

    def __init__(
        self,
        host: str = "localhost",
        port: int = 6380,
        tenant: Optional[str] = None,
        key_id: Optional[str] = None,
        secret: Optional[str] = None,
        password: Optional[str] = None,
        username: Optional[str] = None,
        ca_cert: Optional[str] = None,
        client_cert: Optional[str] = None,
        client_key: Optional[str] = None,
        ssl_check_hostname: bool = True,
    ) -> None:
        if tenant and key_id:
            username = f"{tenant}:{key_id}"
            if secret:
                password = secret
        use_ssl = bool(ca_cert and client_cert and client_key)
        self.r = redis.Redis(
            host=host,
            port=port,
            username=username,
            password=password,
            decode_responses=True,
            protocol=2,
            ssl=use_ssl,
            ssl_ca_certs=ca_cert if use_ssl else None,
            ssl_certfile=client_cert if use_ssl else None,
            ssl_keyfile=client_key if use_ssl else None,
            ssl_check_hostname=ssl_check_hostname if use_ssl else False,
        )

    def ping(self) -> bool:
        try:
            return bool(self.r.ping())
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc

    def set(self, key: str, val: str, ex: Optional[int] = None) -> bool:
        try:
            return bool(self.r.set(key, val, ex=ex))
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc

    def get(self, key: str) -> Optional[str]:
        value = self.r.get(key)
        if value is None:
            return None
        if isinstance(value, bytes):
            return value.decode("utf-8")
        return str(value)

    def delete(self, *keys: str) -> int:
        return int(self.r.delete(*keys) or 0)

    def vadd(self, index_name: str, doc_id: str, vector: List[float]) -> bool:
        try:
            res = self.r.execute_command("VADD", index_name, doc_id, *vector)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return res == 1

    def vadd_batch(
        self, index_name: str, dim: int, doc_ids: List[str], vectors: List[List[float]]
    ) -> int:
        if len(doc_ids) != len(vectors):
            raise ValueError("doc_ids and vectors must be the same length")
        args: List[Union[str, int, float]] = [index_name, dim]
        for i, doc_id in enumerate(doc_ids):
            args.append(doc_id)
            args.extend(vectors[i])
        try:
            res = self.r.execute_command("VADD_BATCH", *args)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return int(cast(Union[int, str], res or 0))

    def vsearch(
        self, index_name: str, query_vector: List[float], top_k: int = 4
    ) -> List[Tuple[str, float]]:
        try:
            res = self.r.execute_command("VSEARCH", index_name, *query_vector, top_k)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        results: List[Tuple[str, float]] = []
        if not res:
            return results
        for item in res:
            if isinstance(item, (list, tuple)) and len(item) >= 2:
                results.append((str(item[0]), float(item[1])))
        return results

    def vdel(self, index_name: str, doc_id: str) -> bool:
        try:
            res = self.r.execute_command("VDEL", index_name, doc_id)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return bool(res)


class ControlPlane:
    """Operator HTTP client for lifecycle, usage, backup, and hibernate."""

    def __init__(self, base: str = "http://127.0.0.1:8000", token: str = "") -> None:
        self.base = base.rstrip("/")
        self.token = token

    def login(self, username: str, password: str) -> str:
        body = self._json(
            "POST",
            "/api/login",
            {"username": username, "password": password},
            auth=False,
        )
        self.token = str(body["token"])
        return self.token

    def provision(self, tenant_id: str, name: str = "") -> Dict[str, Any]:
        return self._json(
            "POST", "/api/provision", {"id": tenant_id, "name": name or tenant_id}
        )

    def create_key(
        self, tenant_id: str, name: str = "writer", role: str = "writer"
    ) -> Dict[str, Any]:
        return self._json(
            "POST",
            f"/api/v1/tenants/{tenant_id}/keys",
            {"name": name, "role": role},
        )

    def usage(self, tenant_id: str) -> Dict[str, Any]:
        return self._json("GET", f"/api/v1/tenants/{tenant_id}/usage")

    def list_usage(self) -> Any:
        return self._json("GET", "/api/usage")

    def backup(self, tenant_id: str) -> Dict[str, Any]:
        return self._json("POST", "/api/tenants/backup", {"id": tenant_id})

    def restore(self, tenant_id: str, path: str) -> Dict[str, Any]:
        return self._json(
            "POST", "/api/tenants/restore", {"id": tenant_id, "path": path}
        )

    def export_tenant(self, tenant_id: str) -> Dict[str, Any]:
        return self._json("POST", "/api/tenants/export", {"id": tenant_id})

    def import_tenant(self, tenant_id: str, path: str) -> Dict[str, Any]:
        return self._json(
            "POST", "/api/tenants/import", {"id": tenant_id, "path": path}
        )

    def hibernate(self, tenant_id: str) -> Dict[str, Any]:
        return self._json("POST", f"/api/v1/tenants/{tenant_id}/hibernate")

    def wake(self, tenant_id: str) -> Dict[str, Any]:
        return self._json("POST", f"/api/v1/tenants/{tenant_id}/wake")

    def delete(self, tenant_id: str, purge: bool = True) -> Dict[str, Any]:
        return self._json(
            "POST", "/api/tenants/delete", {"id": tenant_id, "purge": purge}
        )

    def _json(
        self,
        method: str,
        path: str,
        payload: Optional[Dict[str, Any]] = None,
        auth: bool = True,
    ) -> Any:
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        headers = {"Content-Type": "application/json"}
        if auth:
            if not self.token:
                raise DBXError("control plane token missing — call login() first")
            headers["Authorization"] = "Bearer " + self.token
        req = Request(self.base + path, data=data, headers=headers, method=method)
        try:
            with urlopen(req, timeout=30) as resp:
                raw = resp.read()
        except HTTPError as exc:
            detail = exc.read().decode("utf-8", "replace")
            raise DBXError(f"{exc.code} {path}: {detail}") from exc
        except URLError as exc:
            raise DBXError(
                f"cannot reach control plane {self.base}: {exc.reason}"
            ) from exc
        if not raw:
            return {}
        return json.loads(raw.decode("utf-8"))
