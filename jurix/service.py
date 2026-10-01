"""Jurix operations on the Legal Auditor firm registry and a live DBX node."""

from __future__ import annotations

import os
import re
import secrets
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

ROOT = Path(__file__).resolve().parents[1]
LA = ROOT / "legal-auditor"
if str(LA) not in sys.path:
    sys.path.insert(0, str(LA))
os.environ.setdefault(
    "LA_STORE_PATH", str(Path(__file__).resolve().parent / "data" / "firm_store.json")
)

from dbx_bridge import DBXBridge  # noqa: E402
from embeddings import chunk_text  # noqa: E402
from store import FirmStore  # noqa: E402

from embed import embed  # noqa: E402
from playbook import PLAYBOOK, PLAYBOOK_NAME, evaluate, overlap, tally  # noqa: E402
from records import Records  # noqa: E402
from samples import SAMPLES  # noqa: E402

INDEX = "legal_docs"


def _clause_chunks(text: str) -> List[str]:
    """Keep a section marker with its sentence. Long paragraphs still window."""
    parts = [p.strip() for p in re.split(r"\n\s*\n", text) if p.strip()]
    if not parts:
        return chunk_text(text, chunk_size=420, overlap=50)
    out: List[str] = []
    for part in parts:
        if len(part) <= 900:
            out.append(part)
        else:
            out.extend(chunk_text(part, chunk_size=700, overlap=80))
    return out


def display_name(email: str) -> str:
    local = email.split("@", 1)[0]
    parts = [p for p in re.split(r"[._\-+]+", local) if p]
    if len(parts) >= 2:
        return f"{parts[0][:1].upper()}. {parts[-1].capitalize()}"
    if not local:
        return "Reviewer"
    return local[:1].upper() + local[1:]


def initials(email: str) -> str:
    name = display_name(email)
    letters = [w[0] for w in re.split(r"\s+", name) if w and w[0].isalpha()]
    return ("".join(letters)[:2] or "JX").upper()


def firm_code(firm_name: str) -> str:
    letters = re.sub(r"[^A-Za-z]", "", firm_name).upper()
    prefix = (letters[:2] or "JX")
    return f"{prefix}-{secrets.token_hex(2).upper()}"


def _public_matter(client: Dict[str, Any], meta: Dict[str, Any], docs: List[Dict[str, Any]], run: Optional[Dict[str, Any]]) -> Dict[str, Any]:
    items = (run or {}).get("items") or []
    counts = tally(items) if items else {"high": 0, "medium": 0, "clear": 0, "missing": 0, "open": 0}
    reviewer = meta.get("reviewer") or client.get("owner_email") or ""
    chunk_count = sum(int(d.get("chunk_count") or 0) for d in docs)
    latest_doc = max((d.get("ingested_at") or 0) for d in docs) if docs else 0
    ran_at = (run or {}).get("ran_at") or 0
    if meta.get("hibernated"):
        status = "hibernated"
    elif not run:
        status = "not_started"
    elif counts["open"] == 0 and items:
        status = "cleared"
    else:
        status = "in_review"
    return {
        "id": client["id"],
        "name": client["name"],
        "matter": meta.get("matter") or "Matter",
        "tenant_id": client["tenant_id"],
        "reviewer_email": reviewer,
        "reviewer_name": display_name(reviewer) if reviewer else "",
        "hibernated": bool(meta.get("hibernated")),
        "doc_count": len(docs),
        "chunk_count": chunk_count,
        "created_at": client.get("created_at"),
        "has_run": bool(run),
        "ran_at": ran_at or None,
        "stale": bool(run) and latest_doc > ran_at,
        "status": status,
        "high": counts["high"],
        "missing": counts["missing"],
        "medium": counts["medium"],
        "clear": counts["clear"],
        "open": counts["open"],
        "sibling_hits": (run or {}).get("sibling_hits", 0),
        "siblings_probed": (run or {}).get("siblings_probed", 0),
    }


