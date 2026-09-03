from typing import List, Optional, Tuple

import pytest

from dbx import DBXError, TenantMemory


class FakeClient:
    def __init__(self) -> None:
        self.kv = {}
        self.vecs = {}

    def set(self, key: str, val: str, ex: Optional[int] = None) -> bool:
        self.kv[key] = val
        return True

    def get(self, key: str) -> Optional[str]:
        return self.kv.get(key)

    def delete(self, *keys: str) -> int:
        n = 0
        for key in keys:
            if key in self.kv:
                del self.kv[key]
                n += 1
        return n

    def vadd(self, index_name: str, doc_id: str, vector: List[float]) -> bool:
        self.vecs[doc_id] = vector
        return True

    def vsearch(
        self, index_name: str, query_vector: List[float], top_k: int = 4
    ) -> List[Tuple[str, float]]:
        return [(doc_id, 1.0) for doc_id in list(self.vecs)[:top_k]]

    def vdel(self, index_name: str, doc_id: str) -> bool:
        return self.vecs.pop(doc_id, None) is not None


def test_remember_and_recall_one_customer() -> None:
    mem = TenantMemory(FakeClient())
    mem.remember("pref", "likes dark mode", vector=[1.0, 0.0])
    assert mem.get("pref") == "likes dark mode"
    hits = mem.recall([1.0, 0.0], top_k=1)
    assert hits == [("pref", "likes dark mode", 1.0)]
    mem.forget("pref")
    assert mem.get("pref") is None
    assert mem.recall([1.0, 0.0]) == []


def test_remember_without_vector_is_just_state() -> None:
    mem = TenantMemory(FakeClient())
    mem.remember("session", '{"step": 2}')
    assert mem.get("session") == '{"step": 2}'
    assert mem.recall([0.0]) == []


def test_set_failure_surfaces() -> None:
    class Boom(FakeClient):
        def set(self, key: str, val: str, ex: Optional[int] = None) -> bool:
            return False

    with pytest.raises(DBXError):
        TenantMemory(Boom()).remember("k", "v")
