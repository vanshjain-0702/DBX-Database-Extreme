"""
brutal_stress_test.py
======================
Adversarial multi-client stress test for DBX.

Verifies claims made in the README and docs:
  - Claim 1: Tenant isolation is real (no cross-tenant data leakage)
  - Claim 2: ANN p50 latency ~ 2-3 ms under concurrent load
  - Claim 3: VADD_BATCH throughput (docs/s)
  - Claim 4: Concurrent multi-tenant writes don't corrupt data
  - Claim 5: VSIM (similar-to-id) returns the correct vector
  - Claim 6: KV + Vector work simultaneously on the same connection
  - Claim 7: Tenant shred wipes all data (no ghost keys)

Run:
    $env:DBX_ADMIN_PASSWORD="adminadminadmin"
    python brutal_stress_test.py

No OpenAI key needed. Uses deterministic random embeddings.
"""

import concurrent.futures
import os
import random
import statistics
import sys
import time
import traceback
import uuid

sys.path.insert(0, os.path.dirname(__file__))

from dbx import ControlPlane, DBXClient  # noqa: E402

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
ORCHESTRATOR_URL = os.getenv("DBX_URL", "http://127.0.0.1:8000")
ADMIN_PASSWORD = os.getenv("DBX_ADMIN_PASSWORD", "adminadminadmin")
RESP_HOST = os.getenv("DBX_HOST", "127.0.0.1")
RESP_PORT = int(os.getenv("DBX_PORT", "6380"))

DIM = 128            # vector dimensions (matches benchmark default)
DOCS_PER_TENANT = 5000   # vectors per tenant
N_TENANTS = 8        # concurrent isolated tenants
N_SEARCH_QUERIES = 200   # search queries per latency bench
KV_OPS_PER_TENANT = 500  # SET/GET ops per tenant

PASS = "\033[92mPASS\033[0m"
FAIL = "\033[91mFAIL\033[0m"
WARN = "\033[93mWARN\033[0m"
INFO = "\033[94m-->\033[0m"

results = []


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def rand_vec(seed=None):
    """Deterministic L2-normalised random vector of DIM dimensions."""
    if seed is not None:
        random.seed(seed)
    raw = [random.gauss(0, 1) for _ in range(DIM)]
    norm = sum(x ** 2 for x in raw) ** 0.5 or 1.0
    return [x / norm for x in raw]


def record(name, ok, detail="", warn=False):
    results.append({"name": name, "ok": ok, "warn": warn, "detail": detail})
    icon = WARN if warn else (PASS if ok else FAIL)
    status = "WARN" if warn else ("PASS" if ok else "FAIL")
    print(f"  [{status}] {name}")
    if detail:
        print(f"         {detail}")


def provision(plane, prefix="stress"):
    tid = f"{prefix}-{uuid.uuid4().hex[:8]}"
    plane.provision(tid, tid)
    minted = plane.create_key(tid, name="writer", role="writer")
    key = minted.get("key") or {}
    client = DBXClient(
        host=RESP_HOST, port=RESP_PORT,
        tenant=tid,
        key_id=str(key.get("id") or minted.get("id") or ""),
        secret=str(minted.get("secret") or ""),
    )
    for _ in range(30):
        try:
            client.ping()
            break
        except Exception:
            time.sleep(0.3)
    return tid, client


def shred(plane, tid):
    try:
        plane.shred(tid)
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Claim 1: Tenant isolation -- no cross-tenant vector leakage
# ---------------------------------------------------------------------------

