import multiprocessing
import os
import random
import time
import uuid

# append sdk path
import sys
sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))

from dbx import DBXClient, ControlPlane

def load_test_client(client_id, num_strings=250000, num_vectors=25000, dim=128):
    tenant_id = f"test-tenant-{client_id}-{uuid.uuid4().hex[:8]}"
    print(f"[{client_id}] Starting load test for tenant {tenant_id}")
    
    # 1. Provision tenant
    cp = ControlPlane(base="http://127.0.0.1:8000")
    cp.login("admin", "adminadminadmin")
    cp.provision(tenant_id, f"Test Tenant {client_id}")
    
    # 2. Mint key
    key_info = cp.create_key(tenant_id, name="writer", role="writer")
    
    key_id = key_info.get("key", {}).get("id") or key_info.get("id")
    secret = key_info.get("key", {}).get("secret") or key_info.get("secret")
    
    print(f"[{client_id}] Minted key for {tenant_id}")
    
    # Connect DBXClient
    client = DBXClient(
        host="127.0.0.1",
        port=6380,
        tenant=tenant_id,
        key_id=str(key_id),
        secret=str(secret)
    )
    
    # Wait for tenant to be ready
    max_retries = 20
    for attempt in range(max_retries):
        try:
            client.get("test_ready")
            break
        except Exception as e:
            if "tenant unavailable" in str(e) or "Connection" in str(e):
                time.sleep(1)
            else:
                raise e
    
    # 4. Insert Hash Strings
    print(f"[{client_id}] Inserting {num_strings} strings...")
    start_time = time.time()
    # To speed up, we can use pipeline if it's supported, but the dbx.py client doesn't expose pipeline.
    # We will just do sets. Wait, 250k sets might take a few minutes.
    # Let's do it in smaller batches or just sequential.
    for i in range(num_strings):
        client.set(f"key:{i}", f"value_for_{i}_from_client_{client_id}")
        if i > 0 and i % 50000 == 0:
            print(f"[{client_id}] Inserted {i} strings in {time.time() - start_time:.2f}s")
            
    print(f"[{client_id}] Finished string insertion. Took {time.time() - start_time:.2f}s")
    
    # 5. Insert Vectors
    print(f"[{client_id}] Inserting {num_vectors} vectors...")
    start_time = time.time()
    batch_size = 1000
    for i in range(0, num_vectors, batch_size):
        end = min(i + batch_size, num_vectors)
        doc_ids = [f"vec:{j}" for j in range(i, end)]
        vectors = [[random.random() for _ in range(dim)] for _ in range(i, end)]
        client.vadd_batch("memories", dim, doc_ids, vectors)
        if i > 0 and i % 5000 == 0:
            print(f"[{client_id}] Inserted {i} vectors in {time.time() - start_time:.2f}s")
            
    print(f"[{client_id}] Finished vector insertion. Took {time.time() - start_time:.2f}s")
    
    # 6. Basic Tests
    print(f"[{client_id}] Running basic tests...")
    
    # Test GET
    val = client.get(f"key:{num_strings//2}")
    assert val == f"value_for_{num_strings//2}_from_client_{client_id}", f"GET failed, got {val}"
    
    # Test VSEARCH
    query_vec = [random.random() for _ in range(dim)]
    results = client.vsearch("memories", query_vec, top_k=5)
    assert len(results) == 5, f"VSEARCH failed, got {len(results)} results"
    
    print(f"[{client_id}] Tests passed successfully.")
    
    # We don't delete to leave it in the database for soak verification if needed
    
if __name__ == '__main__':
    num_clients = 5 # Set to 5 clients for huge load test
    processes = []
    
    for i in range(num_clients):
        p = multiprocessing.Process(target=load_test_client, args=(i, 100000, 50000, 256))
        processes.append(p)
        p.start()
        
    for p in processes:
        p.join()
        
    print("All clients finished load testing.")
