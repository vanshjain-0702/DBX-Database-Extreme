# Python SDK

RESP client (`DBXClient`) and control-plane helper (`ControlPlane`) for one
DBX tenant. LangChain adapter: `langchain_dbx.DBXVectorStore`.

```bash
pip install -e .
pip install -e ".[langchain]"   # optional
```

The first command on `:6380` must be `AUTH tenantID:keyID secret`. Pass
`tenant`, `key_id`, and `secret` to `DBXClient`. Mint keys from the dashboard
**Tenant keys** page or `POST /api/v1/tenants/{id}/keys`.

```python
from dbx import ControlPlane, TenantMemory

plane = ControlPlane("http://127.0.0.1:8000")
plane.login("admin", admin_password)
mem = TenantMemory.open(plane, "acme-corp")
mem.remember("session:42", '{"step": 1}')
mem.remember("doc:1", "customer prefers dark mode", vector=[0.1, 0.2, 0.9])
print(mem.recall([0.1, 0.2, 0.8]))
mem.forget("doc:1")
plane.shred("acme-corp")
```

`DBXClient` is still there if you want raw RESP (`SET` / `VADD`). `TenantMemory`
is the product: one customer's working state and recall.

Worked paths: [`examples/quickstart.py`](../../examples/quickstart.py),
[`examples/langchain-rag`](../../examples/langchain-rag).

```bash
# from the repo root — no live node required
make python-check
```

The live LangChain test skips unless `DBX_TENANT`, `DBX_KEY_ID`, and
`DBX_SECRET` are set.
