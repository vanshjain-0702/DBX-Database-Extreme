from typing import Any, Iterable, List, Optional, Tuple, Type
from langchain_core.documents import Document
from langchain_core.embeddings import Embeddings
from langchain_core.vectorstores import VectorStore
from dbx import DBXClient
import uuid
import json

class DBXVectorStore(VectorStore):
    """DBX Vector Store integration for LangChain."""

    def __init__(self, client: DBXClient, embedding: Embeddings, index_name: str = "default_index"):
        self.client = client
        self.embedding = embedding
        self.index_name = index_name

    def add_texts(
        self,
        texts: Iterable[str],
        metadatas: Optional[List[dict]] = None,
        **kwargs: Any,
    ) -> List[str]:
        """Run more texts through the embeddings and add to the vector store."""
        texts = list(texts)
        if len(texts) == 0:
            return []
            
        embeddings = self.embedding.embed_documents(texts)
        ids = []
        
        pipeline = self.client.r.pipeline(transaction=False)
        
        for i, text in enumerate(texts):
            doc_id = str(uuid.uuid4())
            ids.append(doc_id)
            vector = [float(x) for x in embeddings[i]]
            
            # Store the vector
            pipeline.execute_command("VADD", self.index_name, doc_id, *vector)
            
            # Store the metadata and text
            meta = metadatas[i] if metadatas else {}
            doc_data = {
                "page_content": text,
                "metadata": meta
            }
            pipeline.set(f"doc:{self.index_name}:{doc_id}", json.dumps(doc_data))
            
        pipeline.execute()
        return ids

    def similarity_search(
        self, query: str, k: int = 4, **kwargs: Any
    ) -> List[Document]:
        """Return docs most similar to query."""
        query_embedding = [float(x) for x in self.embedding.embed_query(query)]
        results = self.client.vsearch(self.index_name, query_embedding, top_k=k)
        
        docs = []
        for doc_id, score in results:
            # Fetch the document payload
            doc_str = self.client.get(f"doc:{self.index_name}:{doc_id}")
            if doc_str:
                doc_data = json.loads(doc_str)
                docs.append(Document(
                    page_content=doc_data["page_content"], 
                    metadata=doc_data.get("metadata", {})
                ))
        return docs

    @classmethod
    def from_texts(
        cls,
        texts: List[str],
        embedding: Embeddings,
        metadatas: Optional[List[dict]] = None,
        client: Optional[DBXClient] = None,
        index_name: str = "default_index",
        **kwargs: Any,
    ) -> "DBXVectorStore":
        """Return VectorStore initialized from texts and embeddings."""
        if client is None:
            raise ValueError("DBXClient must be provided to from_texts")
            
        store = cls(client, embedding, index_name)
        store.add_texts(texts, metadatas)
        return store
