import os
import sys
import time
import uuid

sys.path.append(os.path.join(os.path.dirname(__file__), 'sdk', 'python'))
from dbx import ControlPlane, DBXClient

def main():
    print("Starting DBX VFUSE Test...")
    plane = ControlPlane("http://127.0.0.1:8000")
    plane.login("admin", "adminadminadmin")
    
    tenant_id = f"vfuse-test-{uuid.uuid4().hex[:4]}"
    plane.provision(tenant_id, "VFUSE Test")
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

    index_name = "multimodal_idx"
    
    print("Inserting data into 'text' space...")
    vec_text_1 = [0.1] * 64
    vec_text_2 = [0.9] * 64
    client.r.execute_command("VADD", index_name, "doc:1", "SPACE", "text", *vec_text_1)
    client.r.execute_command("VADD", index_name, "doc:2", "SPACE", "text", *vec_text_2)

    print("Inserting data into 'image' space...")
    vec_img_1 = [0.9] * 64
    vec_img_2 = [0.1] * 64
    client.r.execute_command("VADD", index_name, "doc:1", "SPACE", "image", *vec_img_1)
    client.r.execute_command("VADD", index_name, "doc:2", "SPACE", "image", *vec_img_2)
    
    q_text = [0.1] * 64
    q_img = [0.9] * 64
    
    print("Running VFUSE query (50% text, 50% image)...")
    res = client.vfuse(
        index_name,
        queries={"text": q_text, "image": q_img},
        top_k=2,
        weights=[0.5, 0.5]
    )
    
    print(f"Results: {res}")
    
    if len(res) == 2 and res[0][0] == "doc:1":
        print("\nSUCCESS: VFUSE multimodal search returned correct fused ranking.")
    else:
        print("\nFAIL: VFUSE did not return expected results.")
        
    try:
        plane.shred(tenant_id)
    except:
        pass

if __name__ == "__main__":
    main()