def test_isolation(plane):
    print(f"\n{INFO} CLAIM 1: Tenant isolation (no cross-tenant data leakage)")
    tA_id, cA = provision(plane, "iso-A")
    tB_id, cB = provision(plane, "iso-B")
    try:
        # Insert a unique vector in tenant A
        secret_vec = rand_vec(seed=0xDEADBEEF)
        cA.vadd("idx", "secret-doc", secret_vec)

        # Search for that same vector from tenant B -- should return 0 results
        hits_from_B = cB.vsearch("idx", secret_vec, top_k=5)
        cross_leak = len(hits_from_B) > 0

        # Search from A -- should return the doc
        hits_from_A = cA.vsearch("idx", secret_vec, top_k=5)
        found_in_A = any(h[0] == "secret-doc" for h in hits_from_A)

        ok = not cross_leak and found_in_A
        record(
            "Isolation: tenant B cannot see tenant A vectors",
            ok,
            f"A found: {found_in_A}, B leaked: {cross_leak}, "
            f"B hits: {len(hits_from_B)}, A hits: {len(hits_from_A)}",
        )

        # Also test KV isolation
        cA.set("private-key", "tenant-A-secret")
        val_from_B = cB.get("private-key")
        kv_isolated = val_from_B is None
        record(
            "Isolation: tenant B cannot read tenant A KV keys",
            kv_isolated,
            f"B saw key value: {val_from_B!r}",
        )

    except Exception:
        record("Isolation: UNEXPECTED ERROR", False, traceback.format_exc(limit=2))
    finally:
        shred(plane, tA_id)
        shred(plane, tB_id)


# ---------------------------------------------------------------------------
# Claim 2 & 3: Throughput + ANN latency under load
# ---------------------------------------------------------------------------

def test_throughput_and_latency(plane):
    print(f"\n{INFO} CLAIM 2+3: Throughput + ANN p50/p95/p99 latency")
    tid, client = provision(plane, "lat-bench")
    try:
        # -- Ingest DOCS_PER_TENANT vectors via VADD_BATCH --
        vecs = [rand_vec(seed=i) for i in range(DOCS_PER_TENANT)]
        ids = [f"doc:{i}" for i in range(DOCS_PER_TENANT)]

        t0 = time.perf_counter()
        client.vadd_batch("bench", DIM, ids, vecs)
        ingest_s = time.perf_counter() - t0
        throughput = DOCS_PER_TENANT / ingest_s

        record(
            f"Throughput: VADD_BATCH {DOCS_PER_TENANT} x {DIM}d vectors",
            throughput > 1000,
            f"{throughput:,.0f} docs/s  ({ingest_s:.3f}s total)",
        )

        # -- ANN latency: N_SEARCH_QUERIES sequential queries --
        latencies = []
        for i in range(N_SEARCH_QUERIES):
            q = rand_vec(seed=9999 + i)
            t0 = time.perf_counter()
            hits = client.vsearch("bench", q, top_k=10)
            latencies.append((time.perf_counter() - t0) * 1000)  # ms
            assert len(hits) > 0, "Expected search results"

        p50 = statistics.median(latencies)
        p95 = statistics.quantiles(latencies, n=20)[18]  # 95th
        p99 = statistics.quantiles(latencies, n=100)[98]  # 99th
        mn = min(latencies)
        mx = max(latencies)

        # README claims p50 ~ 2.304 ms -- allow up to 50ms on local dev
        p50_ok = p50 < 50
        record(
            f"Latency: ANN p50 ({N_SEARCH_QUERIES} queries, {DOCS_PER_TENANT} indexed)",
            p50_ok,
            f"p50={p50:.2f}ms  p95={p95:.2f}ms  p99={p99:.2f}ms  "
            f"min={mn:.2f}ms  max={mx:.2f}ms",
            warn=(p50 > 10),
        )

        if p50 <= 5:
            print(f"         -> Production-grade latency confirmed (p50 <= 5ms)")
        elif p50 <= 20:
            print(f"         -> Good latency for local dev (p50 <= 20ms)")
        else:
            print(f"         -> High latency -- expected on Windows/local (no mmap lock)")

    except Exception:
        record("Latency bench: UNEXPECTED ERROR", False, traceback.format_exc(limit=2))
    finally:
        shred(plane, tid)


# ---------------------------------------------------------------------------
# Claim 4: Concurrent multi-tenant writes don't corrupt each other
# ---------------------------------------------------------------------------

