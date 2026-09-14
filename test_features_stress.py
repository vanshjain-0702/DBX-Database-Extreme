import concurrent.futures
import os
import sys
import time
import uuid

sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))
from dbx import ControlPlane, DBXClient

def run_migration_stress(plane, worker_id):
    tenant_id = f"stress-mig-{worker_id}-{uuid.uuid4().hex[:4]}"
    plane.provision(tenant_id, f"Stress Mig {worker_id}")
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

    index_name = "mig-index"
    success_count = 0
    try:
        for i in range(5):
            client.r.execute_command("VMIGRATE", "START", index_name, "64", "sq8")
            client.r.execute_command("VMIGRATE", "ADD", index_name, f"doc:{i}", *([0.1]*64))
            client.r.execute_command("VMIGRATE", "SWAP", index_name)
            success_count += 1
    except Exception as e:
        return {"worker_id": worker_id, "ok": False, "error": str(e), "type": "migration"}
    finally:
        try:
            plane.shred(tenant_id)
        except:
            pass
            
    return {"worker_id": worker_id, "ok": True, "success_count": success_count, "type": "migration"}

def run_time_travel_stress(plane, worker_id):
    tenant_id = f"stress-tt-{worker_id}-{uuid.uuid4().hex[:4]}"
    plane.provision(tenant_id, f"Stress TT {worker_id}")
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

    index_name = "tt-index"
    success_count = 0
    try:
        for i in range(5):
            client.r.execute_command("VADD", index_name, f"doc:{i}", *([0.1]*64))
            time.sleep(1.0)
            ts = int(time.time() * 10**9)
            time.sleep(1.0)
            client.r.execute_command("VADD", index_name, f"doc:{i}", *([0.9]*64))
            
            res_past = client.r.execute_command("VSEARCH", index_name, *([0.1]*64), "1", "AS_OF", str(ts))
            if res_past and len(res_past) > 0 and res_past[0][0] == f"doc:{i}":
                success_count += 1
            else:
                raise Exception(f"Time travel search failed. Result: {res_past}")
    except Exception as e:
        return {"worker_id": worker_id, "ok": False, "error": str(e), "type": "time_travel"}
    finally:
        try:
            plane.shred(tenant_id)
        except:
            pass
            
    return {"worker_id": worker_id, "ok": True, "success_count": success_count, "type": "time_travel"}

def main():
    print("Starting stress test for VMIGRATE and AS_OF features...")
    plane = ControlPlane("http://127.0.0.1:8000")
    plane.login("admin", "adminadminadmin")
    
    workers = 5
    results = []
    t0 = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=workers * 2) as pool:
        futures = []
        for i in range(workers):
            futures.append(pool.submit(run_migration_stress, plane, i))
            futures.append(pool.submit(run_time_travel_stress, plane, i))
            
        for f in concurrent.futures.as_completed(futures):
            results.append(f.result())
            
    elapsed = time.perf_counter() - t0
    
    mig_oks = sum(1 for r in results if r["type"] == "migration" and r["ok"])
    tt_oks = sum(1 for r in results if r["type"] == "time_travel" and r["ok"])
    
    print(f"Stress test completed in {elapsed:.2f}s")
    print(f"Migration tests passed: {mig_oks}/{workers}")
    print(f"Time travel tests passed: {tt_oks}/{workers}")
    
    for r in results:
        if not r["ok"]:
            print(f"Error in {r['type']} worker {r['worker_id']}: {r['error']}")

if __name__ == "__main__":
    main()