class JurixService:
    def __init__(self, store: Optional[FirmStore] = None, records: Optional[Records] = None, bridge: Optional[DBXBridge] = None) -> None:
        self.store = store or FirmStore()
        self.records = records or Records()
        self.bridge = bridge or DBXBridge()

    def dbx_status(self) -> Dict[str, Any]:
        try:
            self.bridge.ensure_login()
        except Exception as exc:
            return {
                "ok": False,
                "control_url": self.bridge.control_url,
                "resp": f"{self.bridge.resp_host}:{self.bridge.resp_port}",
                "detail": str(exc),
            }
        return {
            "ok": True,
            "control_url": self.bridge.control_url,
            "resp": f"{self.bridge.resp_host}:{self.bridge.resp_port}",
            "detail": "",
        }

    def register(self, email: str, password: str, firm_name: str) -> Dict[str, Any]:
        user = self.store.register(email, password, firm_name)
        code = firm_code(user["firm_name"])
        self.records.set_firm(user["email"], user["firm_name"], code)
        token = self.store.login(email, password)
        return {"token": token, "user": self._user_view(user["email"])}

    def login(self, email: str, password: str, code: str = "") -> Dict[str, Any]:
        token = self.store.login(email, password)
        email_n = email.strip().lower()
        firm = self.records.firm(email_n)
        if code.strip():
            expected = (firm or {}).get("code") or ""
            if not expected or code.strip().upper() != expected.upper():
                raise ValueError("firm code does not match this account")
        if not firm:
            user = self.store.user_for_token(token) or {}
            firm = self.records.set_firm(
                email_n, user.get("firm_name") or email_n, firm_code(user.get("firm_name") or email_n)
            )
        return {"token": token, "user": self._user_view(email_n)}

    def user_view(self, email: str) -> Dict[str, Any]:
        return self._user_view(email)

    def _user_view(self, email: str) -> Dict[str, Any]:
        user = None
        # FirmStore keeps users private; read through a fresh token lookup is awkward.
        # The email is already authenticated by the caller.
        firm = self.records.firm(email) or {}
        stored = self.store.public_user(email) or {}
        firm_name = firm.get("firm_name") or stored.get("firm_name") or email
        return {
            "email": email,
            "firm_name": firm_name,
            "firm_code": firm.get("code") or "",
            "name": display_name(email),
            "initials": initials(email),
        }

    def list_matters(self, email: str) -> List[Dict[str, Any]]:
        rows = []
        for client in self.store.list_clients(email):
            full = self.store.get_client(email, client["id"]) or client
            meta = self.records.matter_meta(client["id"])
            docs = self.records.documents(client["id"])
            run = self.records.run(client["id"])
            rows.append(_public_matter(full, meta, docs, run))
        rows.sort(key=lambda r: (-(r["open"]), r["name"].lower()))
        return rows

    def create_matter(self, email: str, name: str, matter: str) -> Dict[str, Any]:
        creds = self.bridge.provision_client(name)
        client = self.store.add_client(
            email, name.strip(), creds["tenant_id"], creds["key_id"], creds["secret"]
        )
        self.records.init_matter(client["id"], matter.strip() or "Matter", email)
        full = self.store.get_client(email, client["id"]) or {}
        return _public_matter(full, self.records.matter_meta(client["id"]), [], None)

    def _require(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self.store.get_client(email, client_id)
        if not client:
            raise KeyError("matter not found")
        return client

    def detail(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        meta = self.records.matter_meta(client_id)
        docs = self.records.documents(client_id)
        run = self.records.run(client_id)
        public_docs = [
            {
                "id": d["id"],
                "name": d["name"],
                "chunk_count": d["chunk_count"],
                "ingested_at": d["ingested_at"],
            }
            for d in docs
        ]
        matter = _public_matter(client, meta, docs, run)
        matter["documents"] = public_docs
        matter["items"] = (run or {}).get("items") or []
        matter["notes"] = self.records.notes(client_id)
        matter["playbook"] = PLAYBOOK_NAME
        matter["read_path"] = f"dbx://{client['tenant_id']}/{INDEX}"
        return matter

    def ingest(self, email: str, client_id: str, doc_name: str, text: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        if self.records.matter_meta(client_id).get("hibernated"):
            raise RuntimeError("tenant is hibernated — wake it before indexing")
        chunks = _clause_chunks(text)
        if not chunks:
            raise ValueError("document is empty")
        mem = self.bridge.memory_for(client["tenant_id"], client["key_id"], client["secret"])
        doc_slug = re.sub(r"[^a-z0-9]+", "-", doc_name.lower()).strip("-") or "doc"
        suffix = secrets.token_hex(3)
        ids: List[str] = []
        for i, chunk in enumerate(chunks):
            chunk_id = f"doc:{doc_slug}:{suffix}:{i}"
            mem.remember(chunk_id, chunk, vector=embed(chunk))
            ids.append(chunk_id)
        mem.client.set(f"meta:doc:{doc_slug}", f"{doc_name}|chunks={len(ids)}")
        saved = self.records.add_document(client_id, doc_name, text, ids)
        saved["tenant_id"] = client["tenant_id"]
        saved["dim"] = 64
        return saved

    def ingest_sample(self, email: str, client_id: str, kind: str) -> Dict[str, Any]:
        sample = SAMPLES.get(kind)
        if not sample:
            raise ValueError("unknown sample")
        meta = self.records.matter_meta(client_id)
        if meta.get("matter") in ("", "Matter"):
            pass
        saved = self.ingest(email, client_id, sample["doc_name"], sample["text"])
        return saved

    def _chunks_from_dbx(self, client: Dict[str, Any]) -> List[Dict[str, Any]]:
        docs = self.records.documents(client["id"])
        if not docs:
            return []
        mem = self.bridge.memory_for(client["tenant_id"], client["key_id"], client["secret"])
        rows: List[Dict[str, Any]] = []
        for doc in docs:
            for index, chunk_id in enumerate(doc["chunk_ids"]):
                stored = mem.get(chunk_id)
                if stored is None:
                    raise RuntimeError(
                        f"DBX returned no value for {chunk_id} in {client['tenant_id']}"
                    )
                rows.append(
                    {
                        "id": chunk_id,
                        "text": stored,
                        "index": index,
                        "doc_name": doc["name"],
                        "doc_id": doc["id"],
                    }
                )
        return rows

    def _recall(self, client: Dict[str, Any], query: str, top_k: int = 4) -> List[Dict[str, Any]]:
        mem = self.bridge.memory_for(client["tenant_id"], client["key_id"], client["secret"])
        hits = mem.recall(embed(query), top_k=top_k)
        return [{"id": doc_id, "text": text, "score": score} for doc_id, text, score in hits]

    def audit(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        if self.records.matter_meta(client_id).get("hibernated"):
            raise RuntimeError("tenant is hibernated — wake it before running the checklist")
        chunks = self._chunks_from_dbx(client)
        if not chunks:
            raise ValueError("index a document before running the checklist")
        recalls = {spec["key"]: self._recall(client, spec["query"], top_k=4) for spec in PLAYBOOK}
        items = evaluate(chunks, recalls)
        silence = self._silence(email, client, items)
        payload = {
            "items": items,
            "playbook": PLAYBOOK_NAME,
            "sibling_hits": silence["sibling_hits"],
            "siblings_probed": silence["siblings_probed"],
            "chunk_count": len(chunks),
            "tenant_id": client["tenant_id"],
        }
        saved = self.records.save_run(client_id, payload)
        return self.detail(email, client_id) | {"ran_at": saved["ran_at"]}

    def _silence(self, email: str, client: Dict[str, Any], items: List[Dict[str, Any]]) -> Dict[str, int]:
        needle = ""
        for item in items:
            quote = (item.get("quote") or "").strip()
            if len(quote) > len(needle):
                needle = quote
        needle = needle[:180].lower()
        leaks = 0
        probed = 0
        if not needle:
            return {"sibling_hits": 0, "siblings_probed": 0}
        for other in self.store.list_clients(email):
            if other["id"] == client["id"]:
                continue
            sibling = self.store.get_client(email, other["id"])
            if not sibling:
                continue
            if self.records.matter_meta(sibling["id"]).get("hibernated"):
                continue
            if not self.records.documents(sibling["id"]):
                continue
            probed += 1
            try:
                hits = self._recall(sibling, needle, top_k=4)
            except Exception:
                continue
            for hit in hits:
                if needle[:80] and needle[:80] in (hit.get("text") or "").lower():
                    leaks += 1
        return {"sibling_hits": leaks, "siblings_probed": probed}

    def search(self, email: str, client_id: str, query: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        if self.records.matter_meta(client_id).get("hibernated"):
            raise RuntimeError("tenant is hibernated — wake it before searching")
        chunks = self._chunks_from_dbx(client)
        hits = self._recall(client, query, top_k=6)
        by_id = {c["id"]: c for c in chunks}
        rows = []
        for hit in hits:
            meta = by_id.get(hit["id"], {})
            rows.append(
                {
                    "id": hit["id"],
                    "text": hit["text"],
                    "score": hit["score"],
                    "doc_name": meta.get("doc_name") or "",
                    "index": meta.get("index", 0),
                }
            )
        rows.sort(key=lambda row: (overlap(query, row["text"]), row["score"]), reverse=True)
        return {"query": query, "tenant_id": client["tenant_id"], "hits": rows}

    def add_note(self, email: str, client_id: str, item_key: str, text: str) -> Dict[str, Any]:
        self._require(email, client_id)
        if not text.strip():
            raise ValueError("note is empty")
        return self.records.add_note(client_id, item_key, text, email)

    def read_document(self, email: str, client_id: str, doc_id: str = "") -> Dict[str, Any]:
        client = self._require(email, client_id)
        docs = self.records.documents(client_id)
        if not docs:
            raise ValueError("no document indexed")
        chosen = next((d for d in docs if d["id"] == doc_id), None) if doc_id else docs[-1]
        if chosen is None:
            raise KeyError("document not found")
        chunks = [
            c
            for c in self._chunks_from_dbx(client)
            if c["doc_id"] == chosen["id"]
        ]
        return {
            "id": chosen["id"],
            "name": chosen["name"],
            "tenant_id": client["tenant_id"],
            "ingested_at": chosen["ingested_at"],
            "source": "dbx",
            "chunks": [{"id": c["id"], "index": c["index"], "text": c["text"]} for c in chunks],
        }

    def memorandum(self, email: str, client_id: str) -> str:
        matter = self.detail(email, client_id)
        user = self._user_view(email)
        lines = [
            f"# Memorandum — {matter['name']}",
            "",
            f"Firm: {user['firm_name']}",
            f"Matter: {matter['matter']}",
            f"Reviewer: {user['name']}",
            f"Sealed tenant: {matter['tenant_id']}",
            f"Playbook: {matter['playbook']}",
            f"Read path: {matter['read_path']}",
            "",
        ]
        if not matter["items"]:
            lines.append("Checklist has not been run.")
        else:
            counts = tally(matter["items"])
            lines.append(
                f"Open findings: {counts['open']} "
                f"(high {counts['high']}, missing {counts['missing']}, medium {counts['medium']}, clear {counts['clear']})."
            )
            lines.append(
                f"Sibling tenants probed: {matter['siblings_probed']}. "
                f"Passages from other tenants matching this matter: {matter['sibling_hits']}."
            )
            lines.append("")
            for item in matter["items"]:
                lines.append(f"## {item['number']} {item['item']} — {item['state']}")
                lines.append(f"Playbook: {item['playbook']}")
                lines.append(f"Finding: {item['finding']}")
                lines.append(f"Citation: {item['citation']} · {item['doc_name']}")
                lines.append(f"Retrieved from DBX via {item['via']}.")
                if item.get("quote"):
                    lines.append("")
                    lines.append(f"> {item['quote']}")
                lines.append("")
        notes = [n for n in matter["notes"]]
        if notes:
            lines.append("## Reviewer notes")
            for note in notes:
                lines.append(f"- [{note['item_key']}] {note['text']}")
            lines.append("")
        lines.append("Each client is one DBX tenant. This memorandum cites only that tenant.")
        return "\n".join(lines).strip() + "\n"

    def export_memo(self, email: str, client_id: str, kind: str = "memorandum") -> Dict[str, Any]:
        client = self._require(email, client_id)
        body = self.memorandum(email, client_id)
        meta = self.records.matter_meta(client_id)
        row = self.records.add_export(
            email, client_id, client["name"], meta.get("matter") or "Matter", body, kind
        )
        tenant_export: Any = None
        if kind == "offboard":
            tenant_export = self.bridge.export_client(client["tenant_id"])
        return {"export": {k: v for k, v in row.items() if k != "body"}, "body": body, "tenant_export": tenant_export}

    def list_exports(self, email: str) -> List[Dict[str, Any]]:
        rows = []
        for row in self.records.exports(email):
            rows.append({k: v for k, v in row.items() if k != "body"})
        return rows

    def get_export(self, email: str, export_id: str) -> Dict[str, Any]:
        row = self.records.export(email, export_id)
        if not row:
            raise KeyError("export not found")
        return row

    def offboard(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        exported = self.export_memo(email, client_id, kind="offboard")
        receipt = self.bridge.delete_client(client["tenant_id"], purge=True)
        self.store.remove_client(email, client_id)
        self.records.drop_matter(client_id)
        return {
            "deleted": client_id,
            "tenant_id": client["tenant_id"],
            "receipt": receipt,
            "body": exported["body"],
            "export": exported["export"],
        }

    def hibernate(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        result = self.bridge.hibernate_client(client["tenant_id"])
        self.records.set_hibernated(client_id, True)
        return {"tenant_id": client["tenant_id"], "hibernated": True, "receipt": result}

    def wake(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        result = self.bridge.wake_client(client["tenant_id"])
        self.records.set_hibernated(client_id, False)
        return {"tenant_id": client["tenant_id"], "hibernated": False, "receipt": result}

    def usage(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        data = self.bridge.usage(client["tenant_id"])
        return {"tenant_id": client["tenant_id"], "usage": data}

    def playbook(self) -> Dict[str, Any]:
        return {"name": PLAYBOOK_NAME, "items": [{k: v for k, v in item.items() if k != "query"} | {"query": item["query"]} for item in PLAYBOOK]}