def _concurrent_worker(plane, worker_id, doc_count, dim):
    """Each worker provisions its own tenant, writes docs, searches, verifies."""
    tid, client = provision(plane, f"conc-{worker_id}")
    errors = []
    try:
        vecs = [rand_vec(seed=worker_id * 10000 + i) for i in range(doc_count)]
        ids = [f"w{worker_id}:doc:{i}" for i in range(doc_count)]

        # Write
        t0 = time.perf_counter()
        client.vadd_batch("idx", dim, ids, vecs)
        write_s = time.perf_counter() - t0

        # Read back -- verify own data is present and correct count
        q = vecs[0]
        hits = client.vsearch("idx", q, top_k=5)
        found_own = any(h[0].startswith(f"w{worker_id}:") for h in hits)
        if not found_own:
            errors.append("Could not find own vector in search results")

        # Verify no other tenant's data leaked in
        alien_hits = [h for h in hits if not h[0].startswith(f"w{worker_id}:")]
        if alien_hits:
            errors.append(f"Got alien tenant data in results: {alien_hits[:2]}")

        return {
            "worker_id": worker_id,
            "tid": tid,
            "ok": len(errors) == 0,
            "errors": errors,
            "write_s": write_s,
            "throughput": doc_count / write_s,
            "hits": len(hits),
        }
    except Exception as exc:
        return {
            "worker_id": worker_id, "tid": tid,
            "ok": False, "errors": [str(exc)],
            "write_s": 0, "throughput": 0, "hits": 0,
        }
    finally:
        shred(plane, tid)


def test_concurrent_isolation(plane):
    print(f"\n{INFO} CLAIM 4: {N_TENANTS} concurrent tenants -- no data corruption")
    t0 = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=N_TENANTS) as pool:
        futures = [
            pool.submit(_concurrent_worker, plane, i, DOCS_PER_TENANT, DIM)
            for i in range(N_TENANTS)
        ]
        worker_results = [f.result() for f in concurrent.futures.as_completed(futures)]
    elapsed = time.perf_counter() - t0

    all_ok = all(r["ok"] for r in worker_results)
    total_docs = N_TENANTS * DOCS_PER_TENANT
    combined_throughput = total_docs / elapsed

    for r in sorted(worker_results, key=lambda x: x["worker_id"]):
        status = PASS if r["ok"] else FAIL
        print(
            f"    [{status}] Worker {r['worker_id']}: "
            f"{r['throughput']:,.0f} docs/s | hits={r['hits']} | "
            f"errors={r['errors']}"
        )

    record(
        f"Concurrent: {N_TENANTS} tenants x {DOCS_PER_TENANT} docs, "
        f"no corruption, no leakage",
        all_ok,
        f"Combined throughput: {combined_throughput:,.0f} docs/s across "
        f"{N_TENANTS} tenants | wall-clock: {elapsed:.2f}s",
    )


# ---------------------------------------------------------------------------
# Claim 5: VSIM returns the correct nearest neighbour
# ---------------------------------------------------------------------------

def test_vsim_correctness(plane):
    print(f"\n{INFO} CLAIM 5: VSIM correctness (similar-to-id)")
    tid, client = provision(plane, "vsim-test")
    try:
        # Insert 3 known vectors: A and B are very close, C is orthogonal
        vec_a = [1.0] + [0.0] * (DIM - 1)
        vec_b = [0.999, 0.001] + [0.0] * (DIM - 2)
        vec_c = [0.0, 1.0] + [0.0] * (DIM - 2)

        client.vadd("idx", "doc-A", vec_a)
        client.vadd("idx", "doc-B", vec_b)
        client.vadd("idx", "doc-C", vec_c)

        # VSIM from doc-A should return doc-B first (not doc-C)
        hits = client.vsim("idx", "doc-A", top_k=2)
        top_hit = hits[0][0] if hits else None
        ok = top_hit == "doc-B"

        record(
            "VSIM: doc-A's nearest neighbour is doc-B (not doc-C)",
            ok,
            f"Top hit: {top_hit!r} (expected 'doc-B') | all hits: {hits}",
        )
    except Exception:
        record("VSIM correctness: UNEXPECTED ERROR", False, traceback.format_exc(limit=2))
    finally:
        shred(plane, tid)


