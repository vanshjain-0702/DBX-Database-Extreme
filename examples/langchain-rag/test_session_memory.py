"""Unit checks for the LangChain session example. Live AUTH is optional."""

from __future__ import annotations

import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "sdk", "python"))

from session_memory import _FixedEmbeddings, _require_env  # noqa: E402


def test_require_env_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("DBX_TENANT", raising=False)
    with pytest.raises(SystemExit, match="DBX_TENANT"):
        _require_env("DBX_TENANT")


def test_fixed_embeddings_dimension() -> None:
    emb = _FixedEmbeddings(size=8)
    assert len(emb.embed_query("q")) == 8
    docs = emb.embed_documents(["a", "b"])
    assert len(docs) == 2
    assert len(docs[0]) == 8
