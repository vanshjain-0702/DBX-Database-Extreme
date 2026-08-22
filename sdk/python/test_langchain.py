import os
from dbx import DBXClient
from langchain_dbx import DBXVectorStore
from langchain_community.embeddings import FakeEmbeddings

def test_dbx_langchain():
    print("Connecting to DBX via mTLS...")
    # Paths to the certificates we generated earlier
    certs_dir = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), "certs")
    ca_cert = os.path.join(certs_dir, "ca.crt")
    client_cert = os.path.join(certs_dir, "client.crt")
    client_key = os.path.join(certs_dir, "client.key")
    
    client = DBXClient(
        host="127.0.0.1", 
        port=6399,
        ca_cert=ca_cert,
        client_cert=client_cert,
        client_key=client_key
    )
    
    if not client.ping():
        print("Failed to connect to DBX!")
        return
    print("Connected successfully!")
    
    # We use FakeEmbeddings for testing so we don't need an OpenAI API key
    embeddings = FakeEmbeddings(size=128)
    
    print("Initializing LangChain DBX VectorStore...")
    dbx_store = DBXVectorStore(client, embeddings, index_name="knowledge_base")
    
    docs = [
        "DBX is a high-performance in-memory database built in Go.",
        "LangChain is a framework for developing applications powered by language models.",
        "Vector databases are essential for Retrieval-Augmented Generation (RAG)."
    ]
    
    print("Adding documents to DBX...")
    dbx_store.add_texts(docs, metadatas=[{"source": "doc1"}, {"source": "doc2"}, {"source": "doc3"}])
    
    query = "Tell me about the DBX database."
    print(f"Querying DBX for: '{query}'")
    
    results = dbx_store.similarity_search(query, k=1)
    
    print("\n--- Top Result ---")
    if results:
        print(f"Content: {results[0].page_content}")
        print(f"Metadata: {results[0].metadata}")
    else:
        print("No results found.")
        
if __name__ == "__main__":
    test_dbx_langchain()