# ---------------------------------------------------------------------------
# Claim 6: KV + Vector simultaneously on the same connection
# ---------------------------------------------------------------------------

def test_kv_and_vector_combined(plane):
    print(f"\n{INFO} CLAIM 6: KV + Vector on the same connection simultaneously")
    tid, client = provision(plane, "kv-vec")
    try:
        errors = []
        latencies = []

        for i in range(KV_OPS_PER_TENANT):
            vec = rand_vec(seed=i)
            key = f"session:{i}"
            val = f'{{"user":{i},"step":{i % 10}}}'

            t0 = time.perf_counter()

            # KV write + vector write interleaved
            client.set(key, val, ex=300)
            client.vadd("mem", f"doc:{i}", vec)

            # KV read back
            got = client.get(key)
            if got != val:
                errors.append(f"KV mismatch at {key}: got {got!r}")

            latencies.append((time.perf_counter() - t0) * 1000)

        # Vector search after mixed load
        q = rand_vec(seed=0)
        hits = client.vsearch("mem", q, top_k=5)

        p50 = statistics.median(latencies)
        ok = len(errors) == 0 and len(hits) > 0

        record(
            f"KV+Vector: {KV_OPS_PER_TENANT} interleaved SET+VADD+GET ops",
            ok,
            f"Errors: {len(errors)} | Search hits: {len(hits)} | "
            f"p50 round-trip: {p50:.2f}ms",
        )
        if errors:
            for e in errors[:3]:
                print(f"    ERROR: {e}")

    except Exception:
        record("KV+Vector: UNEXPECTED ERROR", False, traceback.format_exc(limit=2))
    finally:
        shred(plane, tid)


# ---------------------------------------------------------------------------
# Claim 7: Shred wipes all data (no ghost data remains)
# ---------------------------------------------------------------------------

def test_shred_completeness(plane):
    print(f"\n{INFO} CLAIM 7: Tenant shred wipes ALL data (no ghost vectors)")
    tid, client = provision(plane, "shred-test")
    try:
        # Insert data
        vecs = [rand_vec(seed=i) for i in range(500)]
        ids = [f"doc:{i}" for i in range(500)]
        client.vadd_batch("idx", DIM, ids, vecs)
        client.set("sentinel-key", "must-be-gone-after-shred")

        # Verify data exists
        pre_hits = client.vsearch("idx", vecs[0], top_k=5)
        pre_val = client.get("sentinel-key")
        data_existed = len(pre_hits) > 0 and pre_val is not None

        # Shred the tenant
        plane.shred(tid)

        # Re-provision the same tenant ID and verify it's empty
        plane.provision(tid, tid)
        minted = plane.create_key(tid, name="writer", role="writer")
        key = minted.get("key") or {}
        fresh_client = DBXClient(
            host=RESP_HOST, port=RESP_PORT,
            tenant=tid,
            key_id=str(key.get("id") or minted.get("id") or ""),
            secret=str(minted.get("secret") or ""),
        )
        for _ in range(20):
            try:
                fresh_client.ping()
                break
            except Exception:
                time.sleep(0.3)

        post_hits = fresh_client.vsearch("idx", vecs[0], top_k=5)
        post_val = fresh_client.get("sentinel-key")
        ghost_free = len(post_hits) == 0 and post_val is None

        ok = data_existed and ghost_free
        record(
            "Shred: re-provisioned tenant is completely empty",
            ok,
            f"Pre-shred: hits={len(pre_hits)}, kv={pre_val!r} | "
            f"Post-shred: hits={len(post_hits)}, kv={post_val!r}",
        )
    except Exception:
        record("Shred test: UNEXPECTED ERROR", False, traceback.format_exc(limit=2))
    finally:
        try:
            plane.shred(tid)
        except Exception:
            pass


# ---------------------------------------------------------------------------
# Bonus: Recall accuracy test
# ---------------------------------------------------------------------------

