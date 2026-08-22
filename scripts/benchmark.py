import sys
import os
import time
import uuid
import numpy as np

# Add SDK path
sys.path.append(os.path.join(os.path.dirname(__file__), '..', 'sdk', 'python'))
from dbx import DBXClient

print("========================================")
print(" DBX Vector Search Benchmark Suite ")
print("========================================\n")

cert_dir = os.path.join(os.path.dirname(__file__), '..', 'certs')
client = DBXClient(port=6399, 
                   password=os.environ.get("DBX_DEFAULT_PASSWORD"),
                   ca_cert=os.path.join(cert_dir, "ca.crt"), 
                   client_cert=os.path.join(cert_dir, "client.crt"), 
                   client_key=os.path.join(cert_dir, "client.key"))
try:
    client.ping()
    print("✅ Connected to DBX Orchestrator")
except Exception as e:
    print("❌ Failed to connect to DBX:", e)
    sys.exit(1)

INDEX_NAME = os.environ.get("DBX_BENCHMARK_INDEX", "benchmark_index")
NUM_VECTORS = int(os.environ.get("DBX_BENCHMARK_VECTORS", "50000"))
DIMENSION = int(os.environ.get("DBX_BENCHMARK_DIMENSION", "384"))
K = int(os.environ.get("DBX_BENCHMARK_K", "5"))

print(f"\nGeneraing {NUM_VECTORS} random vectors (Dimension: {DIMENSION})...")
np.random.seed(42)
# Generate random vectors
vectors = np.random.randn(NUM_VECTORS, DIMENSION).astype(np.float32)

# Normalize vectors (for cosine similarity)
norms = np.linalg.norm(vectors, axis=1, keepdims=True)
vectors = vectors / norms

print("\n🚀 Injecting vectors into DBX...")
start_time = time.time()

# Batch insertion
batch_size = 1000
for i in range(0, NUM_VECTORS, batch_size):
    batch = vectors[i:i+batch_size]
    ids = [f"doc_{i+j}" for j in range(len(batch))]
    vecs = batch.tolist()
    
    client.vadd_batch(INDEX_NAME, DIMENSION, ids, vecs)
    
    if (i + batch_size) % 2000 == 0:
        print(f"  Inserted {i + batch_size}/{NUM_VECTORS}...")

insert_time = time.time() - start_time
print(f"✅ Ingestion Complete: {NUM_VECTORS} vectors in {insert_time:.2f} seconds.")
print(f"   Throughput: {NUM_VECTORS/insert_time:.2f} inserts/sec")

print("\n🔍 Running Graph Search (NSW) Benchmark...")
# Generate 100 query vectors
NUM_QUERIES = int(os.environ.get("DBX_BENCHMARK_QUERIES", "100"))
query_vectors = np.random.randn(NUM_QUERIES, DIMENSION).astype(np.float32)
query_norms = np.linalg.norm(query_vectors, axis=1, keepdims=True)
query_vectors = query_vectors / query_norms

search_start = time.time()
for q in query_vectors:
    results = client.vsearch(INDEX_NAME, q.tolist(), top_k=K)
search_time = time.time() - search_start

avg_latency = (search_time / NUM_QUERIES) * 1000
qps = NUM_QUERIES / search_time

print(f"✅ Search Benchmark Complete: {NUM_QUERIES} queries executed.")
print(f"   Total Time: {search_time:.4f} seconds")
print(f"   Avg Latency: {avg_latency:.2f} ms per query")
print(f"   Throughput (QPS): {qps:.2f} queries/sec")

print("\n========================================")
print(" Benchmark Summary")
print("========================================")
print(f"Index Size:    {NUM_VECTORS} vectors")
print(f"Dimensions:    {DIMENSION}")
print(f"Graph Search:  {avg_latency:.2f} ms")
print(f"QPS:           {qps:.2f}")
print("========================================")
