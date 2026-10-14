"""Jurix operations on the Legal Auditor firm registry and a live DBX node."""

from __future__ import annotations

import os
import re
import secrets
import hashlib
import time
import socket
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
LA = ROOT / "legal-auditor"
if str(LA) not in sys.path:
    sys.path.append(str(LA))
os.environ.setdefault(
    "LA_STORE_PATH", str(Path(__file__).resolve().parent / "data" / "firm_store.json")
)

from dbx_bridge import DBXBridge  # noqa: E402
from embeddings import chunk_text  # noqa: E402
from store import FirmStore  # noqa: E402

from embed import embed  # noqa: E402
from playbook import ALL_CHECKS, evaluate, overlap, tally  # noqa: E402
from records import Records  # noqa: E402
from review import DEFAULT_POLICY, PRESETS, REVIEW_STATES, DECIDED_STATES, normalize_policy, evidence_fingerprint, apply_reviews, aggregate_items, suggestion  # noqa: E402
from samples import SAMPLES  # noqa: E402
from custom_rules import evaluate_rules, source_anchor  # noqa: E402
from billing import DEFAULT_BILLING_POLICY, CODE_SOURCES, normalize_billing_policy, parse_entries, audit_entries  # noqa: E402
from providers import Providers, ProviderUnavailable
from documents import extract, validate_file, tracked_docx, compare, MAX_TEXT
from semantic import evaluate_semantic
from filelock import FileLock

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


