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
from dbx import DBXClient

db = DBXClient(
    host="127.0.0.1",
    port=6380,
    tenant="acme-corp",
    key_id="key-id",
    secret="one-time-key-secret",
)
db.set("session:42", '{"step": 1}')
db.vadd("memories", "doc:1", [0.1, 0.2, 0.9])
```

Worked paths: [`examples/quickstart.py`](../../examples/quickstart.py),
[`examples/langchain-rag`](../../examples/langchain-rag).

```bash
# from the repo root — no live node required
make python-check
```

The live LangChain test skips unless `DBX_TENANT`, `DBX_KEY_ID`, and
`DBX_SECRET` are set.
