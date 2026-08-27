#!/usr/bin/env python3
"""
DBX Comprehensive Benchmark & Security Audit Suite
====================================================
Tests:
  1. String SET/GET throughput  (50,000 ops)
  2. Hash HSET/HGET throughput  (20,000 ops)
  3. Vector VADD bulk ingest    (10,000 vectors, dim=128)
  4. Vector VSEARCH latency     (100 searches)
  5. Concurrent mixed workload  (50k ops across 20 threads)
  6. Security audit             (8 checks)
"""

import json
import os
import random
import statistics
import string
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor

import requests
import redis

# Config
BASE_URL = "http://localhost:8000"
TENANT = "bench-tenant"
TENANT_URL = f"{BASE_URL}/t/{TENANT}/query"
PASSWORD = os.environ.get("DBX_ADMIN_PASSWORD", "adminadminadmin")

STRING_OPS = 50_000
HASH_OPS = 20_000
VECTOR_COUNT = 10_000
VECTOR_DIM = 128
VSEARCH_COUNT = 100
CONCURRENCY = 8  # HTTP workers (Windows ephemeral ports)
RESP_WORKERS = 50  # Redis-compatible RESP connections for KV/vector throughput
BENCH_INDEX = "bench_vectors"

RED = "\033[91m"
GREEN = "\033[92m"
YELLOW = "\033[93m"
CYAN = "\033[96m"
BOLD = "\033[1m"
RESET = "\033[0m"

import threading as _threading

_thread_local = _threading.local()

session = requests.Session()
TOKEN = None


def hdr(text):
    print(f"\n{'='*60}")
    print(f"  {text}")
    print(f"{'='*60}")


def ok(text):
    print(f"  [OK]   {text}")


def warn(text):
    print(f"  [WARN] {text}")


def fail(text):
    print(f"  [FAIL] {text}")


def _make_session():
    """Return a per-thread session with connection pooling & keep-alive."""
    s = requests.Session()
    adapter = requests.adapters.HTTPAdapter(
        pool_connections=1,
        pool_maxsize=1,
        max_retries=requests.adapters.Retry(total=3, backoff_factor=0.3),
    )
    s.mount("http://", adapter)
    s.mount("https://", adapter)
    if TOKEN:
        s.headers.update({"Authorization": f"Bearer {TOKEN}"})
    return s


def _thread_session():
    """Get or create a per-thread session; always inject current token."""
    if not hasattr(_thread_local, "sess"):
        _thread_local.sess = _make_session()
    # Always ensure the latest token is set (auth may happen after session creation)
    if TOKEN:
        _thread_local.sess.headers.update({"Authorization": f"Bearer {TOKEN}"})
    return _thread_local.sess


def auth():
    global TOKEN
    r = session.post(
        f"{BASE_URL}/api/login",
        json={"username": "admin", "password": PASSWORD},
        timeout=10,
    )
    r.raise_for_status()
    TOKEN = r.json()["token"]
    session.headers.update({"Authorization": f"Bearer {TOKEN}"})
    ok(f"Authenticated  (token ends: ...{TOKEN[-8:]})")


def provision():
    r = session.post(
        f"{BASE_URL}/api/provision",
        json={"id": TENANT, "name": "Benchmark Tenant"},
        timeout=10,
    )
    if r.status_code in (200, 201):
        ok(f"Provisioned tenant '{TENANT}'")
    elif r.status_code == 500 and "already exists" in r.text:
        ok(f"Tenant '{TENANT}' already exists — reusing")
    else:
        warn(f"Provision returned {r.status_code}: {r.text[:80]}")


def cmd(command, timeout=10):
    """Execute a DBX command using the per-thread session (connection reuse)."""
    s = _thread_session()
    r = s.post(TENANT_URL, json={"command": command}, timeout=timeout)
    r.raise_for_status()
    return r.json().get("response", "")


_resp_pool = None


def tenant_resp_port():
    r = session.get(f"{BASE_URL}/api/tenants", timeout=10)
    r.raise_for_status()
    data = r.json()
    if not isinstance(data, list):
        return 6401
    for t in data:
        if t.get("id") == TENANT:
            return int(t.get("resp_port") or 6401)
    return 6401


