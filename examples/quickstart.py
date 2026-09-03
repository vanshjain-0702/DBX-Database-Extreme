"""15-minute path: provision, AUTH, SET+VADD, usage, backup, purge.

Requires a local orchestrator (`make run-dev`) and: pip install redis
"""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "sdk", "python"))

from dbx import ControlPlane, DBXError, TenantMemory


def main() -> None:
    admin = os.environ.get("DBX_ADMIN_PASSWORD", "adminadminadmin")
    plane = ControlPlane("http://127.0.0.1:8000")
    try:
        plane.login("admin", admin)
    except DBXError as exc:
        raise SystemExit(f"{exc}\nStart the node first: make run-dev") from exc

    tenant = "acme-quickstart"
    mem = TenantMemory.open(plane, tenant, name="Acme quickstart")
    mem.remember("session:42", '{"thread":"onboarding","step":3}')
    mem.remember("doc:1", "onboarding notes", vector=[0.1, 0.2, 0.9])
    hits = mem.recall([0.1, 0.2, 0.8], top_k=5)
    print("recall:", hits)
    print("session:", mem.get("session:42"))
    print("usage:", plane.usage(tenant))

    exported = plane.export_tenant(tenant)
    print("export:", exported.get("path"))
    plane.delete(tenant, purge=True)
    print("purged", tenant)


if __name__ == "__main__":
    main()
