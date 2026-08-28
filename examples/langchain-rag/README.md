# LangChain + DBX (one tenant)

DBX is not a shared vector database with a metadata filter. Point this example at
**one tenant** over RESP `:6380`. Session KV and embeddings share that engine.

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

`session_memory.py` uses `FakeEmbeddings` so you do not need an OpenAI key.
The store talks RESP through `DBXClient` — not the operator JWT on `:8000`.
