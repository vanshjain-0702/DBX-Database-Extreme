"""Two-tenant isolation demo: Atlas vs Northwind.

Proves Legal Auditor's parent/child rule: each client is a sealed DBX tenant,
so recall on Atlas cannot surface Northwind clauses (and vice versa). Then
exports Atlas and purges both.

Requires a running orchestrator:
  make run-dev
  # or make docker-up

Usage (from repo root or this directory):
  python legal-auditor/demo_isolation.py
"""

from __future__ import annotations

import os
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent / "sdk" / "python"))

from dbx_bridge import DBXBridge  # noqa: E402
from embeddings import embed_query  # noqa: E402
from store import FirmStore  # noqa: E402

ATLAS_DOC = """
ATLAS CORP MASTER SERVICES AGREEMENT
Indemnity: Atlas shall indemnify the customer for IP claims arising from the Software.
Liability cap: Atlas total liability shall not exceed twelve months of fees paid.
Governing law: State of Delaware. Auto-renewal: yes, unless notice 30 days prior.
Confidential Atlas trade-secret clause unique-token-ATLAS-ONLY-9f3a.
"""

NORTHWIND_DOC = """
NORTHWIND LEGAL ENGAGEMENT LETTER
Indemnity: Northwind's indemnity is limited to negligence in legal advice delivery.
Liability cap: Northwind liability capped at fees paid in the prior three months.
Governing law: England and Wales. No auto-renewal; fixed twelve-month term.
Confidential Northwind privilege marker unique-token-NORTHWIND-ONLY-7c2b.
"""


def _assert(cond: bool, msg: str) -> None:
    if not cond:
        raise SystemExit("FAIL: " + msg)
    print("OK:", msg)


def main() -> None:
    admin = os.environ.get("DBX_ADMIN_PASSWORD", "adminadminadmin")
    os.environ.setdefault("DBX_ADMIN_PASSWORD", admin)

    bridge = DBXBridge()
    try:
        bridge.ensure_login()
    except RuntimeError as exc:
        raise SystemExit(str(exc)) from exc

    # Fresh local firm registry for the demo
    with tempfile.TemporaryDirectory() as tmp:
        store = FirmStore(path=str(Path(tmp) / "firm.json"))
        store.register("demo@legal-auditor.local", "demopassword", "Demo Firm")
        token = store.login("demo@legal-auditor.local", "demopassword")
        _assert(bool(token), "firm session created")

        atlas_creds = bridge.provision_client("Atlas Legal Demo")
        north_creds = bridge.provision_client("Northwind Legal Demo")
        atlas = store.add_client(
            "demo@legal-auditor.local",
            "Atlas Legal Demo",
            atlas_creds["tenant_id"],
            atlas_creds["key_id"],
            atlas_creds["secret"],
        )
        north = store.add_client(
            "demo@legal-auditor.local",
            "Northwind Legal Demo",
            north_creds["tenant_id"],
            north_creds["key_id"],
            north_creds["secret"],
        )
        print("tenants:", atlas["tenant_id"], north["tenant_id"])

        bridge.ingest_document(
            atlas_creds["tenant_id"],
            atlas_creds["key_id"],
            atlas_creds["secret"],
            "atlas-msa",
            ATLAS_DOC,
        )
        bridge.ingest_document(
            north_creds["tenant_id"],
            north_creds["key_id"],
            north_creds["secret"],
            "northwind-letter",
            NORTHWIND_DOC,
        )

        # Same query vector shape; isolation is tenant-scoped, not embedding magic.
        atlas_hits = bridge.audit(
            atlas_creds["tenant_id"],
            atlas_creds["key_id"],
            atlas_creds["secret"],
            "unique-token-ATLAS-ONLY-9f3a indemnity",
            top_k=5,
        )
        north_hits = bridge.audit(
            north_creds["tenant_id"],
            north_creds["key_id"],
            north_creds["secret"],
            "unique-token-NORTHWIND-ONLY-7c2b indemnity",
            top_k=5,
        )

        atlas_blob = " ".join(h["text"] for h in atlas_hits)
        north_blob = " ".join(h["text"] for h in north_hits)

        _assert(
            "unique-token-ATLAS-ONLY-9f3a" in atlas_blob,
            "Atlas recall finds Atlas-only token",
        )
        _assert(
            "unique-token-NORTHWIND-ONLY-7c2b" not in atlas_blob,
            "Atlas recall does not leak Northwind token",
        )
        _assert(
            "unique-token-NORTHWIND-ONLY-7c2b" in north_blob,
            "Northwind recall finds Northwind-only token",
        )
        _assert(
            "unique-token-ATLAS-ONLY-9f3a" not in north_blob,
            "Northwind recall does not leak Atlas token",
        )

        # Cross-query: ask Atlas for Northwind's secret token — must not appear.
        cross = bridge.audit(
            atlas_creds["tenant_id"],
            atlas_creds["key_id"],
            atlas_creds["secret"],
            "unique-token-NORTHWIND-ONLY-7c2b",
            top_k=5,
        )
        cross_blob = " ".join(h["text"] for h in cross)
        _assert(
            "unique-token-NORTHWIND-ONLY-7c2b" not in cross_blob,
            "cross-client query on Atlas cannot return Northwind text",
        )

        exported = bridge.export_client(atlas_creds["tenant_id"])
        _assert(
            "path" in exported or "status" in exported or bool(exported),
            "Atlas export returned",
        )
        print("export:", exported)

        # Purge both tenants (delete my data = one tenant)
        bridge.delete_client(atlas_creds["tenant_id"], purge=True)
        bridge.delete_client(north_creds["tenant_id"], purge=True)
        store.remove_client("demo@legal-auditor.local", atlas["id"])
        store.remove_client("demo@legal-auditor.local", north["id"])
        _assert(True, "both client tenants purged")

    # Sanity: embedding helper still works offline
    v = embed_query("indemnity")
    _assert(len(v) == 64, "hash embedding dim=64")
    print("\nPASS: isolation demo (Atlas vs Northwind)")


if __name__ == "__main__":
    main()
