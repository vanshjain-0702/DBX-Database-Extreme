from typing import Any, Dict, List, Optional, Tuple, Union, cast
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import json

import redis


class DBXError(Exception):
    """RESP or control-plane failure with the engine's message, not a stack dump."""


def _vector_flags(
    *,
    min_score: Optional[float] = None,
    ef: Optional[int] = None,
    space: Optional[str] = None,
    with_docs: Optional[str] = None,
    filter_contains: Optional[str] = None,
) -> List[Union[str, int, float]]:
    flags: List[Union[str, int, float]] = []
    if with_docs:
        flags.extend(["WITHDOCS", with_docs])
    if filter_contains:
        flags.extend(["FILTER_CONTAINS", filter_contains])
    if min_score is not None:
        flags.extend(["MIN_SCORE", min_score])
    if ef is not None:
        flags.extend(["EF", ef])
    if space:
        flags.extend(["SPACE", space])
    return flags


def _parse_vector_hits(res: Any) -> List[Tuple[str, float]]:
    results: List[Tuple[str, float]] = []
    if not res:
        return results
    for item in res:
        if isinstance(item, (list, tuple)) and len(item) >= 2:
            results.append((str(item[0]), float(item[1])))
    return results


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

    def vadd(
        self,
        index_name: str,
        doc_id: str,
        vector: List[float],
        *,
        space: Optional[str] = None,
    ) -> bool:
        args: List[Union[str, float]] = [index_name, doc_id]
        if space:
            args.extend(["SPACE", space])
        args.extend(vector)
        try:
            res = self.r.execute_command("VADD", *args)
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
        self,
        index_name: str,
        query_vector: List[float],
        top_k: int = 4,
        *,
        min_score: Optional[float] = None,
        ef: Optional[int] = None,
        space: Optional[str] = None,
        with_docs: Optional[str] = None,
        filter_contains: Optional[str] = None,
    ) -> List[Tuple[str, float]]:
        args: List[Union[str, int, float]] = [index_name, *query_vector, top_k]
        args.extend(
            _vector_flags(
                min_score=min_score,
                ef=ef,
                space=space,
                with_docs=with_docs,
                filter_contains=filter_contains,
            )
        )
        try:
            res = self.r.execute_command("VSEARCH", *args)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return _parse_vector_hits(res)

    def vsim(
        self,
        index_name: str,
        doc_id: str,
        top_k: int = 4,
        *,
        min_score: Optional[float] = None,
        ef: Optional[int] = None,
        space: Optional[str] = None,
        with_docs: Optional[str] = None,
        filter_contains: Optional[str] = None,
    ) -> List[Tuple[str, float]]:
        args: List[Union[str, int, float]] = [index_name, doc_id, top_k]
        args.extend(
            _vector_flags(
                min_score=min_score,
                ef=ef,
                space=space,
                with_docs=with_docs,
                filter_contains=filter_contains,
            )
        )
        try:
            res = self.r.execute_command("VSIM", *args)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return _parse_vector_hits(res)

    def vfuse(
        self,
        index_name: str,
        queries: Dict[str, List[float]],
        top_k: int = 4,
        *,
        weights: Optional[List[float]] = None,
        min_score: Optional[float] = None,
        ef: Optional[int] = None,
        with_docs: Optional[str] = None,
        filter_contains: Optional[str] = None,
    ) -> List[Tuple[str, float]]:
        if len(queries) < 2:
            raise ValueError("vfuse requires at least two named spaces")
        args: List[Union[str, int, float]] = [index_name]
        ordered_spaces = list(queries.items())
        for space, vector in ordered_spaces:
            args.extend(["SPACE", space, *vector])
        args.append(top_k)
        if weights is not None:
            args.extend(["WEIGHTS", ",".join(str(w) for w in weights)])
        args.extend(
            _vector_flags(
                min_score=min_score,
                ef=ef,
                with_docs=with_docs,
                filter_contains=filter_contains,
            )
        )
        try:
            res = self.r.execute_command("VFUSE", *args)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return _parse_vector_hits(res)

    def vdel(
        self, index_name: str, doc_id: str, *, space: Optional[str] = None
    ) -> bool:
        args: List[str] = [index_name, doc_id]
        if space:
            args.extend(["SPACE", space])
        try:
            res = self.r.execute_command("VDEL", *args)
        except redis.RedisError as exc:
            raise DBXError(str(exc)) from exc
        return bool(res)


