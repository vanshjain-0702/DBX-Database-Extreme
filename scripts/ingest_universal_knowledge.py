import os
import sys
import time

from datasets import load_dataset
from langchain_huggingface import HuggingFaceEmbeddings

# Add SDK path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "sdk", "python"))
from dbx import DBXClient
from langchain_dbx import DBXVectorStore

print("Connecting to DBX Orchestrator on localhost:6401...")
# Note: In production we use the certs. Here we will try connecting locally
cert_dir = os.path.join(os.path.dirname(__file__), "..", "certs")
client = DBXClient(
    port=6401,
    ca_cert=os.path.join(cert_dir, "ca.crt"),
    client_cert=os.path.join(cert_dir, "client.crt"),
    client_key=os.path.join(cert_dir, "client.key"),
)
client.ping()
print("Connection successful!")

print("Loading HuggingFace Embeddings Model (all-MiniLM-L6-v2)...")
embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")

datasets_config = {
    "finance": {
        "path": "zeroshot/twitter-financial-news-topic",
        "split": "train",
        "text_column": "text",
        "limit": 500,
    }
}

for index_name, config in datasets_config.items():
    print(f"\n--- Ingesting Domain: {index_name.upper()} ---")
    print(f"Downloading dataset '{config['path']}'...")

    if "name" in config:
        ds = load_dataset(
            config["path"], config["name"], split=config["split"], streaming=True
        )
    else:
        ds = load_dataset(config["path"], split=config["split"], streaming=True)

    texts = []
    for i, row in enumerate(ds):
        if i >= config["limit"]:
            break
        text = row[config["text_column"]]
        if len(text.strip()) > 20:  # Filter out very short texts
            texts.append(text)

    print(f"Extracted {len(texts)} documents. Embedding and pushing to DBX...")

    vector_store = DBXVectorStore(client, embeddings, index_name=index_name)

    t0 = time.time()
    # add_texts will batch embed and send raw TCP VADD commands to DBX
    vector_store.add_texts(texts)

    print(
        f"✅ Successfully indexed {len(texts)} {index_name} documents into DBX in {time.time() - t0:.2f}s!"
    )

print("\n🎉 Universal Knowledge Ingestion Complete!")
