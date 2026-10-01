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

    def add_document(
        self,
        client_id: str,
        doc_name: str,
        text: str,
        chunk_ids: List[str],
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
            }
            self._data["documents"].setdefault(client_id, []).append(doc)
            self._save()
            return {
                "id": doc["id"],
                "name": doc["name"],
                "chunk_ids": doc["chunk_ids"],
                "chunk_count": doc["chunk_count"],
                "ingested_at": doc["ingested_at"],
            }

    def documents(self, client_id: str) -> List[Dict[str, Any]]:
        with self._lock:
            self._load()
            rows = []
            for doc in self._data["documents"].get(client_id, []):
                rows.append(
                    {
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
            self._save()
