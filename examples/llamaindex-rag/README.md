# LlamaIndex RAG with DBX

Demonstrates using DBX as the vector store backend for a LlamaIndex RAG pipeline.

## One-line swap from Pinecone

```python
# Before
from llama_index.vector_stores.pinecone import PineconeVectorStore as VectorStore
# After — nothing else changes
from llamaindex_dbx import DBXVectorStore as VectorStore
```

## Setup

```bash
pip install -e "../../sdk/python[llamaindex]"
pip install llama-index-embeddings-openai llama-index-llms-openai   # optional
```

## Run

```bash
# With OpenAI (full RAG with LLM response)
export OPENAI_API_KEY=sk-...
export DBX_ADMIN_PASSWORD=adminadminadmin
python rag_example.py

# Without OpenAI key (demo mode — random embeddings, no LLM)
export DBX_ADMIN_PASSWORD=adminadminadmin
python rag_example.py
```

## What it does

1. Provisions a fresh isolated DBX tenant.
2. Indexes 7 DBX documentation sentences using `VectorStoreIndex`.
3. Runs 3 similarity queries using `VectorStoreQuery`.
4. Shreds the tenant and all its data on exit.

Each tenant's data is physically isolated at the OS level — no other tenant
can ever access these embeddings, even if there is a bug in the query layer.
