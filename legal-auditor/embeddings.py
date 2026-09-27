"""Caller-side embeddings for Legal Auditor.

DBX never runs a model. Default is a deterministic hash embedding so local
demos work without OpenAI or sentence-transformers. Set EMBEDDING_MODE=openai
and OPENAI_API_KEY for production-quality vectors.
"""

from __future__ import annotations

import hashlib
import math
import os
import struct
from typing import List


DEFAULT_DIM = 64


def chunk_text(text: str, chunk_size: int = 500, overlap: int = 80) -> List[str]:
    text = text.strip()
    if not text:
        return []
    if len(text) <= chunk_size:
        return [text]
    chunks: List[str] = []
    start = 0
    while start < len(text):
        end = min(len(text), start + chunk_size)
        chunks.append(text[start:end].strip())
        if end >= len(text):
            break
        start = max(0, end - overlap)
    return [c for c in chunks if c]


def _hash_embed(text: str, dim: int = DEFAULT_DIM) -> List[float]:
    digest = hashlib.sha256(text.encode("utf-8")).digest()
    vals: List[float] = []
    seed = digest
    while len(vals) < dim:
        for i in range(0, len(seed) - 3, 4):
            n = struct.unpack_from(">I", seed, i)[0]
            vals.append((n / 0xFFFFFFFF) * 2.0 - 1.0)
            if len(vals) >= dim:
                break
        seed = hashlib.sha256(seed).digest()
    # L2 normalize so cosine/ADC paths behave sensibly
    norm = math.sqrt(sum(v * v for v in vals)) or 1.0
    return [v / norm for v in vals]


def embed_texts(texts: List[str], dim: int = DEFAULT_DIM) -> List[List[float]]:
    mode = os.environ.get("EMBEDDING_MODE", "hash").lower()
    if mode == "openai":
        return _openai_embed(texts, dim=dim)
    return [_hash_embed(t, dim=dim) for t in texts]


def embed_query(text: str, dim: int = DEFAULT_DIM) -> List[float]:
    return embed_texts([text], dim=dim)[0]


def _openai_embed(texts: List[str], dim: int = DEFAULT_DIM) -> List[List[float]]:
    try:
        from openai import OpenAI  # type: ignore
    except ImportError as exc:
        raise RuntimeError(
            "EMBEDDING_MODE=openai requires the openai package"
        ) from exc
    api_key = os.environ.get("OPENAI_API_KEY")
    if not api_key:
        raise RuntimeError("OPENAI_API_KEY is required when EMBEDDING_MODE=openai")
    client = OpenAI(api_key=api_key)
    model = os.environ.get("OPENAI_EMBED_MODEL", "text-embedding-3-small")
    resp = client.embeddings.create(model=model, input=texts, dimensions=dim)
    return [list(item.embedding) for item in resp.data]
