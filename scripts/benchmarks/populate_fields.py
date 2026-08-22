import json
import os

import requests


def add_docs(token, index_name, texts):
    # Fake embedding: dimension 384
    # We just create some dummy vectors
    for i, t in enumerate(texts):
        vector = [0.1 * i] * 384
        doc_id = f"doc_{i}"

        # VADD creates the index on its first write.
        payload = {"command": ["VADD", index_name, doc_id] + [str(v) for v in vector]}
        requests.post(
            "http://localhost:8000/t/test-tenant/query",
            json=payload,
            headers={"Authorization": "Bearer " + token},
        ).raise_for_status()

        # Add metadata
        meta_payload = {
            "command": [
                "SET",
                f"doc:{index_name}:{doc_id}",
                json.dumps({"page_content": t}),
            ]
        }
        requests.post(
            "http://localhost:8000/t/test-tenant/query",
            json=meta_payload,
            headers={"Authorization": "Bearer " + token},
        ).raise_for_status()


if __name__ == "__main__":
    password = os.environ.get("DBX_ADMIN_PASSWORD")
    if not password:
        raise SystemExit("DBX_ADMIN_PASSWORD must be set")
    r = requests.post(
        "http://localhost:8000/api/login",
        json={"username": "admin", "password": password},
    )
    r.raise_for_status()
    token = r.json()["token"]

    print("Provisioning test-tenant...")
    requests.post(
        "http://localhost:8000/api/provision",
        json={"id": "test-tenant", "name": "Test Tenant"},
        headers={"Authorization": "Bearer " + token},
    )

    print("Populating test_index...")
    add_docs(
        token,
        "test_index",
        ["hello world", "database performance", "enterprise features"],
    )

    print("Populating quant_knowledge...")
    add_docs(
        token,
        "quant_knowledge",
        ["Quantum computing theory", "Superposition principles", "Entanglement in DBX"],
    )

    print("Populating star_output...")
    add_docs(
        token,
        "star_output",
        ["Star topology", "Graph nodes and edges", "Supernova events logged"],
    )

    print("Done populating.")
