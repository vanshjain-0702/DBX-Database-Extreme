import sys
import os
import time
import threading
import subprocess
import socket
import numpy as np

sys.path.append(os.path.join(os.path.dirname(__file__), '..', 'sdk', 'python'))
from dbx import DBXClient

print("========================================")
print(" DBX Chaos Engineering & Jepsen Test ")
print("========================================\n")

INDEX_NAME = "chaos_index"
NUM_VECTORS = 10000
DIMENSION = 384
BATCH_SIZE = 1000

# 1. Start Fresh Server
print("💥 [Step 1] Wiping local data and starting DBX Server...")
os.system("taskkill /F /IM server.exe >nul 2>&1")
os.system("rmdir /S /Q data\\local >nul 2>&1")
time.sleep(1)

server_process = subprocess.Popen([r".\bin\server.exe", "-config", r".\configs\local.yaml"])
time.sleep(2) # wait for boot

cert_dir = os.path.join(os.path.dirname(__file__), '..', 'certs')
client = DBXClient(port=6399, ca_cert=os.path.join(cert_dir, "ca.crt"), client_cert=os.path.join(cert_dir, "client.crt"), client_key=os.path.join(cert_dir, "client.key"))

# Generate Vectors
np.random.seed(42)
vectors = np.random.randn(NUM_VECTORS, DIMENSION).astype(np.float32)
vectors = vectors / np.linalg.norm(vectors, axis=1, keepdims=True)

# 2. Malicious Thread: Connection Exhaustion & Garbage Bytes
def malicious_actor():
    print("😈 [Malicious Thread] Opening 100 garbage connections...")
    sockets = []
    try:
        for i in range(100):
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.connect(("127.0.0.1", 6399))
            s.sendall(b"GARBAGE BYTES\r\n")
            sockets.append(s)
        time.sleep(3)
        for s in sockets:
            s.close()
    except Exception as e:
        pass

malicious_thread = threading.Thread(target=malicious_actor)
malicious_thread.start()

# 3. Ingest Data
print(f"🚀 [Step 2] Starting Bulk Ingestion of {NUM_VECTORS} vectors...")
ingested_count = 0
try:
    for i in range(0, NUM_VECTORS, BATCH_SIZE):
        batch = vectors[i:i+BATCH_SIZE]
        ids = [f"chaos_{i+j}" for j in range(len(batch))]
        client.vadd_batch(INDEX_NAME, DIMENSION, ids, batch.tolist())
        ingested_count += BATCH_SIZE
        print(f"   Injected {ingested_count}...")
        
        # 4. Trigger the Crash mid-way
        if ingested_count == 5000:
            print("💥 [CRASH] KILLING DBX PROCESS MID-BATCH!")
            os.system("taskkill /F /IM server.exe >nul 2>&1")
            time.sleep(1) # simulate downtime
            break
except Exception as e:
    print(f"   Connection dropped (Expected): {e}")

# Wait for malicious thread
malicious_thread.join()

# 5. Recovery Phase
print("\n🔄 [Step 3] Rebooting DBX Server to test WAL Recovery...")
server_process = subprocess.Popen([r".\bin\server.exe", "-config", r".\configs\local.yaml"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)

# We must wait for the server to bind the port.
time.sleep(3) 

client = DBXClient(port=6399, ca_cert=os.path.join(cert_dir, "ca.crt"), client_cert=os.path.join(cert_dir, "client.crt"), client_key=os.path.join(cert_dir, "client.key"))
try:
    client.ping()
    print("✅ Server Rebooted successfully.")
except Exception as e:
    print("❌ Server failed to reboot!", e)
    sys.exit(1)

# 6. Verify Data Integrity
print("\n🔍 [Step 4] Verifying Data Integrity...")
try:
    # Do a quick search to ensure graph is intact
    q = vectors[0].tolist()
    res = client.vsearch(INDEX_NAME, q, top_k=1)
    if len(res) > 0:
        print(f"✅ Search successful. Graph is intact. Top result: {res[0][0]}")
    else:
        print("❌ Search returned empty! Data was lost.")
except Exception as e:
    print("❌ Search failed:", e)

print("🧹 Cleaning up...")
os.system("taskkill /F /IM server.exe >nul 2>&1")
print("========================================")
