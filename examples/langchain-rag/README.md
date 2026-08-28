# LangChain + DBX (one tenant)

DBX is not a shared vector database with a metadata filter. Point this example at
**one tenant** over RESP `:6380`. Session KV and embeddings share that engine.

The first command must be `AUTH tenantID:keyID secret`. The Python SDK does that
when you pass `tenant`, `key_id`, and `secret`.

## Prerequisites

- Python 3.10+
- `make run-dev`
- A tenant and writer key (see `examples/quickstart.py`)

```bash
pip install redis langchain-core
pip install -e ../../sdk/python
```

## Run

```bash
export DBX_TENANT=acme-quickstart
export DBX_KEY_ID=...
export DBX_SECRET=...
python session_memory.py
```

Without those three variables the script exits with a missing-env message instead of a stack trace. It will not talk to `:6380` until a tenant and writer key exist.

Unit tests do not need a running node:

```bash
python -m pytest sdk/python examples --ignore=sdk/python/.venv
```

The live LangChain test skips unless `DBX_TENANT`, `DBX_KEY_ID`, and `DBX_SECRET` are set.

`session_memory.py` uses a local fixed embedding stub so you do not need an OpenAI
key. The store talks RESP through `DBXClient` — not the operator JWT on `:8000`.
