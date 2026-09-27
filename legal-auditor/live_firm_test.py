"""Live infra test: register fake firms, provision tenants, ingest, audit."""

from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8090"


def req(method: str, path: str, body=None, token=None):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(
        BASE + path, data=data, headers=headers, method=method
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        return exc.code, {"error": exc.read().decode()}
    except urllib.error.URLError as exc:
        return 0, {"error": str(exc.reason)}


FIRMS = [
    {
        "email": "ops@northwind-legal.test",
        "password": "NorthwindPass1",
        "firm_name": "Northwind Legal LLP",
        "clients": [
            (
                "Atlas Manufacturing",
                "atlas-msa",
                """ATLAS MANUFACTURING MSA
Governing Law: Delaware. Liability Cap: 12 months fees.
Indemnity: Atlas indemnifies Customer for IP infringement claims.
Auto-renewal: Yes, 30-day notice. Data residency: US-East.
Confidential marker: NW-ATLAS-TOKEN-AA11.""",
            ),
            (
                "Blue Harbor Logistics",
                "blue-harbor-nda",
                """BLUE HARBOR NDA
Governing Law: New York. Liability Cap: fees paid in prior 6 months.
Indemnity: mutual for breach of confidentiality.
No auto-renewal. Marker: NW-BLUE-TOKEN-BB22.""",
            ),
        ],
        "audit": "indemnity liability cap auto-renewal",
    },
    {
        "email": "admin@meridian-counsel.test",
        "password": "MeridianPass99",
        "firm_name": "Meridian Counsel PC",
        "clients": [
            (
                "Cedar Bank",
                "cedar-dpa",
                """CEDAR BANK DATA PROCESSING ADDENDUM
Governing Law: England and Wales. Liability Cap: 24 months fees.
Indemnity: Processor indemnifies Controller for unauthorized disclosure.
Subprocessors require prior written consent. Marker: MC-CEDAR-TOKEN-CC33.""",
            ),
        ],
        "audit": "subprocessors unauthorized disclosure",
    },
    {
        "email": "partner@oak-and-stone.test",
        "password": "OakStone2026!",
        "firm_name": "Oak & Stone Advocates",
        "clients": [
            (
                "Helios Energy",
                "helios-epc",
                """HELIOS ENERGY EPC AGREEMENT
Governing Law: Texas. Liability Cap: contract price.
Indemnity: Contractor indemnifies Owner for site injuries.
Force majeure includes grid failure. Marker: OS-HELIOS-TOKEN-DD44.""",
            ),
            (
                "Pinnacle Clinics",
                "pinnacle-baa",
                """PINNACLE CLINICS BAA
Governing Law: California. HIPAA business associate terms.
Liability Cap: $2,000,000. Breach notification within 72 hours.
Marker: OS-PINNACLE-TOKEN-EE55.""",
            ),
        ],
        "audit": "liability cap breach notification",
    },
]


def main() -> int:
    print("=== LIVE INFRA TEST: Legal Auditor ===\n")
    code, health = req("GET", "/api/health")
    print(f"health HTTP {code}: {health}")
    if code != 200:
        print("Legal Auditor is down on :8090")
        return 1

    summary = []
    for firm in FIRMS:
        print(f"-- Firm: {firm['firm_name']} <{firm['email']}>")
        code, reg = req(
            "POST",
            "/api/register",
            {
                "email": firm["email"],
                "password": firm["password"],
                "firm_name": firm["firm_name"],
            },
        )
        if code == 400 and "already" in str(reg).lower():
            code, login = req(
                "POST",
                "/api/login",
                {"email": firm["email"], "password": firm["password"]},
            )
            token = login.get("token")
            print(f"   login (existing): HTTP {code}")
        else:
            token = reg.get("token")
            print(f"   register: HTTP {code}")
        if not token:
            print(f"   FAIL auth: {reg}")
            summary.append((firm["firm_name"], False, "auth failed"))
            continue

        code, me = req("GET", "/api/me", token=token)
        print(f"   /api/me: {me.get('firm_name')} ({me.get('email')})")

        firm_ok = True
        for name, doc_name, text in firm["clients"]:
            code, created = req(
                "POST", "/api/clients", {"name": name}, token=token
            )
            if code >= 400:
                print(f"   create client {name}: FAIL HTTP {code} {created}")
                firm_ok = False
                continue
            cid = created["client"]["id"]
            tid = created["client"]["tenant_id"]
            print(f"   client OK: {name} -> tenant {tid}")

            code, ing = req(
                "POST",
                f"/api/clients/{cid}/ingest",
                {"doc_name": doc_name, "text": text},
                token=token,
            )
            if code >= 400:
                print(f"   ingest FAIL: {ing}")
                firm_ok = False
            else:
                print(f"   ingest OK: {ing.get('chunk_count')} chunks")

            code, audit = req(
                "POST",
                f"/api/clients/{cid}/audit",
                {"query": firm["audit"], "top_k": 3},
                token=token,
            )
            if code >= 400:
                print(f"   audit FAIL: {audit}")
                firm_ok = False
            else:
                cites = audit.get("citations") or []
                print(f"   audit OK: {len(cites)} citations")
                for hit in cites[:2]:
                    preview = (hit.get("text") or "")[:70].replace("\n", " ")
                    print(
                        f"      - {hit.get('id')} "
                        f"score={float(hit.get('score', 0)):.4f} | {preview}..."
                    )

            code, usage = req("GET", f"/api/clients/{cid}/usage", token=token)
            if code < 400:
                print(f"   usage OK: {json.dumps(usage)[:120]}...")

        code, listed = req("GET", "/api/clients", token=token)
        n = len(listed.get("clients") or [])
        print(f"   listed clients: {n}")
        summary.append(
            (firm["firm_name"], firm_ok and n >= len(firm["clients"]), f"{n} clients")
        )
        print()

    print("-- Cross-firm isolation spot check")
    _, login = req(
        "POST",
        "/api/login",
        {
            "email": "admin@meridian-counsel.test",
            "password": "MeridianPass99",
        },
    )
    token = login["token"]
    _, clients = req("GET", "/api/clients", token=token)
    cedar = next(c for c in clients["clients"] if "Cedar" in c["name"])
    _, audit = req(
        "POST",
        f"/api/clients/{cedar['id']}/audit",
        {"query": "OS-HELIOS-TOKEN-DD44 force majeure", "top_k": 5},
        token=token,
    )
    blob = " ".join(h.get("text") or "" for h in (audit.get("citations") or []))
    leaked = "OS-HELIOS-TOKEN-DD44" in blob
    print(f"   Meridian/Cedar query for Helios token leaked? {leaked}")
    summary.append(
        (
            "isolation",
            not leaked,
            "leak" if leaked else "no Helios token in Cedar recall",
        )
    )

    print("\n=== SUMMARY ===")
    all_ok = True
    for name, ok, note in summary:
        print(f"  [{'PASS' if ok else 'FAIL'}] {name}: {note}")
        all_ok = all_ok and ok
    print("\nOVERALL:", "PASS" if all_ok else "FAIL")
    return 0 if all_ok else 1


if __name__ == "__main__":
    sys.exit(main())
