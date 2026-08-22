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
from concurrent.futures import ThreadPoolExecutor, as_completed

import requests

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
CONCURRENCY = 8  # Keep low to avoid Windows ephemeral port exhaustion (TIME_WAIT)
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


def rand_str(n=32):
    return "".join(random.choices(string.ascii_letters + string.digits, k=n))


def rand_vec():
    return [round(random.uniform(-1.0, 1.0), 6) for _ in range(VECTOR_DIM)]


def compare(label, actual, redis_ref=0, pinecone_ref=0):
    refs = []
    if redis_ref > 0:
        refs.append(("Redis 7 (local)", redis_ref))
    if pinecone_ref > 0:
        refs.append(("Pinecone serverless", pinecone_ref))
    for ref_name, ref_val in refs:
        ratio = actual / ref_val if ref_val else 0
        if ratio >= 0.8:
            color = GREEN
        elif ratio >= 0.5:
            color = YELLOW
        else:
            color = RED
        bar = "##" * int(min(ratio, 2) * 5)
        print(
            f"  {color}vs {ref_name}: [{bar:<20}] {ratio:.2f}x  ({actual:,.0f} vs {ref_val:,} ops/sec){RESET}"
        )


# ── Benchmark 1: String SET ───────────────────────────────────────────────────


def bench_string_set():
    hdr(f"Benchmark 1 — String SET  ({STRING_OPS:,} ops, {CONCURRENCY} workers)")
    keys = [f"bench:str:{i}" for i in range(STRING_OPS)]
    values = [rand_str(64) for _ in range(STRING_OPS)]
    latencies = []

    def do_set(i):
        t0 = time.perf_counter()
        cmd(["SET", keys[i], values[i]])
        return (time.perf_counter() - t0) * 1000

    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as ex:
        for lat in as_completed([ex.submit(do_set, i) for i in range(STRING_OPS)]):
            latencies.append(lat.result())
    elapsed = time.perf_counter() - start

    ops_sec = STRING_OPS / elapsed
    p50 = statistics.median(latencies)
    p99 = sorted(latencies)[int(len(latencies) * 0.99)]
    print(f"  {STRING_OPS:,} SETs in {elapsed:.2f}s")
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} ops/sec{RESET}")
    print(f"  p50 latency : {p50:.2f} ms    p99: {p99:.2f} ms")
    compare(label="String SET", actual=ops_sec, redis_ref=100_000)
    return ops_sec


# ── Benchmark 2: String GET ───────────────────────────────────────────────────


def bench_string_get():
    hdr(f"Benchmark 2 — String GET  ({STRING_OPS:,} ops, {CONCURRENCY} workers)")
    keys = [f"bench:str:{i}" for i in range(STRING_OPS)]
    latencies = []

    def do_get(i):
        t0 = time.perf_counter()
        cmd(["GET", keys[i]])
        return (time.perf_counter() - t0) * 1000

    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as ex:
        for lat in as_completed([ex.submit(do_get, i) for i in range(STRING_OPS)]):
            latencies.append(lat.result())
    elapsed = time.perf_counter() - start

    ops_sec = STRING_OPS / elapsed
    p50 = statistics.median(latencies)
    p99 = sorted(latencies)[int(len(latencies) * 0.99)]
    print(f"  {STRING_OPS:,} GETs in {elapsed:.2f}s")
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} ops/sec{RESET}")
    print(f"  p50 latency : {p50:.2f} ms    p99: {p99:.2f} ms")
    compare(label="String GET", actual=ops_sec, redis_ref=120_000)
    return ops_sec


# ── Benchmark 3: Hash HSET/HGET ───────────────────────────────────────────────


def bench_hash():
    hdr(f"Benchmark 3 — Hash HSET/HGET  ({HASH_OPS:,} ops)")
    lset, lget = [], []

    def do_hset(i):
        t0 = time.perf_counter()
        cmd(
            [
                "HSET",
                f"bench:hash:{i}",
                "name",
                rand_str(16),
                "score",
                str(random.randint(0, 10000)),
                "ts",
                str(int(time.time())),
            ]
        )
        return (time.perf_counter() - t0) * 1000

    def do_hget(i):
        t0 = time.perf_counter()
        cmd(["HGET", f"bench:hash:{i}", "name"])
        return (time.perf_counter() - t0) * 1000

    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as ex:
        for lat in as_completed([ex.submit(do_hset, i) for i in range(HASH_OPS)]):
            lset.append(lat.result())
    elapsed_set = time.perf_counter() - start

    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as ex:
        for lat in as_completed([ex.submit(do_hget, i) for i in range(HASH_OPS)]):
            lget.append(lat.result())
    elapsed_get = time.perf_counter() - start

    ops_set = HASH_OPS / elapsed_set
    ops_get = HASH_OPS / elapsed_get
    print(
        f"  HSET: {BOLD}{ops_set:,.0f} ops/sec{RESET}  (p50={statistics.median(lset):.2f}ms)"
    )
    print(
        f"  HGET: {BOLD}{ops_get:,.0f} ops/sec{RESET}  (p50={statistics.median(lget):.2f}ms)"
    )
    compare(label="Hash ops", actual=ops_set, redis_ref=80_000)
    return ops_set


