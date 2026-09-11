from typing import List, Optional, Tuple

import pytest

from dbx import DBXClient, DBXError, TenantMemory, _vadd_batch_chunk_size


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

    def vadd(self, index_name: str, doc_id: str, vector: List[float], **kwargs) -> bool:
        self.vecs[(index_name, kwargs.get("space"), doc_id)] = vector
        self.vecs[doc_id] = vector
        return True

    def vsearch(
        self, index_name: str, query_vector: List[float], top_k: int = 4, **kwargs
    ) -> List[Tuple[str, float]]:
        ids = [doc_id for doc_id in self.vecs if isinstance(doc_id, str)]
        return [(doc_id, 1.0) for doc_id in ids[:top_k]]

    def vsim(
        self, index_name: str, doc_id: str, top_k: int = 4, **kwargs
    ) -> List[Tuple[str, float]]:
        ids = [
            other for other in self.vecs if isinstance(other, str) and other != doc_id
        ]
        return [(other, 0.9) for other in ids[:top_k]]

    def vfuse(
        self, index_name: str, queries, top_k: int = 4, **kwargs
    ) -> List[Tuple[str, float]]:
        ids = [doc_id for doc_id in self.vecs if isinstance(doc_id, str)]
        return [(doc_id, 0.8) for doc_id in ids[:top_k]]

    def vdel(self, index_name: str, doc_id: str, **kwargs) -> bool:
        space = kwargs.get("space")
        self.vecs.pop((index_name, space, doc_id), None)
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


def test_similar_and_fuse() -> None:
    mem = TenantMemory(FakeClient())
    mem.remember("a", "alpha", vector=[1.0, 0.0])
    mem.remember("b", "beta", vector=[0.9, 0.1])
    hits = mem.similar("a", top_k=1)
    assert hits[0][0] == "b"
    fused = mem.fuse({"text": [1.0, 0.0], "image": [1.0, 0.0]}, top_k=1)
    assert fused[0][0] in {"a", "b"}


def test_vadd_batch_chunk_fits_resp_array_cap() -> None:
    assert _vadd_batch_chunk_size(32) == min(1000, (4096 - 3) // 33)
    assert _vadd_batch_chunk_size(128) == min(1000, (4096 - 3) // 129)
    assert _vadd_batch_chunk_size(32) * (32 + 1) + 3 <= 4096


def test_vadd_batch_splits_oversized_payloads() -> None:
    class FakeRedis:
        def __init__(self) -> None:
            self.calls = []

        def execute_command(self, cmd, *args):
            self.calls.append((cmd, args))
            n = (len(args) - 2) // (int(args[1]) + 1)
            return n

    client = DBXClient.__new__(DBXClient)
    client.r = FakeRedis()
    dim = 32
    n = _vadd_batch_chunk_size(dim) + 40
    ids = [f"v{i}" for i in range(n)]
    vecs = [[0.0] * dim for _ in range(n)]
    inserted = client.vadd_batch("memory", dim, ids, vecs)
    assert inserted == n
    assert len(client.r.calls) == 2
    first_n = (len(client.r.calls[0][1]) - 2) // (dim + 1)
    assert first_n == _vadd_batch_chunk_size(dim)

    class Boom(FakeClient):
        def set(self, key: str, val: str, ex: Optional[int] = None) -> bool:
            return False

    with pytest.raises(DBXError):
        TenantMemory(Boom()).remember("k", "v")
