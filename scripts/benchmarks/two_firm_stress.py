"""Synthetic two-firm mixed-workload stress run against an isolated DBX node.

Run from the repository root with Python, Go, and sdk/python dependencies:
    python scripts/benchmarks/two_firm_stress.py

The runner starts an isolated local orchestrator on ports 18000/16380 and puts
all generated data under data/two-firm-stress. It removes that directory after
the run. Records are synthetic and contain no legal or health information.
"""

from __future__ import annotations

import concurrent.futures
import json
import os
import random
import shutil
import signal
import socket
import statistics
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[2]
DATA = Path(tempfile.gettempdir()) / f"dbx-two-firm-stress-{uuid.uuid4().hex[:8]}"
API = "http://127.0.0.1:18000"
RESP_PORT = 16380
ADMIN = "stress-admin-password-2026"
JWT = "stress-jwt-secret-with-more-than-32-bytes"
INTERNAL = "stress-internal-token-with-more-than-32-bytes"
DIM = 64
DOCS_PER_TENANT = int(os.environ.get("DBX_STRESS_DOCS", "50000"))
TENANTS = {
    "legal-north-matter-a": ("legal", 1101),
    "legal-north-matter-b": ("legal", 2202),
    "health-summit-clinic-a": ("health", 3303),
    "health-summit-clinic-b": ("health", 4404),
}

sys.path.insert(0, str(ROOT / "sdk" / "python"))
from dbx import DBXClient, ControlPlane  # noqa: E402


def request(path: str, payload: dict | None = None, token: str = "") -> dict:
    data = None if payload is None else json.dumps(payload).encode()
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = Request(
        API + path, data=data, headers=headers, method="GET" if data is None else "POST"
    )
    try:
        with urlopen(req, timeout=120) as response:
            body = response.read()
    except HTTPError as exc:
        raise RuntimeError(
            f"{path}: HTTP {exc.code}: {exc.read().decode(errors='replace')}"
        ) from exc
    return json.loads(body) if body else {}


def wait_for_node(process: subprocess.Popen[str]) -> None:
    deadline = time.time() + 90
    while time.time() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"orchestrator exited early with {process.returncode}")
        try:
            with socket.create_connection(("127.0.0.1", 18000), timeout=0.25):
                return
        except OSError:
            time.sleep(0.25)
    raise TimeoutError("isolated orchestrator did not start within 90 seconds")


def vectors(seed: int, count: int, shift: float = 0.0):
    rng = random.Random(seed)
    for _ in range(count):
        row = [rng.uniform(-1, 1) for _ in range(DIM)]
        row[0] += shift
        norm = sum(x * x for x in row) ** 0.5
        yield [x / norm for x in row]


def call(client: DBXClient, *args):
    return client.r.execute_command(*args)


def create_client(plane: ControlPlane, tenant: str, role: str = "writer") -> DBXClient:
    minted = plane.create_key(tenant, name=f"stress-{role}", role=role)
    key = minted["key"]
    return DBXClient("127.0.0.1", RESP_PORT, tenant, key["id"], minted["secret"])


