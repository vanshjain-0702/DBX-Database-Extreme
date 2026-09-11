"""
langchain_dbx.py — LangChain VectorStore adapter for DBX
=========================================================
Drop-in replacement for PineconeVectorStore / Chroma / Qdrant.

Swap from Pinecone with ONE line:
    # Before
    from langchain_pinecone import PineconeVectorStore as VectorStore
    # After
    from langchain_dbx import DBXVectorStore as VectorStore

Everything else stays exactly the same.
"""

from __future__ import annotations

import json
import uuid
from typing import Any, Dict, Iterable, List, Optional, Tuple

from dbx import DBXClient
from langchain_core.documents import Document
from langchain_core.embeddings import Embeddings
from langchain_core.vectorstores import VectorStore


class DBXVectorStore(VectorStore):
    """LangChain VectorStore backed by an isolated DBX tenant.

    This adapter is API-compatible with PineconeVectorStore, Chroma,
    and Qdrant so that users can swap their vector store with one import line.

    Quick start::

        from langchain_dbx import DBXVectorStore
        from langchain_openai import OpenAIEmbeddings
        from dbx import DBXClient

        client = DBXClient(host="localhost", port=6380,
                           tenant="my-tenant", key_id="k1", secret="s1")
        store = DBXVectorStore(client=client, embedding=OpenAIEmbeddings())

        store.add_texts(["Hello world", "DBX is fast"])
        docs = store.similarity_search("fast database", k=3)
    """

    # ------------------------------------------------------------------ #
    # Construction                                                         #
    # ------------------------------------------------------------------ #

    def __init__(
        self,
        client: DBXClient,
        embedding: Embeddings,
        index_name: str = "lc_index",
        *,
        namespace: str = "",  # Pinecone compat — ignored but accepted
        text_key: str = "page_content",
    ) -> None:
        self._client = client
        self._embedding = embedding
        self._index = index_name
        self._text_key = text_key

    # ------------------------------------------------------------------ #
    # Required property                                                    #
    # ------------------------------------------------------------------ #

    @property
    def embeddings(self) -> Embeddings:
        return self._embedding

    # ------------------------------------------------------------------ #
    # Internal helpers                                                     #
    # ------------------------------------------------------------------ #

    def _meta_key(self, doc_id: str) -> str:
        return f"lc:{self._index}:{doc_id}"

    def _store_doc(self, doc_id: str, text: str, metadata: Dict[str, Any]) -> None:
        payload = {self._text_key: text, "metadata": metadata}
        self._client.set(self._meta_key(doc_id), json.dumps(payload))

    def _load_doc(self, doc_id: str) -> Optional[Document]:
        raw = self._client.get(self._meta_key(doc_id))
        if raw is None:
            return None
        data = json.loads(raw)
        return Document(
            page_content=data.get(self._text_key, ""),
            metadata=data.get("metadata", {}),
        )

    # ------------------------------------------------------------------ #
    # Core write path                                                      #
    # ------------------------------------------------------------------ #

    def add_texts(
        self,
        texts: Iterable[str],
        metadatas: Optional[List[Dict[str, Any]]] = None,
        ids: Optional[List[str]] = None,
        **kwargs: Any,
    ) -> List[str]:
        """Embed and store texts in DBX using efficient batch ingestion."""
        texts_list = list(texts)
        if not texts_list:
            return []

        metas = metadatas or [{} for _ in texts_list]
        doc_ids = ids or [str(uuid.uuid4()) for _ in texts_list]

        # Embed all texts in a single call to the embedding model
        vectors = self._embedding.embed_documents(texts_list)

        # ── batch-write vectors via VADD_BATCH (far faster than N×VADD) ──
        dim = len(vectors[0])
        self._client.vadd_batch(self._index, dim, doc_ids, vectors)

        # ── store raw text + metadata in KV using a pipeline ──
        pipeline = self._client.r.pipeline(transaction=False)
        for doc_id, text, meta in zip(doc_ids, texts_list, metas):
            payload = json.dumps({self._text_key: text, "metadata": meta})
            pipeline.set(self._meta_key(doc_id), payload)
        pipeline.execute()

        return doc_ids

    def add_documents(
        self,
        documents: List[Document],
        ids: Optional[List[str]] = None,
        **kwargs: Any,
    ) -> List[str]:
        """Pinecone-compatible: accept Document objects directly."""
        texts = [d.page_content for d in documents]
        metas = [d.metadata for d in documents]
        return self.add_texts(texts, metas, ids=ids, **kwargs)

    def delete(self, ids: List[str], **kwargs: Any) -> Optional[bool]:
        """Remove vectors and their metadata by document ID."""
        pipeline = self._client.r.pipeline(transaction=False)
        for doc_id in ids:
            self._client.vdel(self._index, doc_id)
            pipeline.delete(self._meta_key(doc_id))
        pipeline.execute()
        return True

    # ------------------------------------------------------------------ #
    # Search                                                               #
    # ------------------------------------------------------------------ #

    def similarity_search(
        self,
        query: str,
        k: int = 4,
        filter: Optional[Dict[str, Any]] = None,  # Pinecone compat, reserved
        **kwargs: Any,
    ) -> List[Document]:
        return [
            doc for doc, _ in self.similarity_search_with_score(query, k=k, **kwargs)
        ]

    def similarity_search_with_score(
        self,
        query: str,
        k: int = 4,
        **kwargs: Any,
    ) -> List[Tuple[Document, float]]:
        """Return (Document, similarity_score) pairs. Pinecone-compatible."""
        q_vec = [float(x) for x in self._embedding.embed_query(query)]
        hits = self._client.vsearch(
            self._index,
            q_vec,
            top_k=k,
            min_score=kwargs.get("min_score"),
            space=kwargs.get("space"),
            ef=kwargs.get("ef"),
        )
        results: List[Tuple[Document, float]] = []
        for doc_id, score in hits:
            doc = self._load_doc(doc_id)
            if doc is not None:
                results.append((doc, score))
        return results

    def similarity_search_by_vector(
        self,
        embedding: List[float],
        k: int = 4,
        **kwargs: Any,
    ) -> List[Document]:
        """Pinecone-compatible: search with a pre-computed embedding vector."""
        hits = self._client.vsearch(self._index, embedding, top_k=k)
        docs = []
        for doc_id, _ in hits:
            doc = self._load_doc(doc_id)
            if doc is not None:
                docs.append(doc)
        return docs

    # ------------------------------------------------------------------ #
    # Async stubs (needed for LangGraph / LCEL async chains)              #
    # ------------------------------------------------------------------ #

    async def aadd_texts(self, texts: Iterable[str], **kwargs: Any) -> List[str]:
        # DBX uses a synchronous Redis client; run in executor for async compat
        import asyncio
        import functools

        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(
            None, functools.partial(self.add_texts, list(texts), **kwargs)
        )

    async def asimilarity_search(
        self, query: str, k: int = 4, **kwargs: Any
    ) -> List[Document]:
        import asyncio
        import functools

        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(
            None, functools.partial(self.similarity_search, query, k=k, **kwargs)
        )

    # ------------------------------------------------------------------ #
    # Class-method constructors (Pinecone / Chroma style)                 #
    # ------------------------------------------------------------------ #

    @classmethod
    def from_texts(
        cls,
        texts: List[str],
        embedding: Embeddings,
        metadatas: Optional[List[Dict[str, Any]]] = None,
        *,
        client: Optional[DBXClient] = None,
        index_name: str = "lc_index",
        ids: Optional[List[str]] = None,
        **kwargs: Any,
    ) -> "DBXVectorStore":
        """Pinecone-compatible class-method factory."""
        if client is None:
            raise ValueError(
                "Provide a DBXClient. Example:\n"
                "  client = DBXClient(host='localhost', port=6380, "
                "tenant='t1', key_id='k1', secret='s1')"
            )
        store = cls(client=client, embedding=embedding, index_name=index_name)
        store.add_texts(texts, metadatas, ids=ids)
        return store

    @classmethod
    def from_documents(
        cls,
        documents: List[Document],
        embedding: Embeddings,
        *,
        client: Optional[DBXClient] = None,
        index_name: str = "lc_index",
        **kwargs: Any,
    ) -> "DBXVectorStore":
        """Pinecone-compatible: build a store directly from Document objects."""
        texts = [d.page_content for d in documents]
        metas = [d.metadata for d in documents]
        return cls.from_texts(
            texts, embedding, metas, client=client, index_name=index_name, **kwargs
        )

    @classmethod
    def from_existing_index(
        cls,
        index_name: str,
        embedding: Embeddings,
        *,
        client: DBXClient,
        **kwargs: Any,
    ) -> "DBXVectorStore":
        """Pinecone-compatible: connect to a DBX tenant that already has data."""
        return cls(client=client, embedding=embedding, index_name=index_name)

    # ------------------------------------------------------------------ #
    # Retriever helpers                                                    #
    # ------------------------------------------------------------------ #

    def as_retriever(self, **kwargs: Any):  # type: ignore[override]
        """Return a LangChain retriever (works with LCEL | chains)."""
        from langchain_core.vectorstores import VectorStoreRetriever

        return VectorStoreRetriever(vectorstore=self, **kwargs)
