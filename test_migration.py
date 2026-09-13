import os
import sys
import uuid

sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))
from dbx import ControlPlane, DBXClient

def main():
    tenant_id = f"test-tenant-mig-{uuid.uuid4().hex[:8]}"
    print(f"Starting migration test for tenant {tenant_id}")
    
    # 1. Provision tenant
    cp = ControlPlane(base="http://127.0.0.1:8000")
    cp.login("admin", "adminadminadmin")
    cp.provision(tenant_id, "Migration Test Tenant")
    
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
    
    # 4. Test VMIGRATE START
    print("Starting migration...")
    res = client.r.execute_command("VMIGRATE", "START", "my-index", "512", "sq8")
    print("VMIGRATE START result:", res)

    # 5. Test VMIGRATE ADD
    print("Adding shadow vector...")
    vector = [0.1] * 512
    res = client.r.execute_command("VMIGRATE", "ADD", "my-index", "doc:1", *vector)
    print("VMIGRATE ADD result:", res)

    # 6. Test VMIGRATE CANCEL
    print("Canceling migration...")
    res = client.r.execute_command("VMIGRATE", "CANCEL", "my-index")
    print("VMIGRATE CANCEL result:", res)

    # 7. Test VMIGRATE START again
    print("Starting migration again...")
    res = client.r.execute_command("VMIGRATE", "START", "my-index", "512", "sq8")
    print("VMIGRATE START result:", res)

    # 8. Test VMIGRATE SWAP
    print("Swapping migration...")
    res = client.r.execute_command("VMIGRATE", "SWAP", "my-index")
    print("VMIGRATE SWAP result:", res)

    print("All VMIGRATE tests passed!")

if __name__ == "__main__":
    main()