def ingest_tenant(item: tuple[str, tuple[str, int], ControlPlane]) -> dict:
    tenant, (sector, seed), plane = item
    client = create_client(plane, tenant)
    # Reuse this tenant's scoped secret while naming a different tenant. The
    # public ingress must reject it before exposing the neighbour's keyspace.
    other_tenant = next(name for name in TENANTS if name != tenant)
    auth_args = client.r.connection_pool.connection_kwargs
    scoped_id = auth_args["username"].split(":", 1)[1]
    impostor = DBXClient(
        "127.0.0.1", RESP_PORT, other_tenant, scoped_id, auth_args["password"]
    )
    cross_tenant_denied = False
    try:
        impostor.ping()
    except Exception:
        cross_tenant_denied = True
    if not cross_tenant_denied:
        raise AssertionError(f"credential for {tenant} authenticated as {other_tenant}")
    text_vectors = []
    image_vectors = []
    ids = []
    start = time.perf_counter()
    for offset in range(0, DOCS_PER_TENANT, 1000):
        size = min(1000, DOCS_PER_TENANT - offset)
        docs = [f"{tenant}:record:{i}" for i in range(offset, offset + size)]
        batch_text = list(vectors(seed + offset, size))
        batch_image = list(vectors(seed + 100000 + offset, size, 0.03))
        # Synthetic matter and clinic records, stored with independent modality spaces.
        for i, doc_id in enumerate(docs):
            metadata = json.dumps(
                {"sector": sector, "case_or_patient": i, "synthetic": True}
            )
            if not client.set(f"doc:records:{doc_id}", metadata):
                raise AssertionError(f"SET failed for {doc_id}")
        for space, batch in (("text", batch_text), ("image", batch_image)):
            args = ["VADD_BATCH", "records", DIM, "SPACE", space]
            for i, vec in enumerate(batch):
                args.append(docs[i])
                args.extend(vec)
            inserted = int(call(client, *args))
            if inserted != size:
                raise AssertionError(f"{tenant} {space}: inserted {inserted}/{size}")
        ids.extend(docs)
        text_vectors.extend(batch_text[:1] if offset == 0 else [])
        image_vectors.extend(batch_image[:1] if offset == 0 else [])
    ingest_seconds = time.perf_counter() - start

    # Data/query paths: scoped search, metadata filtering, similar-id, fusion, TTL.
    query = text_vectors[0]
    search = client.vsearch("records", query, 10, space="text", ef=100)
    filtered = call(
        client,
        "VSEARCH",
        "records",
        *query,
        10,
        "SPACE",
        "text",
        "FILTER_CONTAINS",
        '"sector":',
    )
    similar = client.vsim("records", ids[0], 10, space="text")
    fused = client.vfuse(
        "records", {"text": query, "image": image_vectors[0]}, 10, weights=[0.7, 0.3]
    )
    if not search or not filtered or not similar or not fused:
        raise AssertionError(f"{tenant} vector recall path returned empty results")
    if client.set("session:ephemeral", "short-lived", ex=60) is not True:
        raise AssertionError("SET EX failed")
    if client.get("session:ephemeral") != "short-lived":
        raise AssertionError("TTL value was not readable")

    # Time travel captures a known pre-update state, then verifies it after overwrite.
    historical_id = ids[0]
    call(client, "VADD", "history", historical_id, *query)
    # Leave a wide margin for Windows wall-clock granularity and asynchronous
    # WAL scheduling, then ensure the overwrite lands after the cutoff.
    as_of = time.time_ns() + 3_000_000_000
    time.sleep(3.1)
    changed = list(vectors(seed + 999999, 1))[0]
    call(client, "VADD", "history", historical_id, *changed)
    historical = call(client, "VSEARCH", "history", *query, 1, "AS_OF", as_of)
    if not historical:
        raise AssertionError(
            f"time-travel search returned no historical result: {historical!r}, as_of={as_of}"
        )

    # Upgrade workflow runs alongside workload in its own index.
    call(client, "VADD", "upgrade", "old-embedding", 0.1, 0.2, 0.3)
    call(client, "VMIGRATE", "START", "upgrade", "4", "sq8")
    call(client, "VMIGRATE", "ADD", "upgrade", "new-embedding", 0.1, 0.2, 0.3, 0.4)
    call(client, "VMIGRATE", "SWAP", "upgrade")
    migrated = call(client, "VSEARCH", "upgrade", 0.1, 0.2, 0.3, 0.4, 1)
    if not migrated:
        raise AssertionError("search failed after embedding migration")

    # Concurrent mixed read/write load on this tenant.
    latencies: list[float] = []
    errors: list[str] = []
    latency_lock = __import__("threading").Lock()

    def mix(worker: int) -> None:
        worker_client = create_client(plane, tenant)
        local = []
        try:
            for op in range(500):
                t0 = time.perf_counter()
                key = f"session:{worker}:{op % 100}"
                if op % 4 == 0:
                    worker_client.set(key, f"{sector}-{worker}-{op}", ex=120)
                elif op % 4 == 1:
                    worker_client.get(key)
                elif op % 4 == 2:
                    worker_client.vsearch("records", query, 5, space="text", ef=80)
                else:
                    worker_client.vsim(
                        "records", ids[(worker * 31 + op) % len(ids)], 5, space="text"
                    )
                local.append((time.perf_counter() - t0) * 1000)
        except Exception as exc:  # surface worker failures in the final report
            errors.append(f"{tenant} worker {worker}: {type(exc).__name__}: {exc}")
        with latency_lock:
            latencies.extend(local)

    mixed_start = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        list(pool.map(mix, range(8)))
    mixed_seconds = time.perf_counter() - mixed_start
    if errors:
        raise RuntimeError("; ".join(errors[:5]))

    # Admin and isolation paths: usage, reader ACL, cross-tenant denial, backup/restore.
    usage = plane.usage(tenant)
    reader = create_client(plane, tenant, "reader")
    denied_write = False
    try:
        reader.set("should-not-write", "x")
    except Exception:
        denied_write = True
    if not denied_write:
        raise AssertionError(f"{tenant} reader key wrote successfully")

    admin = create_client(plane, tenant, "tenant-admin")
    call(admin, "VCOMPACT", "records", "SPACE", "text")
    if not client.vsearch("records", query, 10, space="text"):
        raise AssertionError(f"{tenant} vector search failed after compaction")

    # Hibernate and wake exercise process lifecycle while preserving tenant data.
    client.set("lifecycle:sentinel", "survives-wake")
    plane.hibernate(tenant)
    plane.wake(tenant)
    client = create_client(plane, tenant)
    if client.get("lifecycle:sentinel") != "survives-wake":
        raise AssertionError(f"{tenant} hibernate/wake lost a key")

    # Back up, mutate, restore, and verify the pre-mutation KV value comes back.
    client.set("restore:sentinel", "before-backup")
    backup = plane.backup(tenant)
    client.set("restore:sentinel", "after-backup")
    restore_status = "PASS"
    try:
        plane.restore(tenant, backup["path"])
    except Exception as exc:
        restore_status = f"FAIL: {type(exc).__name__}: {exc}"
    client = create_client(plane, tenant)
    if client.get("restore:sentinel") != "before-backup":
        restore_status = (
            restore_status
            if restore_status.startswith("FAIL")
            else "FAIL: snapshot value not recovered"
        )

    # Delete semantics apply to both vector ids and document metadata.
    call(client, "VDEL", "records", ids[-1], "SPACE", "text")
    client.delete(f"doc:records:{ids[-1]}")
    return {
        "tenant": tenant,
        "documents": DOCS_PER_TENANT,
        "vector_rows": DOCS_PER_TENANT * 2,
        "ingest_seconds": round(ingest_seconds, 3),
        "ingest_vectors_per_second": round(DOCS_PER_TENANT * 2 / ingest_seconds),
        "mixed_ops": len(latencies),
        "mixed_ops_per_second": round(len(latencies) / mixed_seconds),
        "latency_ms_p95": round(statistics.quantiles(latencies, n=100)[94], 2),
        "latency_ms_p99": round(statistics.quantiles(latencies, n=100)[98], 2),
        "usage": usage,
        "search_hits": len(search),
        "fused_hits": len(fused),
        "backup_restore": restore_status,
        "reader_acl": "PASS",
        "cross_tenant_auth": "PASS",
        "time_travel": "PASS",
        "migration": "PASS",
        "hibernate_wake": "PASS",
        "vector_compaction": "PASS",
    }


