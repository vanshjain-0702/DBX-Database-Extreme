"""LangChain adapter tests. Live node tests skip without AUTH env."""

from __future__ import annotations

import os

import pytest
from langchain_core.embeddings import Embeddings

from dbx import DBXClient
from langchain_dbx import DBXVectorStore


class _FixedEmbeddings(Embeddings):
    def __init__(self, size: int = 8) -> None:
        self.size: int = size

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        return [[float((i + 1) % self.size) for i in range(self.size)] for _ in texts]

    def embed_query(self, text: str) -> list[float]:
        return [0.1] * self.size


def test_from_texts_requires_client() -> None:
    with pytest.raises(ValueError, match="DBXClient"):
        DBXVectorStore.from_texts(["hello"], _FixedEmbeddings())


def test_live_langchain() -> None:
    tenant = os.environ.get("DBX_TENANT")
    key_id = os.environ.get("DBX_KEY_ID")
    secret = os.environ.get("DBX_SECRET")
    if not tenant or not key_id or not secret:
        pytest.skip("set DBX_TENANT, DBX_KEY_ID, DBX_SECRET against a running node")

    client = DBXClient(
        host="127.0.0.1",
        port=6380,
        tenant=tenant,
        key_id=key_id,
        secret=secret,
    )
    if not client.ping():
        pytest.fail("Failed to AUTH to :6380")

    store = DBXVectorStore(
        client, _FixedEmbeddings(size=8), index_name="knowledge_base"
    )
    store.add_texts(
        [
            "DBX is a per-tenant memory engine for AI products.",
            "Each customer gets an isolated WAL, KV, and HNSW index.",
        ]
    )
    results = store.similarity_search("isolated memory per customer", k=1)
    assert results
    assert results[0].page_content
