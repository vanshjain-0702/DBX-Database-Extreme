"""Jurix working memory: documents, checklist runs, notes, and exports.

Client corpora live in DBX. This file only remembers which tenant belongs
to which matter, plus reviewer notes and the last checklist the firm ran.
"""

from __future__ import annotations

import json
import os
import secrets
import time
from filelock import FileLock
from pathlib import Path
from typing import Any, Dict, List, Optional


class Records:
    def __init__(self, path: Optional[str] = None) -> None:
        default = Path(__file__).resolve().parent / "data" / "jurix_records.json"
        self.path = Path(path or os.environ.get("JURIX_RECORDS_PATH", str(default)))
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = FileLock(str(self.path) + ".lock")
        self._data: Dict[str, Any] = {
            "firms": {},
            "matters": {},
            "documents": {},
            "runs": {},
            "notes": {},
            "exports": [],
            "reviews": {},
            "activity": {},
            "firm_playbooks": {},
            "billing_runs": {},
        }
        self._load()

    def _load(self) -> None:
        if self.path.exists():
            loaded = json.loads(self.path.read_text(encoding="utf-8"))
            self._data.update(loaded)

    def _save(self) -> None:
        tmp = self.path.with_suffix(".tmp")
        tmp.write_text(json.dumps(self._data, indent=2), encoding="utf-8")
        tmp.replace(self.path)

    def set_firm(self, email: str, firm_name: str, code: str) -> Dict[str, Any]:
        with self._lock:
            self._load()
            row = {"email": email, "firm_name": firm_name, "code": code}
            self._data["firms"][email] = row
            self._save()
            return dict(row)

    def firm(self, email: str) -> Optional[Dict[str, Any]]:
        with self._lock:
            self._load()
            row = self._data["firms"].get(email)
            return dict(row) if row else None

    def init_matter(self, client_id: str, matter: str, reviewer: str) -> None:
        with self._lock:
            self._load()
            self._data["matters"][client_id] = {
                "matter": matter,
                "reviewer": reviewer,
                "hibernated": False,
                "created_at": time.time(),
            }
            self._data["documents"].setdefault(client_id, [])
            self._data["notes"].setdefault(client_id, [])
            self._save()

    def matter_meta(self, client_id: str) -> Dict[str, Any]:
        with self._lock:
            self._load()
            return dict(
                self._data["matters"].get(
                    client_id,
                    {"matter": "Matter", "reviewer": "", "hibernated": False},
                )
            )

    def set_hibernated(self, client_id: str, hibernated: bool) -> None:
        with self._lock:
            self._load()
            row = self._data["matters"].setdefault(
                client_id, {"matter": "Matter", "reviewer": "", "hibernated": False}
            )
            row["hibernated"] = hibernated
            self._save()

    def set_policy(self, client_id: str, policy: Dict[str, Any]) -> None:
        with self._lock:
            self._load()
            self._data["matters"][client_id]["policy"] = policy
            self._save()

    def set_processing(self, client_id: str, enabled: bool, author: str) -> None:
        with self._lock:
            self._load()
            self._data["matters"][client_id].update(external_processing=enabled, processing_author=author, processing_updated_at=time.time())
            self._save()

    def firm_playbooks(self, owner: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            return [dict(row) for row in self._data["firm_playbooks"].get(owner, [])]

    def save_firm_playbook(self, owner: str, policy: Dict[str, Any]) -> Dict[str, Any]:
        with self._lock:
            self._load()
            rows = self._data["firm_playbooks"].setdefault(owner, [])
            if len(rows) >= 100:
                raise ValueError("firm playbook library limit reached (100 versions)")
            version = 1 + max((row["version"] for row in rows if row["policy"]["name"] == policy["name"]), default=0)
            row = {"id": secrets.token_hex(8), "version": version, "policy": policy, "created_at": time.time(), "author": owner}
            rows.append(row)
            self._save()
            return dict(row)

    def save_billing_run(self, client_id: str, result: Dict[str, Any], author: str) -> Dict[str, Any]:
        with self._lock:
            self._load()
            result = {**result, "id": secrets.token_hex(8), "created_at": time.time(), "author": author}
            self._data["billing_runs"].setdefault(client_id, []).append(result)
            self._save()
            return dict(result)

    def billing_runs(self, client_id: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            return [dict(row) for row in reversed(self._data["billing_runs"].get(client_id, []))]

    def reviews(self, client_id: str) -> Dict[str, Any]:
        with self._lock:
            self._load()
            return dict(self._data["reviews"].get(client_id, {}))

    def activity(self, client_id: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            return [dict(row) for row in self._data["activity"].get(client_id, [])]

    def set_review(self, client_id: str, item: Dict[str, Any], status: str, note: str, author: str, ran_at: float, suggestion_decision: str = "pending") -> None:
        from review import normalize_policy

        with self._lock:
            self._load()
            run = self._data["runs"].get(client_id) or {}
            latest_doc = max((d["ingested_at"] for d in self._data["documents"].get(client_id, [])), default=0)
            policy = normalize_policy(self._data["matters"].get(client_id, {}).get("policy"))
            doc_ids = [d["id"] for d in self._data["documents"].get(client_id, [])]
            if run.get("ran_at") != ran_at or latest_doc > ran_at or policy != run.get("policy") or run.get("document_snapshot") != doc_ids:
                raise ValueError("documents or playbook changed; run the audit again before recording a decision")
            decision = {"status": status, "note": note, "author": author, "updated_at": time.time(), "fingerprint": item["fingerprint"], "suggestion_decision": suggestion_decision}
            self._data["reviews"].setdefault(client_id, {})[item["id"]] = decision
            self._data["activity"].setdefault(client_id, []).append({
                "id": secrets.token_hex(6), "item_id": item["id"], "status": status,
                "note": note, "author": author, "created_at": decision["updated_at"],
                "fingerprint": item["fingerprint"], "policy": run.get("policy"),
                "evidence": {key: item.get(key) for key in ["doc_id", "doc_name", "quote", "anchor", "state", "finding", "proposed_text", "authority", "provenance", "method"]},
                "ran_at": ran_at, "suggestion_decision": decision.get("suggestion_decision", "pending"),
            })
            self._save()

    def add_document(
        self,
        client_id: str,
        doc_name: str,
        text: str,
        chunk_ids: List[str],
        metadata: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        with self._lock:
            self._load()
            doc = {
                "id": secrets.token_hex(4),
                "name": doc_name,
                "text": text,
                "chunk_ids": list(chunk_ids),
                "chunk_count": len(chunk_ids),
                "ingested_at": time.time(),
                **(metadata or {}),
            }
            self._data["documents"].setdefault(client_id, []).append(doc)
            self._save()
            return {
                "id": doc["id"],
                "name": doc["name"],
                "chunk_ids": doc["chunk_ids"],
                "chunk_count": doc["chunk_count"],
                "ingested_at": doc["ingested_at"],
                **(metadata or {}),
            }

    def documents(self, client_id: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            rows = []
            for doc in self._data["documents"].get(client_id, []):
                rows.append(
                    {
                        **doc,
                        "id": doc["id"],
                        "name": doc["name"],
                        "chunk_ids": list(doc["chunk_ids"]),
                        "chunk_count": doc["chunk_count"],
                        "ingested_at": doc["ingested_at"],
                        "text": doc["text"],
                    }
                )
            return rows

    def save_run(self, client_id: str, payload: Dict[str, Any]) -> Dict[str, Any]:
        with self._lock:
            self._load()
            payload = dict(payload)
            payload["ran_at"] = time.time()
            self._data["runs"][client_id] = payload
            self._save()
            return payload

    def run(self, client_id: str) -> Optional[Dict[str, Any]]:
        with self._lock:
            self._load()
            row = self._data["runs"].get(client_id)
            return dict(row) if row else None

    def add_note(
        self, client_id: str, item_key: str, text: str, author: str
    ) -> Dict[str, Any]:
        with self._lock:
            self._load()
            note = {
                "id": secrets.token_hex(4),
                "item_key": item_key,
                "text": text.strip(),
                "author": author,
                "created_at": time.time(),
            }
            self._data["notes"].setdefault(client_id, []).append(note)
            self._save()
            return dict(note)

    def notes(self, client_id: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            return [dict(n) for n in self._data["notes"].get(client_id, [])]

    def add_export(
        self,
        owner: str,
        client_id: str,
        client_name: str,
        matter: str,
        body: str,
        kind: str,
    ) -> Dict[str, Any]:
        with self._lock:
            self._load()
            row = {
                "id": secrets.token_hex(6),
                "owner": owner,
                "client_id": client_id,
                "client_name": client_name,
                "matter": matter,
                "kind": kind,
                "body": body,
                "created_at": time.time(),
            }
            self._data["exports"].append(row)
            self._save()
            return dict(row)

    def exports(self, owner: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            rows = [dict(e) for e in self._data["exports"] if e["owner"] == owner]
            return sorted(rows, key=lambda e: e["created_at"], reverse=True)

    def export(self, owner: str, export_id: str) -> Optional[Dict[str, Any]]:
        with self._lock:
            self._load()
            for row in self._data["exports"]:
                if row["id"] == export_id and row["owner"] == owner:
                    return dict(row)
            return None

    def drop_matter(self, client_id: str) -> None:
        with self._lock:
            self._load()
            self._data["matters"].pop(client_id, None)
            self._data["documents"].pop(client_id, None)
            self._data["runs"].pop(client_id, None)
            self._data["notes"].pop(client_id, None)
            self._data["reviews"].pop(client_id, None)
            self._data["activity"].pop(client_id, None)
            self._data["billing_runs"].pop(client_id, None)
            self._save()
