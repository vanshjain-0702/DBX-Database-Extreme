"""Thin LangChain adapter: one tenant, one RESP connection, fake embeddings.

No OpenAI key. Session KV and vector memory live in the same engine.
Requires `make run-dev`, a provisioned tenant, and:
  pip install redis langchain-core
"""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "sdk", "python"))

from dbx import DBXClient
from langchain_dbx import DBXVectorStore


class _FixedEmbeddings:
    def __init__(self, size: int = 8) -> None:
        self.size = size

    def embed_documents(self, texts):
        return [[float((i + 1) % self.size) for i in range(self.size)] for _ in texts]

    def embed_query(self, text):
        return [0.1] * self.size


def main() -> None:
    tenant = os.environ["DBX_TENANT"]
    key_id = os.environ["DBX_KEY_ID"]
    secret = os.environ["DBX_SECRET"]
    client = DBXClient(
        host="127.0.0.1",
        port=6380,
        tenant=tenant,
        key_id=key_id,
        secret=secret,
    )
    client.set("session:demo", '{"user":"acme","step":1}')
    store = DBXVectorStore(
        client,
        _FixedEmbeddings(size=8),
        index_name="memories",
    )
    store.add_texts(
        [
            "DBX is a per-tenant memory engine for AI products.",
            "Each customer gets an isolated WAL, KV, and HNSW index.",
        ]
    )
    docs = store.similarity_search("isolated memory per customer", k=2)
    print("session:", client.get("session:demo"))
    for doc in docs:
        print("-", doc.page_content)


if __name__ == "__main__":
    main()
