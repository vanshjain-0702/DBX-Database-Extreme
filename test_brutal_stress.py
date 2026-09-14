import concurrent.futures
import os
import sys
import time
import random
import uuid
import threading
import statistics

sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))
from dbx import ControlPlane, DBXClient

NUM_WORKERS = 50
OPS_PER_WORKER = 500
TENANTS = []

def random_string(length=1024):
    return os.urandom(length).hex()

def brutal_worker(worker_id, plane):
    tenant_id = f"brutal-{worker_id}-{uuid.uuid4().hex[:4]}"
    TENANTS.append(tenant_id)
    plane.provision(tenant_id, f"Brutal {worker_id}")
    key_info = plane.create_key(tenant_id, name="writer", role="writer")
    key_id = key_info.get("key", {}).get("id") or key_info.get("id")
    secret = key_info.get("key", {}).get("secret") or key_info.get("secret")
    
    client = DBXClient(host="127.0.0.1", port=6380, tenant=tenant_id, key_id=str(key_id), secret=str(secret))
    
    # Wait for ready
    for _ in range(30):
        try:
            client.ping()
            break
        except:
            time.sleep(0.5)

    stats = {
        "String": {"count": 0, "latencies": []},
        "Vector": {"count": 0, "latencies": []},
        "TimeTravel": {"count": 0, "latencies": []},
        "Migration": {"count": 0, "latencies": []},
        "errors": 0
    }
    
    try:
        for i in range(OPS_PER_WORKER):
            op = random.randint(0, 3)
            key = f"key-{random.randint(0, 1000)}"
            t0 = time.perf_counter()
            
            try:
                if op == 0:
                    # String: 500KB payload to stress memory
                    client.set(key, random_string(250 * 1024))
                    client.get(key)
                    stats["String"]["latencies"].append(time.perf_counter() - t0)
                    stats["String"]["count"] += 1
                
                elif op == 1:
                    # Vector: 128 dim
                    vec = [random.random() for _ in range(128)]
                    client.r.execute_command("VADD", f"idx-{key}", f"doc:{i}", *vec)
                    client.r.execute_command("VSEARCH", f"idx-{key}", *vec, "10")
                    stats["Vector"]["latencies"].append(time.perf_counter() - t0)
                    stats["Vector"]["count"] += 1

                elif op == 2:
                    # Time Travel
                    vec = [random.random() for _ in range(64)]
                    client.r.execute_command("VADD", f"tt-{key}", "doc:1", *vec)
                    time.sleep(0.01) # fast
                    ts = int(time.time() * 10**9)
                    time.sleep(0.01)
                    client.r.execute_command("VADD", f"tt-{key}", "doc:1", *([0.9]*64))
                    client.r.execute_command("VSEARCH", f"tt-{key}", *vec, "1", "AS_OF", str(ts))
                    stats["TimeTravel"]["latencies"].append(time.perf_counter() - t0)
                    stats["TimeTravel"]["count"] += 1

                elif op == 3:
                    # Migration
                    client.r.execute_command("VMIGRATE", "START", f"mig-{key}", "64", "sq8")
                    client.r.execute_command("VMIGRATE", "ADD", f"mig-{key}", "doc:1", *([0.1]*64))
                    client.r.execute_command("VMIGRATE", "SWAP", f"mig-{key}")
                    stats["Migration"]["latencies"].append(time.perf_counter() - t0)
                    stats["Migration"]["count"] += 1
                    
            except Exception as e:
                stats["errors"] += 1
                
    finally:
        # We don't shred here immediately to leave the memory footprint high
        pass
            
    return stats

def main():
    print("Starting brutal stress test...")
    plane = ControlPlane("http://127.0.0.1:8000")
    plane.login("admin", "adminadminadmin")
    
    t0 = time.perf_counter()
    results = []
    
    with concurrent.futures.ThreadPoolExecutor(max_workers=NUM_WORKERS) as pool:
        futures = [pool.submit(brutal_worker, i, plane) for i in range(NUM_WORKERS)]
        for f in concurrent.futures.as_completed(futures):
            results.append(f.result())
            
    elapsed = time.perf_counter() - t0
    
    # Cleanup tenants at the end
    print("Cleaning up tenants...")
    for t in TENANTS:
        try:
            plane.shred(t)
        except:
            pass

    # Aggregate stats
    agg = {
        "String": [],
        "Vector": [],
        "TimeTravel": [],
        "Migration": []
    }
    total_errors = 0
    total_ops = 0
    
    for r in results:
        total_errors += r["errors"]
        for cat in agg.keys():
            agg[cat].extend(r[cat]["latencies"])
            total_ops += r[cat]["count"]

    print("\n" + "="*50)
    print("           BRUTAL PERFORMANCE MATRIX")
    print("="*50)
    print(f"Total Wall Time:  {elapsed:.2f}s")
    print(f"Total Operations: {total_ops}")
    print(f"Total Errors:     {total_errors}")
    print(f"Overall QPS:      {(total_ops+total_errors)/elapsed:.2f} ops/sec\n")
    
    for cat, lats in agg.items():
        if len(lats) > 0:
            avg = statistics.mean(lats) * 1000
            p99 = statistics.quantiles(lats, n=100)[98] * 1000 if len(lats) > 100 else max(lats) * 1000
            print(f"--- {cat} ---")
            print(f"  Count: {len(lats)}")
            print(f"  Avg Latency: {avg:.2f} ms")
            print(f"  P99 Latency: {p99:.2f} ms")

if __name__ == "__main__":
    main()