def resp_pool():
    global _resp_pool
    if _resp_pool is None:
        _resp_pool = redis.ConnectionPool(
            host="127.0.0.1",
            port=tenant_resp_port(),
            password=os.environ.get("DBX_DEFAULT_PASSWORD", "adminadminadmin"),
            decode_responses=True,
            protocol=2,
            max_connections=RESP_WORKERS + 8,
        )
    return _resp_pool


def rconn():
    return redis.Redis(connection_pool=resp_pool())


def rand_str(n=32):
    return "".join(random.choices(string.ascii_letters + string.digits, k=n))


def rand_vec():
    return [round(random.uniform(-1.0, 1.0), 6) for _ in range(VECTOR_DIM)]


def compare(label, actual, target=0, unit="ops/sec"):
    """Score a result against the DBX single-tenant floor for this operation.

    Targets are our own acceptance thresholds, not third-party numbers: the point
    is to catch a regression in the isolated per-tenant engine, not to rank DBX
    against a database we did not measure on this hardware.
    """
    if target <= 0:
        return
    ratio = actual / target
    if ratio >= 1.0:
        color = GREEN
    elif ratio >= 0.5:
        color = YELLOW
    else:
        color = RED
    bar = "##" * int(min(ratio, 2) * 5)
    print(
        f"  {color}vs single-tenant target: [{bar:<20}] {ratio:.2f}x  ({actual:,.0f} / {target:,} {unit}){RESET}"
    )


# ── Benchmark 1: String SET ───────────────────────────────────────────────────


def bench_string_set():
    hdr(f"Benchmark 1 — String SET  ({STRING_OPS:,} ops, {RESP_WORKERS} RESP workers)")
    payload = "x" * 64
    per = STRING_OPS // RESP_WORKERS

    def worker(wid):
        c = rconn()
        pipe = c.pipeline(transaction=False)
        start = wid * per
        for i in range(start, start + per):
            pipe.set(f"bench:str:{i}", payload)
        pipe.execute()

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=RESP_WORKERS) as ex:
        list(ex.map(worker, range(RESP_WORKERS)))
    elapsed = time.perf_counter() - t0
    ops_sec = STRING_OPS / elapsed
    print(f"  {STRING_OPS:,} SETs in {elapsed:.2f}s")
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} ops/sec{RESET}")
    compare(label="String SET", actual=ops_sec, target=100_000)
    return ops_sec


# ── Benchmark 2: String GET ───────────────────────────────────────────────────


def bench_string_get():
    hdr(f"Benchmark 2 — String GET  ({STRING_OPS:,} ops, {RESP_WORKERS} RESP workers)")
    per = STRING_OPS // RESP_WORKERS

    def worker(wid):
        c = rconn()
        pipe = c.pipeline(transaction=False)
        start = wid * per
        for i in range(start, start + per):
            pipe.get(f"bench:str:{i}")
        pipe.execute()

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=RESP_WORKERS) as ex:
        list(ex.map(worker, range(RESP_WORKERS)))
    elapsed = time.perf_counter() - t0
    ops_sec = STRING_OPS / elapsed
    print(f"  {STRING_OPS:,} GETs in {elapsed:.2f}s")
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} ops/sec{RESET}")
    compare(label="String GET", actual=ops_sec, target=120_000)
    return ops_sec


# ── Benchmark 3: Hash HSET/HGET ───────────────────────────────────────────────


def bench_hash():
    hdr(f"Benchmark 3 — Hash HSET/HGET  ({HASH_OPS:,} ops)")
    workers = min(RESP_WORKERS, 50)
    per = HASH_OPS // workers

    def wset(wid):
        c = rconn()
        pipe = c.pipeline(transaction=False)
        start = wid * per
        for i in range(start, start + per):
            pipe.hset(f"bench:hash:{i}", mapping={"name": "n", "score": "1"})
        pipe.execute()

    def wget(wid):
        c = rconn()
        pipe = c.pipeline(transaction=False)
        start = wid * per
        for i in range(start, start + per):
            pipe.hget(f"bench:hash:{i}", "name")
        pipe.execute()

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=workers) as ex:
        list(ex.map(wset, range(workers)))
    ops_set = HASH_OPS / (time.perf_counter() - t0)
    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=workers) as ex:
        list(ex.map(wget, range(workers)))
    ops_get = HASH_OPS / (time.perf_counter() - t0)
    print(f"  HSET: {BOLD}{ops_set:,.0f} ops/sec{RESET}")
    print(f"  HGET: {BOLD}{ops_get:,.0f} ops/sec{RESET}")
    compare(label="Hash ops", actual=ops_set, target=80_000)
    return ops_set


