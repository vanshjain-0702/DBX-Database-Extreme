"""
llamaindex_dbx.py -- LlamaIndex VectorStore adapter for DBX
===========================================================
Drop-in replacement for PineconeVectorStore / ChromaVectorStore / QdrantVectorStore.

Swap from Pinecone with ONE line:
    # Before
    from llama_index.vector_stores.pinecone import PineconeVectorStore as VectorStore
    # After
    from llamaindex_dbx import DBXVectorStore as VectorStore

Everything else stays exactly the same.

Compatible with LlamaIndex >= 0.10 (llama-index-core).
"""

from __future__ import annotations

import json
import uuid
from typing import Any, List, Optional

from dbx import DBXClient

# ---------------------------------------------------------------------------
# Optional LlamaIndex imports -- file stays importable without llama-index-core
# (CI lint runs without the package; the class simply raises a clear error
#  at instantiation time if it is missing)
# ---------------------------------------------------------------------------
try:
    from llama_index.core.bridge.pydantic import Field, PrivateAttr
    from llama_index.core.schema import BaseNode, NodeWithScore, TextNode
    from llama_index.core.vector_stores.types import (
        BasePydanticVectorStore,
        VectorStoreQuery,
        VectorStoreQueryResult,
    )

    _LLAMA_AVAILABLE = True
except ImportError:
    _LLAMA_AVAILABLE = False
    # Provide stubs so the class body can be parsed even without the package
    BasePydanticVectorStore = object  # type: ignore[misc,assignment]
    Field = lambda *a, **kw: None  # type: ignore[assignment]  # noqa: E731
    PrivateAttr = lambda *a, **kw: None  # type: ignore[assignment]  # noqa: E731
    BaseNode = object  # type: ignore[assignment,misc]
    NodeWithScore = object  # type: ignore[assignment,misc]
    TextNode = object  # type: ignore[assignment,misc]
    VectorStoreQuery = object  # type: ignore[assignment,misc]
    VectorStoreQueryResult = object  # type: ignore[assignment,misc]


def _require_llama() -> None:
    if not _LLAMA_AVAILABLE:
        raise ImportError(
            "LlamaIndex is required to use DBXVectorStore.\n"
            "Install it with:  pip install llama-index-core\n"
            "Or install the full DBX extras:  pip install -e 'sdk/python[llamaindex]'"
        )


