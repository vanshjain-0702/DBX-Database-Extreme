"""Unit tests that do not require a live DBX node."""

from __future__ import annotations

import tempfile
from pathlib import Path

from embeddings import chunk_text, embed_query, embed_texts
from store import FirmStore


def test_chunk_text_overlap() -> None:
    text = "a" * 1200
    chunks = chunk_text(text, chunk_size=500, overlap=80)
    assert len(chunks) >= 2
    assert all(chunks)


def test_hash_embed_normalized_dim() -> None:
    vecs = embed_texts(["hello", "world"], dim=64)
    assert len(vecs) == 2
    assert len(vecs[0]) == 64
    q = embed_query("hello", dim=64)
    assert len(q) == 64
    # identical text → identical vector
    assert embed_query("hello", dim=64) == q


def test_firm_store_register_login_clients() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        store = FirmStore(path=str(Path(tmp) / "store.json"))
        store.register("a@firm.test", "password123", "Firm A")
        token = store.login("a@firm.test", "password123")
        user = store.user_for_token(token)
        assert user is not None
        assert user["email"] == "a@firm.test"
        client = store.add_client("a@firm.test", "Atlas", "la-atlas", "kid", "secret")
        listed = store.list_clients("a@firm.test")
        assert len(listed) == 1
        assert listed[0]["tenant_id"] == "la-atlas"
        got = store.get_client("a@firm.test", client["id"])
        assert got is not None
        assert got["secret"] == "secret"
        store.remove_client("a@firm.test", client["id"])
        assert store.list_clients("a@firm.test") == []