def _public_matter(client: Dict[str, Any], meta: Dict[str, Any], docs: List[Dict[str, Any]], run: Optional[Dict[str, Any]], reviews: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    items = (run or {}).get("items") or []
    counts = tally(items) if items else {"high": 0, "medium": 0, "clear": 0, "missing": 0, "open": 0}
    reviewer = meta.get("reviewer") or client.get("owner_email") or ""
    chunk_count = sum(int(d.get("chunk_count") or 0) for d in docs)
    latest_doc = max((d.get("ingested_at") or 0) for d in docs) if docs else 0
    ran_at = (run or {}).get("ran_at") or 0
    policy = normalize_policy(meta.get("policy"))
    stale = bool(run) and (latest_doc > ran_at or run.get("policy") != policy or
        (run.get("document_snapshot") is not None and run["document_snapshot"] != [d["id"] for d in docs]))
    findings = [item for doc in apply_reviews((run or {}).get("document_runs") or [], reviews or {}) for item in doc["items"]]
    reviewed = sum(item["review"]["status"] in DECIDED_STATES for item in findings)
    if meta.get("hibernated"):
        status = "hibernated"
    elif not run:
        status = "not_started"
    elif not stale and findings and reviewed == len(findings):
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
        "stale": stale,
        "reviewed": reviewed,
        "review_total": len(findings),
        "review_open": len(findings) - reviewed,
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
    def __init__(self, store: Optional[FirmStore] = None, records: Optional[Records] = None, bridge: Optional[DBXBridge] = None, providers=None) -> None:
        if store is None:
            from security import SecureFirmStore
            store = SecureFirmStore()
        self.store = store
        self.database = getattr(store, "database", None)
        if records is None and self.database:
            from persistence import EncryptedRecords
            records = EncryptedRecords(database=self.database)
        self.records = records or Records()
        if os.getenv("JURIX_ENV") == "production" and (not self.database or not os.getenv("DBX_ADMIN_PASSWORD") or os.getenv("DBX_ADMIN_PASSWORD") == "adminadminadmin"):
            raise ValueError("Production requires encrypted persistence and a private DBX credential")
        self.bridge = bridge or DBXBridge()
        self.providers = providers or Providers()

    def owner(self, email):
        return self.store.workspace_owner(email) if hasattr(self.store, "workspace_owner") else email

    def authorize(self, email, *roles):
        if hasattr(self.store, "require_role"):
            self.store.require_role(email, *roles)

    def dbx_status(self) -> Dict[str, Any]:
        try:
            self.bridge.ensure_login()
            # Authentication may be cached; probe current availability on each
            # health request instead of treating a cached token as a live node.
            request = Request(self.bridge.control_url.rstrip("/") + "/api/tenants", headers={
                "Authorization": "Bearer " + self.bridge.plane.token,
            })
            with urlopen(request, timeout=3) as response:
                response.read(1)
            with socket.create_connection((self.bridge.resp_host, self.bridge.resp_port), timeout=2):
                pass
        except Exception as exc:
            self.bridge._logged_in = False
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

    def register(self, email: str, password: str, firm_name: str, invite_token: str = "") -> Dict[str, Any]:
        user = self.store.register(email, password, firm_name, invite_token=invite_token) if self.database else self.store.register(email, password, firm_name)
        owner = self.owner(user["email"])
        code = self.store.firm_code(user["email"]) if self.database else ""
        if not code and not invite_token:
            code = firm_code(user["firm_name"])
            if self.database:
                self.store.set_firm_code(user["email"], code)
        if not self.records.firm(owner):
            self.records.set_firm(owner, user["firm_name"], code)
        token = self.store.login(email, password)
        return {"token": token, "user": self._user_view(user["email"])}

    def login(self, email: str, password: str, code: str = "") -> Dict[str, Any]:
        email_n = email.strip().lower()
        if self.database:
            token = self.store.login(email, password, code)
            firm = self.records.firm(self.owner(email_n))
        else:
            firm = self.records.firm(email_n)
        if code.strip() and not self.database:
            expected = (firm or {}).get("code") or ""
            if not expected or code.strip().upper() != expected.upper():
                raise ValueError("firm code does not match this account")
        if not self.database:
            token = self.store.login(email, password)
        if not firm:
            user = self.store.user_for_token(token) or {}
            firm = self.records.set_firm(
                self.owner(email_n), user.get("firm_name") or email_n, firm_code(user.get("firm_name") or email_n)
            )
        return {"token": token, "user": self._user_view(email_n)}

    def user_view(self, email: str) -> Dict[str, Any]:
        return self._user_view(email)

    def _user_view(self, email: str) -> Dict[str, Any]:
        user = None
        # FirmStore keeps users private; read through a fresh token lookup is awkward.
        # The email is already authenticated by the caller.
        firm = self.records.firm(self.owner(email)) or {}
        stored = self.store.public_user(email) or {}
        firm_name = firm.get("firm_name") or stored.get("firm_name") or email
        return {
            "email": email,
            "firm_name": firm_name,
            "firm_code": firm.get("code") or "",
            "name": display_name(email),
            "initials": initials(email),
            "firm_id": stored.get("firm_id", ""),
            "role": stored.get("role", "admin"),
        }

    def list_matters(self, email: str) -> List[Dict[str, Any]]:
        rows = []
        for client in self.store.list_clients(email):
            full = self.store.get_client(email, client["id"]) or client
            meta = self.records.matter_meta(client["id"])
            docs = self.records.documents(client["id"])
            run = self.records.run(client["id"])
            rows.append(_public_matter(full, meta, docs, run, self.records.reviews(client["id"])))
        rows.sort(key=lambda r: (-(r["open"]), r["name"].lower()))
        return rows

    def create_matter(self, email: str, name: str, matter: str) -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        if not name.strip():
            raise ValueError("client name is empty")
        # The bridge reuses tenants by slug. A fresh identity prevents two firms
        # reviewing a same-named client from receiving keys to the same corpus.
        tenant_name = f"{secrets.token_hex(8)}-{name.strip()[:20]}"
        creds = self.bridge.provision_client(tenant_name)
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
                **{k: v for k, v in d.items() if k not in {"text", "chunk_ids"}},
                "id": d["id"],
                "name": d["name"],
                "chunk_count": d["chunk_count"],
                "ingested_at": d["ingested_at"],
            }
            for d in docs
        ]
        matter = _public_matter(client, meta, docs, run, self.records.reviews(client_id))
        matter["documents"] = public_docs
        matter["items"] = (run or {}).get("items") or []
        matter["notes"] = self.records.notes(client_id)
        policy = normalize_policy(meta.get("policy"))
        document_runs = apply_reviews((run or {}).get("document_runs") or [], self.records.reviews(client_id))
        if document_runs:
            matter["items"] = aggregate_items(document_runs)
        findings = [item for doc in document_runs for item in doc["items"]]
        matter["document_runs"] = document_runs
        matter["review_total"] = len(findings)
        matter["reviewed"] = sum(item["review"]["status"] in DECIDED_STATES for item in findings)
        matter["review_open"] = len(findings) - matter["reviewed"]
        matter["activity"] = self.records.activity(client_id)
        matter["policy"] = policy
        matter["playbook"] = policy["name"]
        matter["stale"] = matter["stale"] or bool(run and run.get("policy") != policy)
        if matter["stale"] and not matter["hibernated"]:
            matter["status"] = "in_review"
        matter["read_path"] = f"dbx://{client['tenant_id']}/{INDEX}"
        matter["external_processing"] = bool(meta.get("external_processing"))
        return matter

    def ingest(self, email: str, client_id: str, doc_name: str, text: str, metadata=None) -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        client = self._require(email, client_id)
        if not text.strip() or len(text) > MAX_TEXT or "\x00" in text or not doc_name.strip() or len(doc_name) > 200:
            raise ValueError("Use a document name up to 200 characters and text up to 200,000 characters")
        if len(self.records.documents(client_id)) >= 100:
            raise ValueError("Matter document version limit reached (100)")
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
        saved = self.records.add_document(client_id, doc_name, text, ids, {"text_sha256": hashlib.sha256(text.encode()).hexdigest(), **(metadata or {})})
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

    def ingest_version(self, email, client_id, doc_name, text, supersedes=""):
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        lock_path = str(self.records.path) + ".import-" + hashlib.sha256(client_id.encode()).hexdigest()[:20] + ".lock"
        with FileLock(lock_path, timeout=10):
            docs = self.records.documents(client_id)
            previous = next((d for d in docs if d["id"] == supersedes), None) if supersedes else None
            if supersedes and (not previous or any(d.get("supersedes") == supersedes for d in docs)):
                raise ValueError("Choose the current document version to supersede")
            return self.ingest(email, client_id, doc_name, text, {
                "version": int((previous or {}).get("version", 1)) + 1 if previous else 1,
                "supersedes": supersedes or None, "method": "Pasted text",
                "reviewed_by": email, "reviewed_at": time.time(),
            })

    def _chunks_from_dbx(self, client: Dict[str, Any], docs: Optional[List[Dict[str, Any]]] = None) -> List[Dict[str, Any]]:
        docs = self.records.documents(client["id"]) if docs is None else docs
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
        self.authorize(email, "admin", "reviewer")
        client = self._require(email, client_id)
        if self.records.matter_meta(client_id).get("hibernated"):
            raise RuntimeError("tenant is hibernated — wake it before running the checklist")
        all_docs = self.records.documents(client_id)
        superseded = {d.get("supersedes") for d in all_docs if d.get("supersedes")}
        docs = [d for d in all_docs if d["id"] not in superseded]
        chunks = self._chunks_from_dbx(client, docs)
        if not chunks:
            raise ValueError("index a document before running the checklist")
        policy = normalize_policy(self.records.matter_meta(client_id).get("policy"))
        if any(r["kind"] == "semantic" for r in policy["custom_rules"]):
            self.require_processing(email, client_id, "llm")
        recalls = {spec["key"]: self._recall(client, spec["query"], top_k=4) for spec in ALL_CHECKS if spec["key"] in policy["enabled_checks"]}
        document_runs = []
        for doc in docs:
            doc_chunks = [chunk for chunk in chunks if chunk["doc_id"] == doc["id"]]
            doc_items = evaluate(doc_chunks, recalls, policy)
            doc_items.extend(evaluate_rules(doc_chunks, policy["custom_rules"], len(doc_items)))
            doc_items.extend(evaluate_semantic(doc, doc_chunks, policy["custom_rules"], self.providers, len(doc_items)))
            for item in doc_items:
                item["doc_id"] = doc["id"]
                item["doc_name"] = doc["name"]
                item["id"] = f"{doc['id']}:{item['key']}"
                item["suggestion"] = suggestion(item["key"], policy)
                if "anchor" not in item:
                    item["anchor"] = source_anchor(item, doc_chunks, doc["text"])
                if item["anchor"]:
                    item["citation"] = f"{item['section']} · lines {item['anchor']['line_start']}–{item['anchor']['line_end']}"
                item.setdefault("method", "Explicit firm rule / deterministic text check")
                item["fingerprint"] = evidence_fingerprint(item, policy)
            document_runs.append({"doc_id": doc["id"], "doc_name": doc["name"], "items": doc_items,
                                  "scope_note": "No selected rules apply to this document" if not doc_items else ""})
        items = aggregate_items(document_runs)
        if not items:
            raise ValueError("No selected rules apply to these documents; change the custom rule scope or enable a review check")
        silence = self._silence(email, client, items)
        payload = {
            "items": items,
            "playbook": policy["name"],
            "policy": policy,
            "document_runs": document_runs,
            "document_snapshot": [doc["id"] for doc in all_docs],
            "sibling_hits": silence["sibling_hits"],
            "siblings_probed": silence["siblings_probed"],
            "chunk_count": len(chunks),
            "tenant_id": client["tenant_id"],
        }
        saved = self.records.save_run(client_id, payload)
        return self.detail(email, client_id) | {"ran_at": saved["ran_at"]}

    def matter_playbook(self, email: str, client_id: str) -> Dict[str, Any]:
        self._require(email, client_id)
        return normalize_policy(self.records.matter_meta(client_id).get("policy"))

    def set_playbook(self, email: str, client_id: str, policy: Dict[str, Any]) -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        self.records.set_policy(client_id, normalize_policy(policy))
        return self.detail(email, client_id)

    def firm_playbooks(self, email: str) -> Dict[str, Any]:
        return {"playbooks": self.records.firm_playbooks(self.owner(email))}

    def save_firm_playbook(self, email: str, policy: Dict[str, Any]) -> Dict[str, Any]:
        self.authorize(email, "admin")
        return self.records.save_firm_playbook(self.owner(email), normalize_policy(policy))

    def review_finding(self, email: str, client_id: str, item_id: str, status: str, note: str, suggestion_decision: str = "pending") -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        if status not in REVIEW_STATES:
            raise ValueError("unsupported review status")
        if suggestion_decision not in {"pending", "accepted", "rejected"}:
            raise ValueError("unsupported proposal decision")
        if (status in DECIDED_STATES or suggestion_decision != "pending") and not note.strip():
            raise ValueError("a rationale is required to resolve a finding or accept its risk")
        matter = self.detail(email, client_id)
        if matter["stale"]:
            raise ValueError("documents or playbook changed; run the audit again before recording a decision")
        item = next((item for doc in matter["document_runs"] for item in doc["items"] if item["id"] == item_id), None)
        if item is None:
            raise KeyError("finding not found")
        if suggestion_decision != "pending" and not item.get("proposed_text"):
            raise ValueError("this finding has no firm-supplied clause proposal")
        self.records.set_review(client_id, item, status, note.strip(), email, matter["ran_at"], suggestion_decision)
        return self.detail(email, client_id)

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
                    "doc_id": meta.get("doc_id") or "",
                    "index": meta.get("index", 0),
                }
            )
        rows.sort(key=lambda row: (overlap(query, row["text"]), row["score"]), reverse=True)
        return {"query": query, "tenant_id": client["tenant_id"], "hits": rows}

    def add_note(self, email: str, client_id: str, item_key: str, text: str) -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        if not text.strip():
            raise ValueError("note is empty")
        run = self.records.run(client_id) or {}
        keys = {item["key"] for item in run.get("items", [])}
        keys.update(item["id"] for doc in run.get("document_runs", []) for item in doc["items"])
        if item_key not in keys:
            raise KeyError("finding not found")
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
            **{k: v for k, v in chosen.items() if k not in {"chunk_ids", "text"}},
            "id": chosen["id"],
            "name": chosen["name"],
            "tenant_id": client["tenant_id"],
            "ingested_at": chosen["ingested_at"],
            "source": "dbx",
            "text": chosen["text"],
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
            "Review method: first-pass text checks against firm-defined standards; counsel must verify scope and legal effect.",
            f"Reviewer decisions: {matter['reviewed']} of {matter['review_total']} recorded.",
            f"Audit currency: {'STALE — documents or playbook changed; rerun required' if matter['stale'] else 'current' if matter['has_run'] else 'not run'}.",
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
            report_items = [item for doc in matter["document_runs"] for item in doc["items"]] or matter["items"]
            for item in report_items:
                lines.append(f"## {item['number']} {item['item']} — {item['state']}")
                lines.append(f"Playbook: {item['playbook']}")
                lines.append(f"Finding: {item['finding']}")
                lines.append(f"Citation: {item['citation']} · {item['doc_name']}")
                lines.append(f"Retrieved from DBX via {item['via']}.")
                decision = item.get("review", {})
                lines.append(f"Reviewer status: {decision.get('status', 'open')}.")
                if decision.get("note"):
                    lines.append(f"Rationale: {decision['note']} (by {decision.get('author', '')}).")
                if item.get("suggestion"):
                    lines.append(f"Suggested next step: {item['suggestion']}")
                if item.get("authority"):
                    lines.append(f"Firm-supplied reference: {item['authority']['title']} — {item['authority']['url']} (applicability not independently verified).")
                if item.get("proposed_text"):
                    lines.append(f"Firm clause proposal ({decision.get('suggestion_decision', 'pending')}): {item['proposed_text']}")
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
        if matter["activity"]:
            lines.append("## Decision history")
            for event in matter["activity"]:
                stamp = datetime.fromtimestamp(event['created_at'], timezone.utc).isoformat()
                lines.append(f"- {stamp} [{event['item_id']}] {event['status']}; proposal {event.get('suggestion_decision', 'pending')} by {event['author']}: {event['note']}")
                if event.get("fingerprint"):
                    lines.append(f"  Evidence fingerprint: {event['fingerprint']}; audit: {event.get('ran_at')}; policy: {event.get('policy', {}).get('name', '')}.")
                if event.get("evidence"):
                    lines.append(f"  Evidence snapshot: {event['evidence']}")
            lines.append("")
        lines.append("Each client is one DBX tenant. This memorandum cites only that tenant.")
        return "\n".join(lines).strip() + "\n"

    def export_proposals(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        matter = self.detail(email, client_id)
        if not matter["has_run"] or matter["stale"]:
            raise ValueError("run a current audit before exporting clause proposals")
        items = [i for d in matter["document_runs"] for i in d["items"] if i.get("proposed_text") and i["review"].get("suggestion_decision") == "accepted"]
        if not items:
            raise ValueError("accept at least one firm clause proposal before exporting")
        lines = [f"# Accepted clause proposals — {matter['name']}", "", "Review comparison only. Source documents have not been changed. Counsel must integrate the clauses and confirm their context.", ""]
        for item in items:
            lines += [f"## {item['doc_name']} — {item['item']}", f"Source: {item['citation']}", f"Rule: {item['playbook']}", "", "### Original passage", item["quote"] or "No matching provision located; counsel must choose an insertion point.", "", "### Firm preferred text", item["proposed_text"], "", f"Accepted by {item['review']['author']}: {item['review']['note']}", f"Evidence fingerprint: {item['fingerprint']}", ""]
        body = "\n".join(lines)
        row = self.records.add_export(self.owner(email), client_id, client["name"], matter["matter"], body, "clause-proposals")
        return {"export": {k: v for k, v in row.items() if k != "body"}, "body": body}

    def billing_history(self, email: str, client_id: str) -> Dict[str, Any]:
        self._require(email, client_id)
        return {"runs": self.records.billing_runs(client_id), "defaults": DEFAULT_BILLING_POLICY, "sources": CODE_SOURCES}

    def audit_billing(self, email: str, client_id: str, csv_text: str, policy: Dict[str, Any]) -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        result = audit_entries(parse_entries(csv_text), normalize_billing_policy(policy))
        return self.records.save_billing_run(client_id, result, email)

    def export_billing(self, email: str, client_id: str, run_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        run = next((r for r in self.records.billing_runs(client_id) if r["id"] == run_id), None)
        if run is None:
            raise KeyError("billing review not found")
        # Indented literal text keeps submitted narratives out of Markdown markup.
        literal = lambda text: "\n".join("    " + line for line in str(text).splitlines())
        stamp = datetime.fromtimestamp(run["created_at"], timezone.utc).isoformat()
        lines = [f"# Pre-bill review — {client['name']}", "", run["method"], f"Reviewed by {run['author']} at {stamp}", f"Code set: {run['code_set']}", f"Guideline snapshot: {run['policy']}", f"Entries: {len(run['entries'])}; flagged: {run['flagged_entries']}; hours: {run['total_hours']:g}", ""]
        for entry in run["entries"]:
            lines += [f"## Entry {entry['row'] - 1}", literal(f"{entry['date']} · {entry['timekeeper']} · {entry['hours']:g} hours · {entry['task_code']} / {entry['activity_code']}"), "", literal(entry["narrative"]), ""]
            for flag in entry["flags"]:
                lines += [literal(flag["description"]), literal(flag["next_step"]), ""]
            if not entry["flags"]:
                lines += ["No flags under the selected checks. Partner review still required.", ""]
        lines += ["Code references: " + "; ".join(source["url"] for source in run["sources"])]
        body = "\n".join(lines)
        row = self.records.add_export(self.owner(email), client_id, client["name"], self.records.matter_meta(client_id).get("matter", "Matter"), body, "billing-review")
        return {"export": {k: v for k, v in row.items() if k != "body"}, "body": body}

    def export_memo(self, email: str, client_id: str, kind: str = "memorandum") -> Dict[str, Any]:
        client = self._require(email, client_id)
        body = self.memorandum(email, client_id)
        meta = self.records.matter_meta(client_id)
        row = self.records.add_export(
            self.owner(email), client_id, client["name"], meta.get("matter") or "Matter", body, kind
        )
        tenant_export: Any = None
        if kind == "offboard":
            tenant_export = self.bridge.export_client(client["tenant_id"])
        return {"export": {k: v for k, v in row.items() if k != "body"}, "body": body, "tenant_export": tenant_export}

    def list_exports(self, email: str) -> List[Dict[str, Any]]:
        rows = []
        for row in self.records.exports(self.owner(email)):
            rows.append({k: v for k, v in row.items() if k != "body"})
        return rows

    def get_export(self, email: str, export_id: str) -> Dict[str, Any]:
        row = self.records.export(self.owner(email), export_id)
        if not row:
            raise KeyError("export not found")
        return row

    def offboard(self, email: str, client_id: str) -> Dict[str, Any]:
        self.authorize(email, "admin")
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
        self.authorize(email, "admin")
        client = self._require(email, client_id)
        result = self.bridge.hibernate_client(client["tenant_id"])
        self.records.set_hibernated(client_id, True)
        return {"tenant_id": client["tenant_id"], "hibernated": True, "receipt": result}

    def wake(self, email: str, client_id: str) -> Dict[str, Any]:
        self.authorize(email, "admin", "reviewer")
        client = self._require(email, client_id)
        result = self.bridge.wake_client(client["tenant_id"])
        self.records.set_hibernated(client_id, False)
        return {"tenant_id": client["tenant_id"], "hibernated": False, "receipt": result}

    def usage(self, email: str, client_id: str) -> Dict[str, Any]:
        client = self._require(email, client_id)
        data = self.bridge.usage(client["tenant_id"])
        return {"tenant_id": client["tenant_id"], "usage": data}

    def playbook(self) -> Dict[str, Any]:
        return {"name": DEFAULT_POLICY["name"], "items": ALL_CHECKS, "defaults": normalize_policy(), "presets": PRESETS}

    def capabilities(self):
        return self.providers.capabilities()

    def require_processing(self, email, client_id, capability):
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        if not self.records.matter_meta(client_id).get("external_processing"):
            raise PermissionError("Enable external processing for this matter after checking the client's confidentiality requirements")
        if not self.capabilities()[capability]["available"]:
            raise ProviderUnavailable("This processor is not configured on the server")

    def set_processing(self, email, client_id, enabled):
        self.authorize(email, "admin")
        self._require(email, client_id)
        self.records.set_processing(client_id, enabled, email)
        return self.detail(email, client_id)

    def _feature_db(self):
        if not self.database:
            raise ProviderUnavailable("This workflow requires encrypted persistence")
        return self.database

    def upload_document(self, email, client_id, filename, data, use_ocr=False):
        self.authorize(email, "admin", "reviewer")
        self._require(email, client_id)
        name, extension = validate_file(filename, data)
        if use_ocr or extension in {".png", ".jpg", ".jpeg", ".tif", ".tiff"}:
            self.require_processing(email, client_id, "ocr")
        db, owner = self._feature_db(), self.owner(email)
        if sum(e.get("client_id") == client_id for e in db.list_objects(owner, "extractions")) >= 100:
            raise ValueError("Matter import limit reached (100 drafts)")
        result = extract(name, data, self.providers, use_ocr)
        source_type = {".pdf": "application/pdf", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".txt": "text/plain", ".md": "text/markdown", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".tif": "image/tiff", ".tiff": "image/tiff"}[extension]
        blob = db.save_blob(owner, name, data, source_type)
        result.update(id=secrets.token_hex(12), client_id=client_id, blob_id=blob, has_original=True, created_at=time.time(), author=email)
        try:
            db.put_object(owner, "extractions", result["id"], result)
        except Exception:
            db.delete_object(owner, "archive", blob)
            raise
        return {k: v for k, v in result.items() if k != "blob_id"}

    def extraction(self, email, client_id, extraction_id):
        self._require(email, client_id)
        row = self._feature_db().get_object(self.owner(email), "extractions", extraction_id)
        if not row or row["client_id"] != client_id:
            raise KeyError("Document import not found")
        return row

    def original(self, email, client_id, extraction_id):
        row = self.extraction(email, client_id, extraction_id)
        if not row.get("blob_id"):
            if row.get("original_extraction_id"):
                return self.original(email, client_id, row["original_extraction_id"])
            raise KeyError("Original file not found")
        return self.database.open_blob(self.owner(email), row["blob_id"])

    def commit_extraction(self, email, client_id, extraction_id, text, name, supersedes=""):
        self.authorize(email, "admin", "reviewer")
        db = self._feature_db()
        # Serialize duplicate commit requests and version choices across workers.
        with FileLock(str(db.path) + ".import-" + hashlib.sha256(client_id.encode()).hexdigest()[:20] + ".lock", timeout=10):
            row = self.extraction(email, client_id, extraction_id)
            if row["status"] != "needs_review":
                raise ValueError("This import has already been committed")
            docs = self.records.documents(client_id)
            previous = next((d for d in docs if d["id"] == supersedes), None) if supersedes else None
            if supersedes and (not previous or any(d.get("supersedes") == supersedes for d in docs)):
                raise ValueError("Choose the current document version to supersede")
            pages = row["pages"] if text == row["text"] else []
            metadata = {"extraction_id": extraction_id, "source_sha256": row["source_sha256"], "has_original": bool(row.get("blob_id") or row.get("has_original")), "pages": pages, "method": row["method"], "warnings": row["warnings"], "version": int((previous or {}).get("version", 1)) + 1 if previous else 1, "supersedes": supersedes or None, "reviewed_by": email, "reviewed_at": time.time(), "corrected": text != row["text"], "translated_from": row.get("translated_from"), "provider_model": row.get("provider_model")}
            # Source page offsets cannot truthfully survive arbitrary corrections.
            if not pages:
                metadata["warnings"] = list(metadata["warnings"]) + ["Text was corrected; original page offsets are unavailable for this version."]
            saved = self.ingest(email, client_id, name, text, metadata)
            row.update(status="committed", document_id=saved["id"], committed_at=time.time(), reviewed_by=email)
            db.put_object(self.owner(email), "extractions", extraction_id, row)
            return saved

    def _document(self, email, client_id, document_id):
        self._require(email, client_id)
        doc = next((d for d in self.records.documents(client_id) if d["id"] == document_id), None)
        if not doc:
            raise KeyError("Document not found")
        return doc

    def translate_document(self, email, client_id, document_id, language, glossary):
        self.require_processing(email, client_id, "translation")
        db = self._feature_db()
        doc = self._document(email, client_id, document_id)
        result = self.providers.translate(doc["text"], language, glossary)
        row = {"id": secrets.token_hex(12), "client_id": client_id, "name": doc["name"] + " — " + language, "text": result["text"], "pages": [], "warnings": result.get("warnings", []) + ["Draft translation. Verify procedural terms and citations against the original."], "source_sha256": doc.get("source_sha256") or doc.get("text_sha256") or hashlib.sha256(doc["text"].encode()).hexdigest(), "method": "OpenAI translation", "status": "needs_review", "translated_from": doc["id"], "target_language": language, "glossary": glossary, "provider_model": self.providers.model, "original_extraction_id": doc.get("extraction_id"), "created_at": time.time(), "author": email}
        row["has_original"] = bool(doc.get("extraction_id")) and doc.get("has_original", True)
        db.put_object(self.owner(email), "extractions", row["id"], row)
        return row

    def word_redline(self, email, client_id, document_id):
        self.authorize(email, "admin", "reviewer")
        doc = self._document(email, client_id, document_id)
        matter = self.detail(email, client_id)
        if not matter["has_run"] or matter["stale"]:
            raise ValueError("Run a current audit before exporting Word revisions")
        items = [i for d in matter["document_runs"] if d["doc_id"] == document_id for i in d["items"] if i.get("proposed_text") and i["review"].get("suggestion_decision") == "accepted"]
        return tracked_docx(doc["text"], items, email)

    def compare_documents(self, email, client_id, document_id, other_id):
        return compare(self._document(email, client_id, document_id), self._document(email, client_id, other_id))

    def dictation(self, email, client_id, filename, data, lexicon, language):
        self.require_processing(email, client_id, "dictation")
        name, ext = validate_file(filename, data)
        if ext not in {".wav", ".mp3", ".mp4", ".mpeg", ".mpga", ".m4a", ".webm"}:
            raise ValueError("Upload WAV, MP3, MP4, MPEG, MPGA, M4A or WebM audio")
        return self.providers.transcribe(name, data, lexicon, language)

    def regulatory_watches(self):
        from regulations import RegulatoryWatches
        return RegulatoryWatches(self._feature_db(), self.store, self.records)
