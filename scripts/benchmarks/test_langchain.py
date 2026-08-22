import os
import sys

# Add the sdk to PYTHONPATH so we can import dbx
sys.path.append(os.path.join(os.path.dirname(__file__), "sdk", "python"))

from dbx import DBXClient
from langchain_dbx import DBXVectorStore


# Fake Embeddings class for testing
class MockEmbeddings:
    def embed_documents(self, texts):
        return [[len(t) * 0.1, 0.5, 0.5, 0.5] for t in texts]

    def embed_query(self, text):
        return [len(text) * 0.1, 0.5, 0.5, 0.5]


print("Connecting to DBX...")
# The orchestrator starts test-tenant on RESP 6401
client = DBXClient(
    port=6401,
    ca_cert="./certs/ca.crt",
    client_cert="./certs/client.crt",
    client_key="./certs/client.key",
)
try:
    client.ping()
    print("Ping successful!")
except Exception as e:
    print(f"Ping failed: {e}")
    sys.exit(1)

print("Initializing Langchain VectorStore...")
store = DBXVectorStore(client, MockEmbeddings(), index_name="test_index")

texts = ["hello world", "database performance", "enterprise features"]
print("Adding texts...")
ids = store.add_texts(texts)
print(f"Added texts with IDs: {ids}")

print("Performing similarity search...")
results = store.similarity_search("hello", k=2)
print("Search results:")
for r in results:
    print(r.page_content)

print("SUCCESS: Langchain SDK is working.")
