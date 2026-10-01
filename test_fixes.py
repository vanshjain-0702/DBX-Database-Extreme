import urllib.request
import urllib.error
import json
import time

TOKEN = "internalapitoken1234567890123456"
TENANT = "test-tenant-fix-" + str(int(time.time()))

def main():
    print("Logging in...")
    login_req = urllib.request.Request(
        "http://127.0.0.1:8000/api/login",
        data=json.dumps({"username": "admin", "password": "adminadminadmin"}).encode('utf-8'),
        headers={"Content-Type": "application/json"}
    )
    login_resp = urllib.request.urlopen(login_req)
    jwt_token = json.loads(login_resp.read())["token"]
    print("Logged in successfully.")

    print(f"Provisioning {TENANT}...")
    req = urllib.request.Request(
        "http://127.0.0.1:8000/api/provision",
        data=json.dumps({"id": TENANT, "name": "Test Tenant Fixes", "replicas": 0, "vector_encoding": "fp32"}).encode('utf-8'),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {jwt_token}"}
    )
    
    t0 = time.time()
    resp = urllib.request.urlopen(req)
    t1 = time.time()
    
    res = json.loads(resp.read())
    print(f"Provisioned in {t1 - t0:.3f}s: {res}")
    
    print("\nTesting Query with String Command...")
    query_req = urllib.request.Request(
        f"http://127.0.0.1:8000/t/{TENANT}/query",
        data=json.dumps({"command": "VADD test1 \"[1.0, 2.0, 3.0]\" {\"key\":\"value\"}"}).encode('utf-8'),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {jwt_token}"}
    )
    
    try:
        query_resp = urllib.request.urlopen(query_req)
        print(f"Query Result (String format): {query_resp.read().decode('utf-8')}")
    except urllib.error.HTTPError as e:
        print(f"Query Error: {e.code} - {e.read().decode('utf-8')}")
        return

    print("\nTesting Query with Array Command (Backward Compatibility)...")
    query_req2 = urllib.request.Request(
        f"http://127.0.0.1:8000/t/{TENANT}/query",
        data=json.dumps({"command": ["VSEARCH", "test1", "0.0", "10", "[1.0, 2.0, 3.0]"]}).encode('utf-8'),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {jwt_token}"}
    )
    
    try:
        query_resp2 = urllib.request.urlopen(query_req2)
        print(f"Query Result (Array format): {query_resp2.read().decode('utf-8')}")
    except urllib.error.HTTPError as e:
        print(f"Query Error: {e.code} - {e.read().decode('utf-8')}")

if __name__ == "__main__":
    main()