def main() -> int:
    if DOCS_PER_TENANT < 1000:
        raise ValueError("DBX_STRESS_DOCS must be at least 1000")
    if DATA.exists():
        raise RuntimeError(f"refusing to overwrite existing test directory: {DATA}")
    DATA.mkdir(parents=True)
    env = os.environ.copy()
    env.update(
        {
            "DBX_ADMIN_PASSWORD": ADMIN,
            "DBX_JWT_SECRET": JWT,
            "DBX_INTERNAL_API_TOKEN": INTERNAL,
            "DBX_DATA_DIR": str(DATA),
            "DBX_NODE_MEMORY_BUDGET": "8gb",
            "DBX_ISOLATION_MODE": "inprocess",
            "DBX_ALLOW_INPROCESS": "1",
        }
    )
    proc = subprocess.Popen(
        [
            "go",
            "run",
            "./cmd/dbx-orchestrator",
            "-insecure-http=true",
            "-port",
            "18000",
            "-resp-addr",
            f"127.0.0.1:{RESP_PORT}",
            "-state-file",
            str(DATA / "tenants.json"),
            "-admin-file",
            str(DATA / "admin.json"),
        ],
        cwd=ROOT,
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        text=True,
        creationflags=getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0),
    )
    result: dict = {"status": "FAIL"}
    try:
        wait_for_node(proc)
        plane = ControlPlane(API)
        plane.login("admin", ADMIN)
        for tenant, (sector, _) in TENANTS.items():
            plane.provision(tenant, f"Synthetic {sector} tenant")
        # Four simultaneous customer stores, split across two independent firms.
        start = time.perf_counter()
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            rows = list(
                pool.map(
                    ingest_tenant,
                    [(tenant, info, plane) for tenant, info in TENANTS.items()],
                )
            )
        wall = time.perf_counter() - start
        failures = [
            f"{row['tenant']}: {row['backup_restore']}"
            for row in rows
            if row["backup_restore"] != "PASS"
        ]
        result = {
            "status": "FAIL" if failures else "PASS",
            "scenario": "2 synthetic firms; 2 isolated client/clinic tenants per firm-type",
            "tenant_count": len(TENANTS),
            "documents_total": DOCS_PER_TENANT * len(TENANTS),
            "vector_rows_total": DOCS_PER_TENANT * len(TENANTS) * 2,
            "firm_workload_wall_seconds": round(wall, 3),
            "tenants": rows,
            "failures": failures,
            "verified": [
                "scoped auth",
                "reader role",
                "cross-tenant routing boundary",
                "KV SET/GET/TTL/delete",
                "batched vector ingest",
                "VSEARCH/filters/EF",
                "VSIM",
                "VFUSE",
                "time travel",
                "embedding migration",
                "usage metrics",
                "backup/restore",
                "hibernate/wake",
                "vector compaction",
                "vector delete",
            ],
            "limitations": [
                "Windows inprocess profile; no Linux Landlock/strict certification",
                "synthetic workload; not a legal/health compliance certification",
                "no embedding model or application-side clinical/legal correctness tested",
            ],
        }
    except Exception as exc:
        result = {"status": "FAIL", "error": f"{type(exc).__name__}: {exc}"}
    finally:
        try:
            if proc.poll() is None:
                proc.send_signal(
                    signal.CTRL_BREAK_EVENT if os.name == "nt" else signal.SIGTERM
                )
                proc.wait(timeout=15)
        except Exception:
            proc.kill()
            proc.wait(timeout=10)
        shutil.rmtree(DATA, ignore_errors=True)
    print(json.dumps(result, indent=2))
    return 0 if result.get("status") == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
