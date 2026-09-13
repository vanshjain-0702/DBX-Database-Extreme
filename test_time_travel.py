import os
import sys
import uuid
import time
import math

sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))
from dbx import ControlPlane, DBXClient

def main():
    tenant_id = f"test-tenant-tt-{uuid.uuid4().hex[:8]}"
    print(f"Starting time-travel test for tenant {tenant_id}")
    
    # 1. Provision tenant
    cp = ControlPlane(base="http://127.0.0.1:8000")
    cp.login("admin", "adminadminadmin")
    cp.provision(tenant_id, "Time-Travel Test Tenant")
    
    # 2. Mint key
    key_info = cp.create_key(tenant_id, name="writer", role="writer")
    key_id = key_info.get("key", {}).get("id") or key_info.get("id")
    secret = key_info.get("key", {}).get("secret") or key_info.get("secret")
    print(f"Minted! Token: {secret[:10]}...")

    # 3. Connect via DBXClient
    client = DBXClient(
        host="127.0.0.1",
        port=6380,
        tenant=tenant_id,
        key_id=str(key_id),
        secret=str(secret)
    )
    
    print("Wait for tenant to be ready...")
    for _ in range(10):
        try:
            client.get("foo")
            break
        except Exception:
            time.sleep(1)

    index_name = "tt-index"
    
    # Insert version 1
    print("Inserting doc:1 with vector [0.1, 0.1]")
    client.r.execute_command("VADD", index_name, "doc:1", 0.1, 0.1)

    # Get timestamp A in nanoseconds
    time.sleep(1.5)
    timestamp_a = int(time.time() * 10**9)
    print(f"Recorded Timestamp A: {timestamp_a}")
    time.sleep(1.5)

    # Insert version 2 (overwrite doc:1)
    print("Overwriting doc:1 with vector [0.9, 0.9]")
    client.r.execute_command("VADD", index_name, "doc:1", 0.9, 0.9)

    time.sleep(0.5)

    # Search current (should match version 2)
    print("\n--- Searching Current State ---")
    res_current = client.r.execute_command("VSEARCH", index_name, 0.9, 0.9, 1, "WITHDOCS", "1")
    print("Current state results:", res_current)
    assert res_current[0][0] == "doc:1", "Expected doc:1 in current search"
    assert float(res_current[0][1]) > 0.99, "Expected high score for [0.9, 0.9]"
    
    # Search AS_OF Timestamp A
    print("\n--- Searching AS_OF Timestamp A ---")
    res_past = client.r.execute_command("VSEARCH", index_name, 0.1, 0.1, 1, "WITHDOCS", "1", "AS_OF", str(timestamp_a))
    print("AS_OF results:", res_past)
    assert res_past[0][0] == "doc:1", "Expected doc:1 in AS_OF search"
    assert float(res_past[0][1]) > 0.99, "Expected high score for [0.1, 0.1]"

    print("\nAll Time-Travel tests passed successfully!")

if __name__ == "__main__":
    main()
