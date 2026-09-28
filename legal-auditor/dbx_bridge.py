"""Bridge from Legal Auditor to DBX control plane + RESP tenant memory."""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

# Prefer installed package; fall back to repo sdk/python for local runs.
_SDK = Path(__file__).resolve().parents[1] / "sdk" / "python"
if _SDK.is_dir() and str(_SDK) not in sys.path:
    sys.path.insert(0, str(_SDK))

from dbx import ControlPlane, DBXClient, DBXError, TenantMemory  # noqa: E402

from embeddings import DEFAULT_DIM, chunk_text, embed_query, embed_texts

INDEX = "legal_docs"


def _slug(name: str) -> str:
    s = re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")
    return (s or "client")[:40]


class DBXBridge:
    def __init__(
        self,
        control_url: Optional[str] = None,
        resp_host: str = "127.0.0.1",
        resp_port: int = 6380,
    ) -> None:
        self.control_url = control_url or os.environ.get(
            "DBX_CONTROL_URL", "http://127.0.0.1:8000"
        )
        self.resp_host = os.environ.get("DBX_RESP_HOST", resp_host)
        self.resp_port = int(os.environ.get("DBX_RESP_PORT", str(resp_port)))
        self.plane = ControlPlane(self.control_url)
        self._logged_in = False

    def ensure_login(self) -> None:
        if self._logged_in and self.plane.token:
            return
        user = os.environ.get("DBX_ADMIN_USER", "admin")
        password = os.environ.get("DBX_ADMIN_PASSWORD", "adminadminadmin")
        try:
            self.plane.login(user, password)
        except DBXError as exc:
            raise RuntimeError(
                f"cannot reach DBX control plane at {self.control_url}: {exc}. "
                "Start the node with `make run-dev` or `make docker-up`."
            ) from exc
        self._logged_in = True

    def provision_client(self, client_name: str) -> Dict[str, str]:
        self.ensure_login()
        tenant_id = f"la-{_slug(client_name)}"
        try:
            self.plane.provision(tenant_id, name=f"Legal Auditor: {client_name}")
        except DBXError as exc:
            text = str(exc).lower()
            if "already" not in text and "exist" not in text:
                raise
        minted = self.plane.create_key(tenant_id, name="legal-writer", role="writer")
        key = minted.get("key") or {}
        key_id = str(key.get("id") or minted.get("id") or "")
        secret = str(minted.get("secret") or "")
        if not key_id or not secret:
            raise RuntimeError(f"key mint failed for {tenant_id}: {minted}")
        return {"tenant_id": tenant_id, "key_id": key_id, "secret": secret}

    def memory_for(self, tenant_id: str, key_id: str, secret: str) -> TenantMemory:
        client = DBXClient(
            host=self.resp_host,
            port=self.resp_port,
            tenant=tenant_id,
            key_id=key_id,
            secret=secret,
        )
        return TenantMemory(client, index=INDEX)

    def ingest_document(
        self,
        tenant_id: str,
        key_id: str,
        secret: str,
        doc_name: str,
        text: str,
    ) -> Dict[str, Any]:
        mem = self.memory_for(tenant_id, key_id, secret)
        chunks = chunk_text(text)
        if not chunks:
            raise ValueError("document is empty")
        vectors = embed_texts(chunks, dim=DEFAULT_DIM)
        doc_slug = _slug(doc_name) or "doc"
        ids: List[str] = []
        for i, (chunk, vector) in enumerate(zip(chunks, vectors)):
            chunk_id = f"doc:{doc_slug}:{i}"
            mem.remember(chunk_id, chunk, vector=vector)
            ids.append(chunk_id)
        meta_key = f"meta:doc:{doc_slug}"
        mem.client.set(meta_key, f"{doc_name}|chunks={len(ids)}")
        return {
            "doc_name": doc_name,
            "chunk_count": len(ids),
            "chunk_ids": ids,
            "dim": DEFAULT_DIM,
        }

    def audit(
        self,
        tenant_id: str,
        key_id: str,
        secret: str,
        query: str,
        top_k: int = 5,
    ) -> List[Dict[str, Any]]:
        mem = self.memory_for(tenant_id, key_id, secret)
        vector = embed_query(query, dim=DEFAULT_DIM)
        hits = mem.recall(vector, top_k=top_k)
        return [
            {"id": doc_id, "text": text, "score": score} for doc_id, text, score in hits
        ]

    def export_client(self, tenant_id: str) -> Dict[str, Any]:
        self.ensure_login()
        return self.plane.export_tenant(tenant_id)

    def hibernate_client(self, tenant_id: str) -> Dict[str, Any]:
        self.ensure_login()
        return self.plane.hibernate(tenant_id)

    def wake_client(self, tenant_id: str) -> Dict[str, Any]:
        self.ensure_login()
        return self.plane.wake(tenant_id)

    def delete_client(self, tenant_id: str, purge: bool = True) -> Dict[str, Any]:
        self.ensure_login()
        return self.plane.delete(tenant_id, purge=purge)

    def usage(self, tenant_id: str) -> Dict[str, Any]:
        self.ensure_login()
        return self.plane.usage(tenant_id)
