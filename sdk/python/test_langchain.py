"""Manual live check against a provisioned tenant. Not part of `go test`.

Requires `make run-dev` and:
  DBX_TENANT, DBX_KEY_ID, DBX_SECRET
"""

from __future__ import annotations

import os
import sys

from dbx import DBXClient
from langchain_dbx import DBXVectorStore


class _FixedEmbeddings:
    def __init__(self, size: int = 8) -> None:
        self.size = size

    def embed_documents(self, texts):
        return [[float((i + 1) % self.size) for i in range(self.size)] for _ in texts]

    def embed_query(self, text):
        return [0.1] * self.size


def test_dbx_langchain() -> None:
    tenant = os.environ.get("DBX_TENANT")
    key_id = os.environ.get("DBX_KEY_ID")
    secret = os.environ.get("DBX_SECRET")
    if not tenant or not key_id or not secret:
        print("skip: set DBX_TENANT, DBX_KEY_ID, DBX_SECRET")
        return

    client = DBXClient(
        host="127.0.0.1",
        port=6380,
        tenant=tenant,
        key_id=key_id,
        secret=secret,
    )
    if not client.ping():
        print("Failed to AUTH to :6380")
        sys.exit(1)

    store = DBXVectorStore(client, _FixedEmbeddings(size=8), index_name="knowledge_base")
    store.add_texts(
        [
            "DBX is a per-tenant memory engine for AI products.",
            "Each customer gets an isolated WAL, KV, and HNSW index.",
        ]
    )
    results = store.similarity_search("isolated memory per customer", k=1)
    if not results:
        print("No results found.")
        sys.exit(1)
    print(results[0].page_content)


if __name__ == "__main__":
    test_dbx_langchain()
