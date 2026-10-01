"""Feature-hash embeddings for Jurix.

DBX stores the floats it is given and does not run a model. These vectors
use a hashing trick over tokens so a checklist query lands near the clause
that shares its words. They are L2-normalized for cosine search.
"""

from __future__ import annotations

import hashlib
import math
import re
from typing import List

DIM = 64

_STOP = {
    "a", "an", "and", "are", "as", "at", "be", "by", "for", "from", "if",
    "in", "is", "it", "its", "of", "on", "or", "that", "the", "this", "to",
    "under", "with", "shall", "any", "such", "not", "other", "than",
}


def _tokens(text: str) -> List[str]:
    words = re.findall(r"[a-z0-9]+", text.lower())
    return [w for w in words if w not in _STOP and len(w) > 1]


def embed(text: str, dim: int = DIM) -> List[float]:
    vec = [0.0] * dim
    tokens = _tokens(text)
    if not tokens:
        vec[0] = 1.0
        return vec
    for tok in tokens:
        digest = hashlib.sha256(tok.encode("utf-8")).digest()
        bucket = int.from_bytes(digest[:4], "big") % dim
        sign = 1.0 if digest[4] & 1 else -1.0
        vec[bucket] += sign
    for left, right in zip(tokens, tokens[1:]):
        digest = hashlib.sha256(f"{left}_{right}".encode("utf-8")).digest()
        bucket = int.from_bytes(digest[:4], "big") % dim
        sign = 0.65 if digest[4] & 1 else -0.65
        vec[bucket] += sign
    norm = math.sqrt(sum(v * v for v in vec)) or 1.0
    return [v / norm for v in vec]


def embed_texts(texts: List[str], dim: int = DIM) -> List[List[float]]:
    return [embed(t, dim=dim) for t in texts]
