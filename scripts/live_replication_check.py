"""Live check of authenticated replication through a running orchestrator."""

import os
import socket
import sys
import time
import uuid

sys.path.append(
    os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "sdk", "python")
)
from dbx import ControlPlane, DBXClient  # noqa: E402

N = 300
NO_TCP_LISTENER = "no TCP listener (replication runs on a 0600 Unix socket)"


def wait_ping(client):
    for _ in range(60):
        try:
            client.ping()
            return
        except Exception:
            time.sleep(0.25)
    raise SystemExit("tenant never answered PING")


def sniff(port, send=b""):
    """Connect like a hostile local process and collect whatever is streamed."""
    try:
        s = socket.create_connection(("127.0.0.1", port), timeout=2)
    except ConnectionRefusedError:
        return None
    if send:
        s.sendall(send)
    s.settimeout(7)
    got = b""
    try:
        while True:
            chunk = s.recv(65536)
            if not chunk:
                break
            got += chunk
    except (socket.timeout, ConnectionResetError, ConnectionAbortedError):
        pass
    s.close()
    return got


plane = ControlPlane("http://127.0.0.1:8000")
plane.login("admin", "adminadminadmin")
tid = f"repl-live-{uuid.uuid4().hex[:4]}"
plane._json("POST", "/api/provision", {"id": tid, "name": "Repl Live", "replicas": 1})
info = plane.create_key(tid, name="writer", role="writer")
kid = info.get("key", {}).get("id") or info.get("id")
secret = info.get("key", {}).get("secret") or info.get("secret")

primary = DBXClient(
    host="127.0.0.1", port=6380, tenant=tid, key_id=str(kid), secret=str(secret)
)
wait_ping(primary)
t0 = time.perf_counter()
for i in range(N):
    primary.set(f"doc:{i}", f"privileged-contract-{i}")
print(f"primary: wrote {N} keys in {time.perf_counter() - t0:.2f}s")

views = plane._json("GET", "/api/tenants")
views = views.get("tenants", views) if isinstance(views, dict) else views
pview = next(v for v in views if v.get("id") == tid)
rport = pview.get("replication_port")
replica_id = (pview.get("replicas") or [f"{tid}-r1"])[0]
print(f"primary replication port={rport} replica={replica_id}")

ok = True
if rport:
    for label, payload in (
        ("silent connect", b""),
        ("RESP SYNC", b"*1\r\n$4\r\nSYNC\r\n"),
    ):
        leaked = sniff(rport, payload)
        if leaked is None:
            print(f"attacker [{label}]: {NO_TCP_LISTENER}")
            continue
        bad = b"privileged-contract" in leaked or len(leaked) > 0
        verdict = "LEAK" if bad else "nothing (blocked)"
        print(f"attacker [{label}]: received {len(leaked)} bytes -> {verdict}")
        ok &= not bad
else:
    print("no TCP replication port exposed (Unix socket mode)")

# Proxy credentials are scoped to one tenant id, so the replica needs its own key.
rinfo = plane.create_key(replica_id, name="reader", role="reader")
rkid = rinfo.get("key", {}).get("id") or rinfo.get("id")
rsecret = rinfo.get("key", {}).get("secret") or rinfo.get("secret")
replica = DBXClient(
    host="127.0.0.1",
    port=6380,
    tenant=replica_id,
    key_id=str(rkid),
    secret=str(rsecret),
)
wait_ping(replica)
deadline = time.time() + 20
last = None
while time.time() < deadline:
    last = replica.get(f"doc:{N - 1}")
    if last == f"privileged-contract-{N - 1}":
        break
    time.sleep(0.2)
caught_up = last == f"privileged-contract-{N - 1}"
sample_ok = all(
    replica.get(f"doc:{i}") == f"privileged-contract-{i}" for i in (0, N // 2, N - 1)
)
print(f"replica caught up: {caught_up}, spot-check: {sample_ok}")
ok &= caught_up and sample_ok

# Promotion keeps the public id: the replica's engine now serves `tid`, and
# `replica_id` restarts as a replica of it and must pass the handshake again.
plane._json("POST", "/api/tenants/promote", {"replica_id": replica_id})
promoted = DBXClient(
    host="127.0.0.1", port=6380, tenant=tid, key_id=str(kid), secret=str(secret)
)
wait_ping(promoted)
promoted.set("after-promote", "yes")
after = (
    promoted.get("after-promote") == "yes"
    and promoted.get("doc:0") == "privileged-contract-0"
)
print(f"promoted engine serves writes and old data under {tid}: {after}")
ok &= after

demoted = DBXClient(
    host="127.0.0.1",
    port=6380,
    tenant=replica_id,
    key_id=str(rkid),
    secret=str(rsecret),
)
wait_ping(demoted)
deadline = time.time() + 20
rejoined = False
while time.time() < deadline and not rejoined:
    try:
        rejoined = demoted.get("after-promote") == "yes"
    except Exception:
        pass
    if not rejoined:
        time.sleep(0.2)
print(f"demoted node re-authenticated and replicates the new primary: {rejoined}")
ok &= rejoined

for t in (replica_id, tid):
    try:
        plane.delete(t, purge=True)
    except Exception:
        pass

print("LIVE REPLICATION:", "PASSED" if ok else "FAILED")
sys.exit(0 if ok else 1)
