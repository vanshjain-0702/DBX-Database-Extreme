"""
LlamaIndex RAG example with DBX as the vector store.

One-line swap from Pinecone:
    # Before: from llama_index.vector_stores.pinecone import PineconeVectorStore as VS
    from llamaindex_dbx import DBXVectorStore as VS

Setup:
    pip install -e "../../sdk/python[llamaindex]"
    pip install llama-index-embeddings-openai llama-index-llms-openai

Run:
    export OPENAI_API_KEY=sk-...
    export DBX_ADMIN_PASSWORD=adminadminadmin
    python rag_example.py
"""

import os
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "../../sdk/python"))

from dbx import ControlPlane, DBXClient  # noqa: E402

# -- LlamaIndex imports ------------------------------------------------------
try:
    from llama_index.core import VectorStoreIndex, StorageContext
    from llamaindex_dbx import DBXVectorStore
    from llama_index.core.vector_stores.types import VectorStoreQuery
except ImportError:
    print("Install: pip install -e '../../sdk/python[llamaindex]'")
    sys.exit(1)

# -- Config ------------------------------------------------------------------
ORCHESTRATOR_URL = os.getenv("DBX_URL", "http://127.0.0.1:8000")
ADMIN_PASSWORD = os.getenv("DBX_ADMIN_PASSWORD", "adminadminadmin")
RESP_HOST = os.getenv("DBX_HOST", "127.0.0.1")
RESP_PORT = int(os.getenv("DBX_PORT", "6380"))
TENANT_ID = "llamaindex-rag-demo"

# -- Provision tenant --------------------------------------------------------
plane = ControlPlane(ORCHESTRATOR_URL)
plane.login("admin", ADMIN_PASSWORD)
try:
    plane.provision(TENANT_ID, "LlamaIndex RAG Demo")
except Exception as e:
    if "already" not in str(e).lower():
        raise

minted = plane.create_key(TENANT_ID, name="writer", role="writer")
key = minted.get("key") or {}
client = DBXClient(
    host=RESP_HOST,
    port=RESP_PORT,
    tenant=TENANT_ID,
    key_id=str(key.get("id") or minted.get("id") or ""),
    secret=str(minted.get("secret") or ""),
)

# Wait for tenant to come online
for _ in range(20):
    try:
        client.ping()
        break
    except Exception:
        time.sleep(0.5)

# -- Build VectorStore -------------------------------------------------------
vector_store = DBXVectorStore(client=client, index_name="rag_index")
storage_context = StorageContext.from_defaults(vector_store=vector_store)

# -- Sample documents --------------------------------------------------------
DOCUMENTS = [
    "DBX is a per-tenant, OS-isolated AI memory engine built in Go.",
    "Each DBX tenant gets its own isolated process secured with Linux Landlock.",
    "DBX supports both KV state and HNSW vector recall in one engine.",
    "DBX uses SQ8 scalar quantization to reduce vector memory footprint by 75%.",
    "DBX achieves 10,000+ requests per second under concurrent multi-tenant load.",
    "The DBX isolation kernel uses Cgroups v2 to bound per-tenant memory usage.",
    "DBX is a drop-in replacement for running Redis + Pinecone side by side.",
]

# -- Embedding model ---------------------------------------------------------
OPENAI_KEY = os.getenv("OPENAI_API_KEY")
if OPENAI_KEY:
    from llama_index.embeddings.openai import OpenAIEmbedding

    embed_model = OpenAIEmbedding()
    print("Using OpenAI embeddings.")
else:
    from llama_index.core.embeddings import MockEmbedding

    print("No OPENAI_API_KEY found. Using MockEmbedding for demo purposes.")
    embed_model = MockEmbedding(embed_dim=64)

# -- Index documents ---------------------------------------------------------
print(f"\nIndexing {len(DOCUMENTS)} documents into DBX tenant '{TENANT_ID}'...")
t0 = time.perf_counter()

from llama_index.core import Document  # noqa: E402

li_docs = [Document(text=d) for d in DOCUMENTS]
index = VectorStoreIndex.from_documents(
    li_docs,
    storage_context=storage_context,
    embed_model=embed_model,
)

elapsed = time.perf_counter() - t0
print(f"Indexed in {elapsed:.3f}s")

# -- Query -------------------------------------------------------------------
queries = [
    "How does DBX isolate tenants?",
    "What is the memory footprint of DBX?",
    "How fast is DBX under load?",
]

print("\n--- Vector Recall Results ---")
for q in queries:
    q_vec = embed_model.get_query_embedding(q)

    result = vector_store.query(
        VectorStoreQuery(query_embedding=q_vec, similarity_top_k=2)
    )
    print(f"\nQ: {q}")
    for node, score in zip(result.nodes or [], result.similarities or []):
        print(f"  [{score:.4f}] {node.get_content()[:80]}")

# -- Cleanup -----------------------------------------------------------------
print("\nShredding demo tenant...")
plane.shred(TENANT_ID)
print("Done.")