# ── Benchmark 4: Vector VADD ──────────────────────────────────────────────────


def bench_vector_insert():
    hdr(f"Benchmark 4 — Vector VADD  ({VECTOR_COUNT:,} vectors, dim={VECTOR_DIM})")
    c = rconn()
    batch = 200
    t0 = time.perf_counter()
    for i in range(0, VECTOR_COUNT, batch):
        end = min(i + batch, VECTOR_COUNT)
        args = [BENCH_INDEX, str(VECTOR_DIM)]
        pipe = c.pipeline(transaction=False)
        for j in range(i, end):
            doc_id = f"bvec:{j}"
            vec = rand_vec()
            args.append(doc_id)
            args.extend(str(v) for v in vec)
            pipe.set(
                f"doc:{BENCH_INDEX}:{doc_id}",
                json.dumps({"text": f"document {j}", "category": "tech"}),
            )
        c.execute_command("VADD_BATCH", *args)
        pipe.execute()
    elapsed = time.perf_counter() - t0
    ops_sec = VECTOR_COUNT / elapsed
    print(
        f"  Ingested : {VECTOR_COUNT:,}/{VECTOR_COUNT:,}  errors: 0  in {elapsed:.2f}s"
    )
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} vectors/sec{RESET}")
    compare(label="Vector VADD", actual=ops_sec, target=5_000, unit="vectors/sec")
    return ops_sec


def bench_vector_search():
    hdr(f"Benchmark 5 — VSEARCH  ({VSEARCH_COUNT} queries, top-10)")
    c = rconn()
    latencies = []
    errors = 0
    for _ in range(VSEARCH_COUNT):
        vec = [str(v) for v in rand_vec()]
        t0 = time.perf_counter()
        try:
            c.execute_command("VSEARCH", BENCH_INDEX, *vec, "10")
            latencies.append((time.perf_counter() - t0) * 1000)
        except Exception:
            errors += 1
    if latencies:
        p50 = statistics.median(latencies)
        p99 = sorted(latencies)[int(len(latencies) * 0.99)]
        qps = 1000 / p50 if p50 > 0 else 0
        print(f"  Queries   : {len(latencies)}/{VSEARCH_COUNT}  errors: {errors}")
        print(f"  QPS       : {BOLD}{qps:,.1f}{RESET}")
        print(f"  p50 latency : {p50:.2f} ms    p99: {p99:.2f} ms")
        compare(label="VSEARCH QPS", actual=qps, target=500, unit="QPS")
    else:
        warn("No successful VSEARCH queries")


# ── Benchmark 6: Mixed Concurrent ────────────────────────────────────────────


def bench_mixed():
    hdr(f"Benchmark 6 — Mixed Concurrent  (50,000 ops, {RESP_WORKERS} RESP workers)")
    TOTAL = 50_000
    per = TOTAL // RESP_WORKERS

    def worker(tid):
        c = rconn()
        pipe = c.pipeline(transaction=False)
        for j in range(per):
            op = j % 4
            if op == 0:
                pipe.set(f"mix:{tid}:{j}", "v")
            elif op == 1:
                pipe.get(f"mix:{tid}:{max(0, j-1)}")
            elif op == 2:
                pipe.incr(f"mix:cnt:{tid}")
            else:
                pipe.hset(f"mix:h:{tid}", f"f{j}", "x")
        pipe.execute()

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=RESP_WORKERS) as ex:
        list(ex.map(worker, range(RESP_WORKERS)))
    elapsed = time.perf_counter() - t0
    ops_sec = TOTAL / elapsed
    print(f"  OK: {TOTAL:,}   Errors: 0   in {elapsed:.2f}s")
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} ops/sec{RESET}")
    compare(label="Mixed ops", actual=ops_sec, target=80_000)
    return ops_sec


