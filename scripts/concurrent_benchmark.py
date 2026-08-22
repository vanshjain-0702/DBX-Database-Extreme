import sys
import os
import time
import threading
import numpy as np

sys.path.append(os.path.join(os.path.dirname(__file__), '..', 'sdk', 'python'))
from dbx import DBXClient

INDEX_NAME = "concurrent_index"
NUM_VECTORS = 10000
DIMENSION = 384
BATCH_SIZE = 1000

print("========================================")
print(" DBX Concurrent Workload Benchmark ")
print("========================================\n")

cert_dir = os.path.join(os.path.dirname(__file__), '..', 'certs')
client_ingest = DBXClient(port=6399, password=os.environ.get("DBX_DEFAULT_PASSWORD"), ca_cert=os.path.join(cert_dir, "ca.crt"), client_cert=os.path.join(cert_dir, "client.crt"), client_key=os.path.join(cert_dir, "client.key"))
client_search = DBXClient(port=6399, password=os.environ.get("DBX_DEFAULT_PASSWORD"), ca_cert=os.path.join(cert_dir, "ca.crt"), client_cert=os.path.join(cert_dir, "client.crt"), client_key=os.path.join(cert_dir, "client.key"))

np.random.seed(42)
vectors = np.random.randn(NUM_VECTORS, DIMENSION).astype(np.float32)
norms = np.linalg.norm(vectors, axis=1, keepdims=True)
vectors = vectors / norms

query_vectors = np.random.randn(1000, DIMENSION).astype(np.float32)
query_norms = np.linalg.norm(query_vectors, axis=1, keepdims=True)
query_vectors = query_vectors / query_norms

search_metrics = {"queries": 0, "total_time": 0.0, "errors": 0}
stop_search = False

def background_search():
    idx = 0
    while not stop_search:
        q = query_vectors[idx % 1000]
        idx += 1
        start = time.time()
        try:
            client_search.vsearch(INDEX_NAME, q.tolist(), top_k=5)
            search_metrics["queries"] += 1
            search_metrics["total_time"] += (time.time() - start)
        except Exception:
            search_metrics["errors"] += 1

print("🚀 Starting Background Search Thread...")
search_thread = threading.Thread(target=background_search)
search_thread.start()

print("🚀 Starting Bulk Ingestion...")
start_time = time.time()
for i in range(0, NUM_VECTORS, BATCH_SIZE):
    batch = vectors[i:i+BATCH_SIZE]
    ids = [f"cdoc_{i+j}" for j in range(len(batch))]
    client_ingest.vadd_batch(INDEX_NAME, DIMENSION, ids, batch.tolist())

insert_time = time.time() - start_time
stop_search = True
search_thread.join()

print(f"✅ Ingestion Complete: {NUM_VECTORS} vectors in {insert_time:.2f} seconds.")
print(f"   Ingestion Throughput: {NUM_VECTORS/insert_time:.2f} inserts/sec")
print(f"✅ Concurrent Search Results:")
print(f"   Total Queries Executed during Ingestion: {search_metrics['queries']}")
if search_metrics['queries'] > 0:
    avg_latency = (search_metrics["total_time"] / search_metrics["queries"]) * 1000
    qps = search_metrics['queries'] / insert_time
    print(f"   Avg Latency Under Load: {avg_latency:.2f} ms per query")
    print(f"   Search Throughput Under Load: {qps:.2f} QPS")
print(f"   Search Errors: {search_metrics['errors']}")
print("========================================")
