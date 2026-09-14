import concurrent.futures
import os
import sys
import time
import random
import uuid

sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))
from dbx import ControlPlane, DBXClient

NUM_WORKERS = 20
OPS_PER_WORKER = 500

def random_string(length=1000):
    return os.urandom(length).hex()

def stress_worker(worker_id, plane):
    tenant_id = f"stress-all-{worker_id}-{uuid.uuid4().hex[:4]}"
    plane.provision(tenant_id, f"Stress All {worker_id}")
    key_info = plane.create_key(tenant_id, name="writer", role="writer")
    key_id = key_info.get("key", {}).get("id") or key_info.get("id")
    secret = key_info.get("key", {}).get("secret") or key_info.get("secret")
    
    client = DBXClient(host="127.0.0.1", port=6380, tenant=tenant_id, key_id=str(key_id), secret=str(secret))
    
    for _ in range(30):
        try:
            client.ping()
            break
        except:
            time.sleep(0.5)

    success = 0
    errors = 0
    
    try:
        for i in range(OPS_PER_WORKER):
            op = random.randint(0, 7)
            key = f"key-{random.randint(0, 100)}"
            
            try:
                if op == 0: # String
                    client.set(key, random_string(1024))
                    client.get(key)
                elif op == 1: # Hash
                    client.r.execute_command("HSET", key, "f1", random_string(500), "f2", random_string(500))
                    client.r.execute_command("HGETALL", key)
                elif op == 2: # List
                    client.r.execute_command("LPUSH", key, random_string(100))
                    client.r.execute_command("LRANGE", key, "0", "-1")
                elif op == 3: # Set
                    client.r.execute_command("SADD", key, random_string(100))
                    client.r.execute_command("SMEMBERS", key)
                elif op == 4: # ZSet
                    client.r.execute_command("ZADD", key, str(random.random()), random_string(50))
                    client.r.execute_command("ZRANGE", key, "0", "-1")
                elif op == 5: # Vector
                    vec = [random.random() for _ in range(64)]
                    client.r.execute_command("VADD", key, f"doc:{i}", *vec)
                    client.r.execute_command("VSEARCH", key, *vec, "5")
                elif op == 6: # Delete
                    client.delete(key)
                elif op == 7: # Large payload
                    client.set(f"large-{i}", random_string(1024 * 100)) # 200KB payload
                    
                success += 1
            except Exception as e:
                if errors < 5:
                    print(f"Worker {worker_id} op {op} error: {e}")
                errors += 1
                
    finally:
        try:
            plane.shred(tenant_id)
        except:
            pass
            
    return {"worker_id": worker_id, "success": success, "errors": errors}

def main():
    print("Starting all-features structure-breaking stress test...")
    plane = ControlPlane("http://127.0.0.1:8000")
    plane.login("admin", "adminadminadmin")
    
    t0 = time.perf_counter()
    results = []
    
    with concurrent.futures.ThreadPoolExecutor(max_workers=NUM_WORKERS) as pool:
        futures = [pool.submit(stress_worker, i, plane) for i in range(NUM_WORKERS)]
        for f in concurrent.futures.as_completed(futures):
            results.append(f.result())
            
    elapsed = time.perf_counter() - t0
    
    total_success = sum(r["success"] for r in results)
    total_errors = sum(r["errors"] for r in results)
    
    print(f"\nStress Test Completed in {elapsed:.2f}s")
    print(f"Total Successful Ops: {total_success}")
    print(f"Total Errored Ops: {total_errors}")
    print(f"Throughput: {(total_success+total_errors)/elapsed:.2f} ops/sec")
    
    if total_errors > 0:
        print("\nWARNING: Some operations failed. The structure broke under stress!")
    else:
        print("\nSUCCESS: The structure held up perfectly under stress!")

if __name__ == "__main__":
    main()
