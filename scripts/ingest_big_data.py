import os
import sys
import time

from datasets import load_dataset
from langchain_huggingface import HuggingFaceEmbeddings

# Add SDK path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "sdk", "python"))
from dbx import DBXClient
from langchain_dbx import DBXVectorStore


def main():
    print("Connecting to DBX Orchestrator on localhost:6401...")
    client = DBXClient(
        port=6401,
        password=os.environ.get("DBX_DEFAULT_PASSWORD", "adminadminadmin"),
    )
    client.ping()
    print("Connection successful!")

    print("Loading HuggingFace Embeddings Model (all-MiniLM-L6-v2, dim 384)...")
    embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")

    config = {
        "path": "fancyzhx/ag_news",
        "split": "train",
        "text_column": "text",
        "limit": 30000,  # The big website simulation dataset size
    }

    index_name = "big_web_index"

    print(f"\n--- Ingesting Domain: {index_name.upper()} ---")
    print(f"Downloading dataset '{config['path']}'...")

    ds = load_dataset(config["path"], split=config["split"], streaming=True)

    texts = []
    print(f"Extracting {config['limit']} documents...")
    t_extract_start = time.time()
    for i, row in enumerate(ds):
        if i >= config["limit"]:
            break
        text = row[config["text_column"]]
        if len(text.strip()) > 20:  # Filter out very short texts
            texts.append(text)

    print(f"Extracted {len(texts)} documents in {time.time() - t_extract_start:.2f}s.")
    print("Embedding and pushing to DBX in batches... (This may take several minutes)")

    vector_store = DBXVectorStore(client, embeddings, index_name=index_name)

    # We batch push them in chunks to show progress
    batch_size = 2000
    total_docs = len(texts)

    t0 = time.time()

    for i in range(0, total_docs, batch_size):
        batch_texts = texts[i : i + batch_size]
        print(f"Processing batch {i} to {i+len(batch_texts)} out of {total_docs}...")
        t_batch_start = time.time()
        # add_texts will embed and send raw TCP VADD_BATCH commands to DBX
        vector_store.add_texts(batch_texts)
        print(f"  Batch inserted in {time.time() - t_batch_start:.2f}s")

    print(
        f" Successfully indexed {total_docs} documents into DBX in {time.time() - t0:.2f}s!"
    )
    print(
        f"Average ingestion speed: {total_docs / (time.time() - t0):.2f} docs/sec (including local embedding time)"
    )
    print("\n🎉 Universal Knowledge Ingestion Complete!")


if __name__ == "__main__":
    main()