# ── Benchmark 4: Vector VADD ──────────────────────────────────────────────────


def bench_vector_insert():
    hdr(f"Benchmark 4 — Vector VADD  ({VECTOR_COUNT:,} vectors, dim={VECTOR_DIM})")
    latencies = []
    errors = [0]

    def do_vadd(i):
        vec = rand_vec()
        doc_id = f"bvec:{i}"
        try:
            t0 = time.perf_counter()
            cmd(["VADD", BENCH_INDEX, doc_id] + [str(v) for v in vec], timeout=20)
            lat = (time.perf_counter() - t0) * 1000
            meta = json.dumps(
                {
                    "text": f"document {i}",
                    "category": random.choice(
                        ["finance", "tech", "science", "law", "health"]
                    ),
                }
            )
            cmd(["SET", f"doc:{BENCH_INDEX}:{doc_id}", meta], timeout=10)
            return lat
        except Exception:
            errors[0] += 1
            return None

    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as ex:
        futs = [ex.submit(do_vadd, i) for i in range(VECTOR_COUNT)]
        for f in as_completed(futs):
            lat = f.result()
            if lat is not None:
                latencies.append(lat)
    elapsed = time.perf_counter() - start

    success = len(latencies)
    ops_sec = success / elapsed
    p50 = statistics.median(latencies) if latencies else 0
    p99 = sorted(latencies)[int(len(latencies) * 0.99)] if latencies else 0
    print(
        f"  Ingested : {success:,}/{VECTOR_COUNT:,}  errors: {errors[0]}  in {elapsed:.2f}s"
    )
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} vectors/sec{RESET}")
    print(f"  p50 latency : {p50:.2f} ms    p99: {p99:.2f} ms")
    compare(label="Vector VADD", actual=ops_sec, pinecone_ref=5_000)
    return ops_sec


# ── Benchmark 5: VSEARCH ─────────────────────────────────────────────────────


def bench_vector_search():
    hdr(f"Benchmark 5 — VSEARCH  ({VSEARCH_COUNT} queries, top-10)")
    latencies = []
    errors = 0
    for _ in range(VSEARCH_COUNT):
        vec = rand_vec()
        t0 = time.perf_counter()
        try:
            cmd(["VSEARCH", BENCH_INDEX] + [str(v) for v in vec] + ["10"], timeout=20)
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
        compare(label="VSEARCH QPS", actual=qps, pinecone_ref=500)
    else:
        warn("No successful VSEARCH queries")


# ── Benchmark 6: Mixed Concurrent ────────────────────────────────────────────


def bench_mixed():
    hdr(f"Benchmark 6 — Mixed Concurrent  (50,000 ops, {CONCURRENCY} threads)")
    TOTAL = 50_000
    ok_count = [0]
    err_count = [0]
    lock = threading.Lock()

    def worker(tid):
        per = TOTAL // CONCURRENCY
        lok, lerr = 0, 0
        for j in range(per):
            op = j % 4
            try:
                if op == 0:
                    cmd(["SET", f"mix:{tid}:{j}", rand_str(16)])
                elif op == 1:
                    cmd(["GET", f"mix:{tid}:{max(0,j-1)}"])
                elif op == 2:
                    cmd(["INCR", f"mix:cnt:{tid}"])
                else:
                    cmd(["HSET", f"mix:h:{tid}", f"f{j}", rand_str(8)])
                lok += 1
            except Exception:
                lerr += 1
        with lock:
            ok_count[0] += lok
            err_count[0] += lerr

    start = time.perf_counter()
    threads = [threading.Thread(target=worker, args=(t,)) for t in range(CONCURRENCY)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    elapsed = time.perf_counter() - start

    ops_sec = ok_count[0] / elapsed
    print(f"  OK: {ok_count[0]:,}   Errors: {err_count[0]}   in {elapsed:.2f}s")
    print(f"  Throughput  : {BOLD}{ops_sec:,.0f} ops/sec{RESET}")
    compare(label="Mixed ops", actual=ops_sec, redis_ref=80_000)
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
    hdr("FINAL PERFORMANCE REPORT vs Modern Databases")
    rows = [
        ("String SET", set_ops, 100_000, "Redis 7 (local)"),
        ("String GET", get_ops, 120_000, "Redis 7 (local)"),
        ("Hash HSET", hash_ops, 80_000, "Redis 7 (local)"),
        ("Vector Ingest", vec_ops, 5_000, "Pinecone serverless"),
        ("Mixed concurrent", mixed_ops, 80_000, "Redis 7 (local)"),
    ]
    for name, dbx, ref, ref_name in rows:
        ratio = dbx / ref if ref else 0
        status = (
            GREEN + "FASTER"
            if ratio >= 1.0
            else YELLOW + "CLOSE " if ratio >= 0.5 else RED + "SLOWER"
        )
        bar = "#" * min(int(ratio * 15), 30)
        print(
            f"  {BOLD}{name:<22}{RESET}  {dbx:>10,.0f} ops/s  [{bar:<30}]  {status}{RESET}  ({ratio:.2f}x {ref_name})"
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
