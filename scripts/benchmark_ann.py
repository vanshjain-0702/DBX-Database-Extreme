import os
import random
import sys
import time

try:
    import numpy as np
    from sklearn.datasets import make_blobs
except ImportError:
    print("Please install numpy and scikit-learn: pip install numpy scikit-learn")
    sys.exit(1)

sys.path.append(os.path.join(os.path.dirname(__file__), "..", "sdk", "python"))
from dbx import DBXClient


def normalize(v):
    norm = np.linalg.norm(v)
    if norm == 0:
        return v
    return v / norm


def exact_search(query, dataset, k=5):
    # compute cosine similarity: dot product since vectors are normalized
    scores = np.dot(dataset, query)
    top_indices = np.argsort(scores)[::-1][:k]
    return top_indices, scores[top_indices]


def main():
    print("Generating highly real-world synthetic data (clustered)...")
    n_samples = 10000
    n_features = 128
    n_clusters = 50
    X, _ = make_blobs(
        n_samples=n_samples, n_features=n_features, centers=n_clusters, random_state=42
    )

    # Normalize vectors for cosine similarity
    X = np.array([normalize(v) for v in X])

    client = DBXClient(
        host="127.0.0.1",
        port=6401,
        password="strong-engine-password123",
        ca_cert="certs/ca.crt",
        client_cert="certs/client.crt",
        client_key="certs/client.key",
    )

    try:
        client.ping()
    except Exception:
        print(
            "DBX server is not running on 6399 or certs are missing. Start the server first!"
        )
        sys.exit(1)

    print("Server connected.")

    index_name = f"ann_benchmark_{int(time.time())}"

    # Ingestion
    print(f"Ingesting {n_samples} vectors into DBX...")
    start_time = time.time()
    batch_size = 500
    for i in range(0, n_samples, batch_size):
        end = min(i + batch_size, n_samples)
        batch_X = X[i:end]
        doc_ids = [f"doc_{j}" for j in range(i, end)]
        vectors = [v.tolist() for v in batch_X]
        client.vadd_batch(index_name, n_features, doc_ids, vectors)
        time.sleep(0.05)  # Slow down slightly to observe in dashboard

    ingest_time = time.time() - start_time

    print(
        f"Ingestion complete. Time: {ingest_time:.2f}s | Speed: {n_samples/ingest_time:.2f} ops/sec"
    )

    # Generate test queries
    n_queries = 100
    queries = []
    for _ in range(n_queries):
        idx = random.randint(0, n_samples - 1)
        # Add some noise to a known vector to simulate a real query
        noise = np.random.normal(0, 0.1, n_features)
        q = normalize(X[idx] + noise)
        queries.append(q)

    print(f"Running {n_queries} queries to measure Recall@5 and Latency...")

    k = 5
    true_positives = 0
    false_positives = 0
    total_dbx_latency = 0

    for q in queries:
        # Exact search ground truth
        exact_indices, _ = exact_search(q, X, k)
        expected_ids = set([f"doc_{i}" for i in exact_indices])

        # DBX ANN Search
        t0 = time.time()
        res = client.vsearch(index_name, q.tolist(), top_k=k)
        total_dbx_latency += time.time() - t0

        retrieved_ids = set([r[0] if isinstance(r, tuple) else r["id"] for r in res])

        tp = len(expected_ids.intersection(retrieved_ids))
        fp = len(retrieved_ids) - tp

        true_positives += tp
        false_positives += fp

    avg_latency = (total_dbx_latency / n_queries) * 1000
    qps = n_queries / total_dbx_latency
    recall = true_positives / (n_queries * k)

    print("\n" + "=" * 40)
    print("      ANN PERFORMANCE REPORT")
    print("=" * 40)
    print(f"Index Size:       {n_samples} vectors")
    print(f"Dimensions:       {n_features}")
    print(f"Ingest Speed:     {n_samples/ingest_time:.2f} v/sec")
    print(f"Avg Q. Latency:   {avg_latency:.2f} ms")
    print(f"Queries Per Sec:  {qps:.2f} QPS")
    print("\n--- CONFUSION MATRIX (Top-5) ---")
    print(f"True Positives:   {true_positives}")
    print(f"False Positives:  {false_positives}")
    print(f"Recall@{k}:        {recall*100:.2f}%")
    print("=" * 40)


if __name__ == "__main__":
    main()
