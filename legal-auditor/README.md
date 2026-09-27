# Legal Auditor

First **child product** of the DBX parent platform: per-client legal document
memory and audit recall. Each law-firm client is one sealed DBX tenant.

| Doc | Purpose |
|---|---|
| [PRD.md](PRD.md) | One-page product definition |
| [BOUNDARIES.md](BOUNDARIES.md) | Parent/child ownership rules |
| [DESIGN_PARTNERS.md](DESIGN_PARTNERS.md) | How to run 1–3 design partners |
| [BRAND_PLAYBOOK.md](BRAND_PLAYBOOK.md) | Activate after partner signal |

## Prerequisites

1. DBX orchestrator running (`make run-dev` or `make docker-up` from repo root).
2. Python 3.10+.

```bash
cd legal-auditor
pip install -r requirements.txt
pip install -e ../sdk/python
```

## Run the app

```bash
# from legal-auditor/
set DBX_ADMIN_PASSWORD=adminadminadmin   # Windows PowerShell: $env:DBX_ADMIN_PASSWORD=...
uvicorn app:app --reload --port 8090
```

Open http://127.0.0.1:8090 — register a firm, create a client (provisions a DBX
tenant), paste contract text, run an audit query.

## Isolation demo (Atlas vs Northwind)

```bash
python demo_isolation.py
```

Creates two tenants, ingests distinct secret tokens, asserts no cross-client
recall, exports Atlas, purges both.

## Unit tests (no live node)

```bash
python -m pytest test_unit.py -q
```

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `DBX_CONTROL_URL` | `http://127.0.0.1:8000` | Orchestrator |
| `DBX_RESP_HOST` / `DBX_RESP_PORT` | `127.0.0.1` / `6380` | RESP ingress |
| `DBX_ADMIN_PASSWORD` | `adminadminadmin` | Control-plane login |
| `EMBEDDING_MODE` | `hash` | `hash` or `openai` |
| `LA_STORE_PATH` | `legal-auditor/data/firm_store.json` | Firm registry |

Embeddings stay in this app. DBX stores floats you send — it does not run MiniLM.