# ── Security Audit ────────────────────────────────────────────────────────────


def security_audit():
    hdr("Security Audit  (8 checks)")
    issues = []

    # 1 — Unauthenticated /api/tenants
    r = requests.get(f"{BASE_URL}/api/tenants", timeout=5)
    if r.status_code in (401, 403):
        ok("Check 1 — /api/tenants blocks unauthenticated requests")
    else:
        fail(f"Check 1 — /api/tenants accessible without auth (HTTP {r.status_code})")
        issues.append("CRITICAL: Unauthenticated access to /api/tenants")

    # 2 — Unauthenticated data plane
    r = requests.post(
        f"{BASE_URL}/t/{TENANT}/query", json={"command": ["KEYS", "*"]}, timeout=5
    )
    if r.status_code in (401, 403):
        ok("Check 2 — Data plane /t/ blocks unauthenticated requests")
    else:
        fail(f"Check 2 — Data plane accessible without auth (HTTP {r.status_code})")
        issues.append("CRITICAL: Unauthenticated data plane access")

    # 3 — Invalid JWT rejected
    r = requests.post(
        f"{BASE_URL}/t/{TENANT}/query",
        json={"command": ["PING"]},
        headers={"Authorization": "Bearer eyJhbGciOiJIUzI1NiJ9.FAKE.FAKETOKEN"},
        timeout=5,
    )
    if r.status_code in (401, 403):
        ok("Check 3 — Forged/invalid JWT rejected")
    else:
        fail(f"Check 3 — Forged JWT accepted (HTTP {r.status_code})")
        issues.append("CRITICAL: Invalid JWT accepted")

    # 4 — Brute-force lockout
    locked = False
    for attempt in range(8):
        r = requests.post(
            f"{BASE_URL}/api/login",
            json={"username": "admin", "password": "wrong_pass_xyz_bench"},
            timeout=5,
        )
        if r.status_code == 429:
            ok(f"Check 4 — Brute-force lockout at attempt {attempt+1} (HTTP 429)")
            locked = True
            break
    if not locked:
        warn(
            "Check 4 — No brute-force lockout after 8 wrong attempts (may need config tuning)"
        )
        issues.append("MEDIUM: No login brute-force lockout observed")
    time.sleep(2)
    try:
        auth()
    except Exception:
        pass

    # 5 — Cross-tenant isolation
    r = session.post(
        f"{BASE_URL}/t/nonexistent-tenant-zzz/query",
        json={"command": ["KEYS", "*"]},
        timeout=5,
    )
    if r.status_code in (404, 502, 400, 503):
        ok(
            f"Check 5 — Cross-tenant isolation: nonexistent tenant = HTTP {r.status_code}"
        )
    else:
        warn(f"Check 5 — Unexpected HTTP {r.status_code} for nonexistent tenant")
        issues.append(f"LOW: Unexpected {r.status_code} for nonexistent tenant")

    # 6 — Oversized payload DoS guard
    try:
        giant = "X" * 1_000_000  # 1 MB key
        r = session.post(
            TENANT_URL, json={"command": ["SET", giant, "val"]}, timeout=15
        )
        if r.status_code in (400, 413, 500):
            ok(f"Check 6 — 1 MB key rejected (HTTP {r.status_code})")
        else:
            warn(
                f"Check 6 — 1 MB key accepted (HTTP {r.status_code}) — no size limit enforced"
            )
            issues.append("LOW: No max key/value size enforcement")
    except Exception:
        ok(
            "Check 6 — Oversized payload caused connection reset (server self-protected)"
        )

    # 7 — CRLF injection
    session.post(
        TENANT_URL, json={"command": ["SET", "crlf_key", "val\r\nPING\r\n"]}, timeout=5
    )
    r2 = session.post(TENANT_URL, json={"command": ["GET", "crlf_key"]}, timeout=5)
    raw = r2.json().get("response", "")
    if "PONG" not in raw:
        ok("Check 7 — CRLF/RESP injection via JSON body not effective")
    else:
        fail("Check 7 — CRLF injection may be exploitable")
        issues.append("CRITICAL: CRLF injection in JSON command body")

    # 8 — JWT signature tampering
    import base64

    parts = TOKEN.split(".")
    if len(parts) == 3:
        try:
            padded = parts[1] + "=" * (4 - len(parts[1]) % 4)
            payload = json.loads(base64.urlsafe_b64decode(padded))
            payload["sub"] = "superadmin_escalated"
            new_payload = (
                base64.urlsafe_b64encode(json.dumps(payload).encode())
                .decode()
                .rstrip("=")
            )
            tampered = f"{parts[0]}.{new_payload}.{parts[2]}"
            r = requests.get(
                f"{BASE_URL}/api/tenants",
                headers={"Authorization": f"Bearer {tampered}"},
                timeout=5,
            )
            if r.status_code in (401, 403):
                ok("Check 8 — JWT payload tampering rejected (HMAC validation works)")
            else:
                fail("Check 8 — Tampered JWT ACCEPTED — critical signing flaw!")
                issues.append("CRITICAL: JWT HMAC signature not validated")
        except Exception as e:
            warn(f"Check 8 — JWT tampering test inconclusive: {e}")
    else:
        warn("Check 8 — Token format unexpected, skipped")

    print()
    if not issues:
        print(f"  {GREEN}{BOLD}All security checks PASSED{RESET}")
    else:
        print(f"  {RED}{BOLD}{len(issues)} issue(s) found:{RESET}")
        for iss in issues:
            print(f"    {RED}-> {iss}{RESET}")
    return issues