class TenantMemory:
    """One customer's working state and recall.

    This is the product surface: remember a fact (optional embedding),
    recall by vector, forget one key, or shred the tenant through the
    control plane. It is not a generic Redis wrapper.
    """

    def __init__(self, client: DBXClient, index: str = "memory") -> None:
        self.client = client
        self.index = index

    @classmethod
    def open(
        cls,
        plane: "ControlPlane",
        tenant_id: str,
        name: str = "",
        host: str = "127.0.0.1",
        port: int = 6380,
        index: str = "memory",
    ) -> "TenantMemory":
        try:
            plane.provision(tenant_id, name or tenant_id)
        except DBXError as exc:
            text = str(exc).lower()
            if "already" not in text and "exist" not in text:
                raise
        minted = plane.create_key(tenant_id, name="memory-writer", role="writer")
        key = minted.get("key") or {}
        client = DBXClient(
            host=host,
            port=port,
            tenant=tenant_id,
            key_id=str(key.get("id") or minted.get("id") or ""),
            secret=str(minted.get("secret") or ""),
        )
        return cls(client, index=index)

    def remember(
        self,
        key: str,
        value: str,
        vector: Optional[List[float]] = None,
        *,
        space: Optional[str] = None,
    ) -> None:
        if not self.client.set(key, value):
            raise DBXError("SET failed")
        if vector is not None:
            if not self.client.vadd(self.index, key, vector, space=space):
                raise DBXError("VADD failed")

    def recall(
        self,
        vector: List[float],
        top_k: int = 4,
        *,
        min_score: Optional[float] = None,
        space: Optional[str] = None,
    ) -> List[Tuple[str, str, float]]:
        hits = self.client.vsearch(
            self.index, vector, top_k, min_score=min_score, space=space
        )
        out: List[Tuple[str, str, float]] = []
        for doc_id, score in hits:
            stored = self.client.get(doc_id)
            out.append((doc_id, stored or "", score))
        return out

    def similar(
        self,
        item_id: str,
        top_k: int = 4,
        *,
        min_score: Optional[float] = None,
        space: Optional[str] = None,
    ) -> List[Tuple[str, str, float]]:
        hits = self.client.vsim(
            self.index, item_id, top_k, min_score=min_score, space=space
        )
        out: List[Tuple[str, str, float]] = []
        for doc_id, score in hits:
            stored = self.client.get(doc_id)
            out.append((doc_id, stored or "", score))
        return out

    def fuse(
        self,
        queries: Dict[str, List[float]],
        top_k: int = 4,
        *,
        weights: Optional[List[float]] = None,
        min_score: Optional[float] = None,
    ) -> List[Tuple[str, str, float]]:
        hits = self.client.vfuse(
            self.index, queries, top_k, weights=weights, min_score=min_score
        )
        out: List[Tuple[str, str, float]] = []
        for doc_id, score in hits:
            stored = self.client.get(doc_id)
            out.append((doc_id, stored or "", score))
        return out

    def get(self, key: str) -> Optional[str]:
        return self.client.get(key)

    def forget(self, key: str) -> None:
        self.client.delete(key)
        self.client.vdel(self.index, key)


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

    def shred(self, tenant_id: str) -> Dict[str, Any]:
        """Purge one tenant and shred its wrapped DEK. O(1) at-rest deletion."""
        return self.delete(tenant_id, purge=True)

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