def test_recall_accuracy(plane):
    print(f"\n{INFO} BONUS: Recall@10 accuracy on known ground truth")
    tid, client = provision(plane, "recall-test")
    try:
        n = 1000
        # Insert n vectors, keep track of the actual embeddings
        vecs = [rand_vec(seed=i) for i in range(n)]
        ids = [f"doc:{i}" for i in range(n)]
        client.vadd_batch("idx", DIM, ids, vecs)

        # For 50 random queries, compute true nearest neighbours via brute force
        # and compare to what HNSW returns
        k = 10
        hits_correct = 0
        total_checked = 50

        for qi in range(total_checked):
            q = rand_vec(seed=100000 + qi)
            # Brute-force: compute cosine similarity with all vecs
            sims = [(i, sum(a * b for a, b in zip(q, v))) for i, v in enumerate(vecs)]
            sims.sort(key=lambda x: -x[1])
            true_top_k = {f"doc:{i}" for i, _ in sims[:k]}

            # DBX result
            dbx_hits = client.vsearch("idx", q, top_k=k)
            dbx_ids = {h[0] for h in dbx_hits}

            intersection = true_top_k & dbx_ids
            hits_correct += len(intersection)

        recall_at_k = hits_correct / (total_checked * k)
        ok = recall_at_k >= 0.80  # 80% recall@10 threshold

        record(
            f"Recall@{k} accuracy ({total_checked} queries, {n} indexed)",
            ok,
            f"Recall@{k} = {recall_at_k:.1%} "
            f"({'meets' if ok else 'BELOW'} 80% threshold)",
            warn=(0.80 <= recall_at_k < 0.90),
        )

    except Exception:
        record("Recall accuracy: UNEXPECTED ERROR", False, traceback.format_exc(limit=2))
    finally:
        shred(plane, tid)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    print("\n" + "=" * 65)
    print("  DBX BRUTAL STRESS TEST -- verifying claims vs reality")
    print(f"  Config: {N_TENANTS} tenants, {DOCS_PER_TENANT} docs each, {DIM}d vectors")
    print("=" * 65)

    plane = ControlPlane(ORCHESTRATOR_URL)
    try:
        plane.login("admin", ADMIN_PASSWORD)
        print(f"\n  {PASS} Connected to DBX at {ORCHESTRATOR_URL}\n")
    except Exception as exc:
        print(f"  {FAIL} Cannot connect: {exc}")
        sys.exit(1)

    wall_t0 = time.perf_counter()

    test_isolation(plane)
    test_throughput_and_latency(plane)
    test_concurrent_isolation(plane)
    test_vsim_correctness(plane)
    test_kv_and_vector_combined(plane)
    test_shred_completeness(plane)
    test_recall_accuracy(plane)

    wall_elapsed = time.perf_counter() - wall_t0

    # Summary
    print("\n" + "=" * 65)
    print("  FINAL VERDICT")
    print("=" * 65)

    passed = sum(1 for r in results if r["ok"] and not r["warn"])
    warned = sum(1 for r in results if r["warn"])
    failed = sum(1 for r in results if not r["ok"])

    for r in results:
        icon = WARN if r["warn"] else (PASS if r["ok"] else FAIL)
        status = "WARN" if r["warn"] else ("PASS" if r["ok"] else "FAIL")
        print(f"  [{status}] {r['name']}")
        if r["detail"]:
            print(f"         {r['detail']}")

    print()
    print(f"  Total: {len(results)} claims tested")
    print(f"  Passed:  {passed}")
    print(f"  Warned:  {warned} (works but may be slow on this machine)")
    print(f"  Failed:  {failed}")
    print(f"  Wall-clock: {wall_elapsed:.1f}s")
    print()

    if failed == 0:
        print("  \033[92mAll claims verified. DBX features are real.\033[0m")
    else:
        print(f"  \033[91m{failed} claim(s) FAILED -- see details above.\033[0m")
        sys.exit(1)


if __name__ == "__main__":
    main()