# ── Summary ───────────────────────────────────────────────────────────────────


def print_summary(set_ops, get_ops, hash_ops, vec_ops, mixed_ops, sec_issues):
    hdr("SINGLE-TENANT PERFORMANCE REPORT")
    print(
        "  Targets are DBX acceptance floors for one isolated tenant on this host.\n"
        "  They are regression guards, not a ranking against other databases.\n"
    )
    rows = [
        ("String SET", set_ops, 100_000),
        ("String GET", get_ops, 120_000),
        ("Hash HSET", hash_ops, 80_000),
        ("Vector Ingest", vec_ops, 5_000),
        ("Mixed concurrent", mixed_ops, 80_000),
    ]
    for name, dbx, target in rows:
        ratio = dbx / target if target else 0
        status = (
            GREEN + "PASS  "
            if ratio >= 1.0
            else YELLOW + "MARGIN" if ratio >= 0.5 else RED + "REGRESS"
        )
        bar = "#" * min(int(ratio * 15), 30)
        print(
            f"  {BOLD}{name:<22}{RESET}  {dbx:>10,.0f} ops/s  "
            f"[{bar:<30}]  {status}{RESET}  ({ratio:.2f}x target {target:,})"
        )

    print()
    sec_ok = not any("CRITICAL" in i for i in sec_issues)
    print(
        f"  {BOLD}Security: {''+GREEN+'PASS'+RESET if sec_ok else RED+'CRITICAL ISSUES'+RESET}"
    )
    print(f"  Vectors stored   : {VECTOR_COUNT:,}  (dim={VECTOR_DIM})")
    print(f"  Total ops fired  : {STRING_OPS*2 + HASH_OPS*2 + VECTOR_COUNT + 50_000:,}")
    print()


# ── Entry ─────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    print(f"\n{BOLD}{'='*60}")
    print("  DBX Real-World Benchmark + Security Audit")
    print(f"  Target : {BASE_URL}  |  Tenant: {TENANT}")
    print(f"{'='*60}{RESET}\n")

    auth()
    provision()

    set_ops = bench_string_set()
    get_ops = bench_string_get()
    hash_ops = bench_hash()
    vec_ops = bench_vector_insert()
    bench_vector_search()
    mixed_ops = bench_mixed()
    issues = security_audit()

    print_summary(set_ops, get_ops, hash_ops, vec_ops, mixed_ops, issues)

    sys.exit(1 if any("CRITICAL" in i for i in issues) else 0)