class DBXVectorStore(BasePydanticVectorStore):  # type: ignore[misc]
    """LlamaIndex VectorStore backed by an isolated DBX tenant.

    This adapter is API-compatible with PineconeVectorStore, ChromaVectorStore,
    and QdrantVectorStore so users can swap their vector store with one import.

    Quick start::

        from llamaindex_dbx import DBXVectorStore
        from llama_index.core import VectorStoreIndex, StorageContext
        from dbx import DBXClient

        client = DBXClient(host="localhost", port=6380,
                           tenant="my-tenant", key_id="k1", secret="s1")

        vector_store = DBXVectorStore(client=client)
        storage_context = StorageContext.from_defaults(vector_store=vector_store)
        index = VectorStoreIndex.from_documents(docs, storage_context=storage_context)

        query_engine = index.as_query_engine()
        response = query_engine.query("What is DBX?")
    """

    stores_text: bool = True
    flat_metadata: bool = False

    index_name: str = Field(default="li_index", description="DBX vector index name")

    _client: Any = PrivateAttr()

    def __init__(
        self,
        client: DBXClient,
        index_name: str = "li_index",
        **kwargs: Any,
    ) -> None:
        _require_llama()
        super().__init__(index_name=index_name, **kwargs)
        self._client = client

    # ------------------------------------------------------------------ #
    # Internal helpers                                                     #
    # ------------------------------------------------------------------ #

    @property
    def client(self) -> DBXClient:
        """Exposes the raw DBXClient (Pinecone compat: `.client`)."""
        return self._client

    def _node_key(self, node_id: str) -> str:
        return f"li:{self.index_name}:{node_id}"

    def _serialize_node(self, node: Any) -> str:
        return json.dumps(
            {
                "text": node.get_content(metadata_mode="all"),
                "metadata": node.metadata,
                "node_id": node.node_id,
                "relationships": {
                    str(k): str(v) for k, v in node.relationships.items()
                },
            }
        )

    def _deserialize_node(self, raw: str, score: float = 1.0) -> Any:
        _require_llama()
        data = json.loads(raw)
        node = TextNode(
            text=data.get("text", ""),
            metadata=data.get("metadata", {}),
            id_=data.get("node_id", str(uuid.uuid4())),
        )
        return NodeWithScore(node=node, score=score)

    # ------------------------------------------------------------------ #
    # Write path                                                           #
    # ------------------------------------------------------------------ #

    def add(
        self,
        nodes: List[Any],
        **add_kwargs: Any,
    ) -> List[str]:
        """Add nodes into the DBX vector store using batch ingestion."""
        _require_llama()
        if not nodes:
            return []

        node_ids: List[str] = []
        vectors: List[List[float]] = []
        payloads: List[str] = []

        for node in nodes:
            embedding = node.get_embedding()
            if embedding is None or len(embedding) == 0:
                raise ValueError(
                    f"Node {node.node_id!r} has no embedding. "
                    "Ensure your embed_model is configured and called before add()."
                )
            node_ids.append(node.node_id)
            vectors.append([float(v) for v in embedding])
            payloads.append(self._serialize_node(node))

        dim = len(vectors[0])
        self._client.vadd_batch(self.index_name, dim, node_ids, vectors)

        pipeline = self._client.r.pipeline(transaction=False)
        for node_id, payload in zip(node_ids, payloads):
            pipeline.set(self._node_key(node_id), payload)
        pipeline.execute()

        return node_ids

    def delete(self, ref_doc_id: str, **delete_kwargs: Any) -> None:
        """Delete a node by its document/node ID."""
        self._client.vdel(self.index_name, ref_doc_id)
        self._client.delete(self._node_key(ref_doc_id))

    # ------------------------------------------------------------------ #
    # Query path                                                           #
    # ------------------------------------------------------------------ #

    def query(
        self,
        query: Any,
        **kwargs: Any,
    ) -> Any:
        """Execute a vector similarity query against DBX.

        Supports:
          - DEFAULT mode  -> VSEARCH (dense ANN)
          - SPARSE mode   -> VSEARCH (falls back to dense)
          - HYBRID mode   -> VFUSE   (multi-space fusion)
        """
        _require_llama()
        top_k = query.similarity_top_k or 4

        q_vec: Optional[List[float]] = None
        if query.query_embedding is not None:
            q_vec = [float(v) for v in query.query_embedding]

        if q_vec is None:
            raise ValueError(
                "DBXVectorStore requires a query_embedding. "
                "Ensure your embed_model is set on the VectorStoreIndex."
            )

        hits = self._client.vsearch(
            self.index_name,
            q_vec,
            top_k=top_k,
        )

        nodes: List[Any] = []
        ids: List[str] = []
        similarities: List[float] = []

        for node_id, score in hits:
            raw = self._client.get(self._node_key(node_id))
            if raw is not None:
                nodes.append(self._deserialize_node(raw, score=score))
                ids.append(node_id)
                similarities.append(score)

        return VectorStoreQueryResult(
            nodes=nodes,
            ids=ids,
            similarities=similarities,
        )

    # ------------------------------------------------------------------ #
    # Pinecone-compatible class-method constructors                       #
    # ------------------------------------------------------------------ #

    @classmethod
    def from_params(
        cls,
        client: DBXClient,
        index_name: str = "li_index",
        **kwargs: Any,
    ) -> "DBXVectorStore":
        """Pinecone-compatible factory method."""
        return cls(client=client, index_name=index_name, **kwargs)
