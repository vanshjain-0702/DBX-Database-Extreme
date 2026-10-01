"""Live check: Jurix HTTP against the running DBX node."""

import json
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8091"


def call(method, path, body=None, token=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=90) as resp:
            text = resp.read().decode()
            return json.loads(text or "{}")
    except urllib.error.HTTPError as exc:
        raise SystemExit(f"{method} {path} -> {exc.code} {exc.read().decode()}") from exc


def main() -> None:
    health = call("GET", "/api/health")
    print("health", health["dbx"]["ok"], (health["dbx"].get("detail") or "")[:240])
    assert health["dbx"]["ok"], health
    email = f"a.okonkwo.{int(time.time())}@halemercer.test"
    reg = call(
        "POST",
        "/api/register",
        {"email": email, "password": "password123", "firm_name": "Hale and Mercer"},
    )
    token = reg["token"]
    print("user", reg["user"]["firm_name"], reg["user"]["firm_code"])
    created = call(
        "POST",
        "/api/matters",
        {"name": "Atlas Freight", "matter": "MSA Harborline"},
        token,
    )
    atlas = created["matter"]["id"]
    print("tenant", created["matter"]["tenant_id"])
    call("POST", f"/api/matters/{atlas}/sample", {"kind": "msa"}, token)
    audit = call("POST", f"/api/matters/{atlas}/audit", None, token)
    items = {i["key"]: i for i in audit["items"]}
    for key, item in items.items():
        print(key, item["state"], item["finding"], item["citation"], item["via"])
    assert items["indemnity"]["state"] == "high", items["indemnity"]
    assert items["law"]["state"] == "clear"
    assert items["breach"]["state"] == "missing"
    assert items["renewal"]["state"] == "medium"
    assert audit["read_path"].startswith("dbx://")

    north = call(
        "POST",
        "/api/matters",
        {"name": "Northwind Holdings", "matter": "DPA refresh"},
        token,
    )
    nid = north["matter"]["id"]
    call("POST", f"/api/matters/{nid}/sample", {"kind": "dpa"}, token)
    naudit = call("POST", f"/api/matters/{nid}/audit", None, token)
    nitems = {i["key"]: i for i in naudit["items"]}
    print("north", {k: v["state"] for k, v in nitems.items()}, "sibling", naudit["sibling_hits"])
    assert nitems["law"]["finding"] == "California"
    assert naudit["siblings_probed"] >= 1
    assert naudit["sibling_hits"] == 0

    again = call("POST", f"/api/matters/{atlas}/audit", None, token)
    print("atlas sibling", again["sibling_hits"], "probed", again["siblings_probed"])
    assert again["sibling_hits"] == 0

    doc_id = audit["documents"][0]["id"]
    doc = call("GET", f"/api/matters/{atlas}/document?doc_id={doc_id}", None, token)
    blob = " ".join(c["text"] for c in doc["chunks"])
    assert "three months" in blob
    assert doc["source"] == "dbx"
    print("document", len(doc["chunks"]), "chunks from", doc["tenant_id"])

    search = call(
        "POST",
        f"/api/matters/{atlas}/search",
        {"query": "aggregate liability cap fees months"},
        token,
    )
    assert search["hits"]
    print("search", search["hits"][0]["text"][:90])

    call(
        "POST",
        f"/api/matters/{atlas}/notes",
        {"item_key": "indemnity", "text": "Ask Harborline for twelve months."},
        token,
    )
    exported = call("POST", f"/api/matters/{atlas}/export", None, token)
    assert "Indemnity cap" in exported["body"]
    assert "Ask Harborline" in exported["body"]
    print("memo", len(exported["body"]))

    usage = call("GET", f"/api/matters/{atlas}/usage", None, token)
    print("usage", usage["usage"].get("status"), usage["usage"].get("vectors"))
    assert usage["usage"].get("vectors", 0) > 0

    hib = call("POST", f"/api/matters/{nid}/hibernate", None, token)
    print("hibernate", hib.get("hibernated"))
    wake = call("POST", f"/api/matters/{nid}/wake", None, token)
    print("wake", wake.get("hibernated"))
    off = call("POST", f"/api/matters/{nid}/offboard", None, token)
    print("purged", off["tenant_id"])
    matters = call("GET", "/api/matters", None, token)
    ids = [m["id"] for m in matters["matters"]]
    assert atlas in ids and nid not in ids
    still = call("POST", f"/api/matters/{atlas}/audit", None, token)
    assert still["items"]
    print("LIVE OK", still["tenant_id"])


if __name__ == "__main__":
    main()
