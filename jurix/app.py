"""Jurix — legal audit site on DBX.

From the repo root, with the orchestrator already listening:

  pip install -r legal-auditor/requirements.txt
  $env:DBX_ADMIN_PASSWORD = "adminadminadmin"
  python -m uvicorn app:app --app-dir jurix --port 8091
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Dict, Optional

from fastapi import Depends, FastAPI, Header, HTTPException
from fastapi.responses import FileResponse, PlainTextResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from service import JurixService

app = FastAPI(title="Jurix", version="1.0.0")
svc = JurixService()

STATIC = Path(__file__).resolve().parent / "static"
ASSETS = Path(__file__).resolve().parents[1] / "website" / "assets"
if ASSETS.is_dir():
    app.mount("/assets", StaticFiles(directory=str(ASSETS)), name="assets")


class RegisterBody(BaseModel):
    email: str
    password: str = Field(min_length=8)
    firm_name: str = Field(min_length=1)


class LoginBody(BaseModel):
    email: str
    password: str
    code: str = ""


class MatterBody(BaseModel):
    name: str = Field(min_length=1)
    matter: str = "Matter"


class IngestBody(BaseModel):
    doc_name: str = Field(min_length=1)
    text: str = Field(min_length=1)


class SampleBody(BaseModel):
    kind: str = Field(pattern="^(msa|dpa)$")


class SearchBody(BaseModel):
    query: str = Field(min_length=1)


class NoteBody(BaseModel):
    item_key: str = Field(min_length=1)
    text: str = Field(min_length=1)


def current_user(authorization: Optional[str] = Header(default=None)) -> Dict[str, Any]:
    if not authorization or not authorization.lower().startswith("bearer "):
        raise HTTPException(status_code=401, detail="missing bearer token")
    token = authorization.split(" ", 1)[1].strip()
    user = svc.store.user_for_token(token)
    if not user:
        raise HTTPException(status_code=401, detail="invalid session")
    return user


def _call(fn, *args, **kwargs):
    try:
        return fn(*args, **kwargs)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc



@app.get("/api/health")
def health() -> Dict[str, Any]:
    dbx = svc.dbx_status()
    return {"status": "ok", "product": "jurix", "parent": "dbx", "dbx": dbx}


@app.get("/api/playbook")
def playbook() -> Dict[str, Any]:
    return svc.playbook()


@app.post("/api/register")
def register(body: RegisterBody) -> Dict[str, Any]:
    try:
        return svc.register(body.email, body.password, body.firm_name)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc


@app.post("/api/login")
def login(body: LoginBody) -> Dict[str, Any]:
    try:
        return svc.login(body.email, body.password, body.code)
    except ValueError as exc:
        raise HTTPException(status_code=401, detail=str(exc)) from exc


@app.get("/api/me")
def me(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return svc.user_view(user["email"])


@app.get("/api/matters")
def list_matters(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return {"matters": svc.list_matters(user["email"])}


@app.post("/api/matters")
def create_matter(body: MatterBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    matter = _call(svc.create_matter, user["email"], body.name, body.matter)
    return {"matter": matter}


@app.get("/api/matters/{client_id}")
def matter_detail(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.detail, user["email"], client_id)


@app.post("/api/matters/{client_id}/documents")
def ingest(client_id: str, body: IngestBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.ingest, user["email"], client_id, body.doc_name, body.text)


@app.post("/api/matters/{client_id}/sample")
def ingest_sample(client_id: str, body: SampleBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.ingest_sample, user["email"], client_id, body.kind)


@app.post("/api/matters/{client_id}/audit")
def audit(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.audit, user["email"], client_id)


@app.post("/api/matters/{client_id}/search")
def search(client_id: str, body: SearchBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.search, user["email"], client_id, body.query)


@app.post("/api/matters/{client_id}/notes")
def add_note(client_id: str, body: NoteBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.add_note, user["email"], client_id, body.item_key, body.text)


@app.get("/api/matters/{client_id}/document")
def read_document(
    client_id: str,
    doc_id: str = "",
    user: Dict[str, Any] = Depends(current_user),
) -> Dict[str, Any]:
    return _call(svc.read_document, user["email"], client_id, doc_id)


@app.post("/api/matters/{client_id}/export")
def export_memo(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.export_memo, user["email"], client_id)


@app.get("/api/exports")
def list_exports(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return {"exports": svc.list_exports(user["email"])}


@app.get("/api/exports/{export_id}")
def get_export(export_id: str, user: Dict[str, Any] = Depends(current_user)) -> PlainTextResponse:
    try:
        row = svc.get_export(user["email"], export_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    filename = f"jurix-{row['client_name']}-{row['kind']}.md".replace(" ", "-")
    return PlainTextResponse(
        row["body"],
        media_type="text/markdown",
        headers={"Content-Disposition": f'attachment; filename="{filename}"'},
    )


@app.post("/api/matters/{client_id}/offboard")
def offboard(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.offboard, user["email"], client_id)


@app.post("/api/matters/{client_id}/hibernate")
def hibernate(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.hibernate, user["email"], client_id)


@app.post("/api/matters/{client_id}/wake")
def wake(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.wake, user["email"], client_id)


@app.get("/api/matters/{client_id}/usage")
def usage(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.usage, user["email"], client_id)


# Mount static files at the root level (fallback after all API routes)
if STATIC.is_dir():
    app.mount("/", StaticFiles(directory=str(STATIC), html=True), name="static")
