"""Legal Auditor API — thin child product on DBX.

Run from this directory:
  pip install -r requirements.txt
  uvicorn app:app --reload --port 8090
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Dict, Optional

from fastapi import Depends, FastAPI, Header, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from dbx_bridge import DBXBridge
from store import FirmStore

app = FastAPI(title="Legal Auditor", version="0.1.0")
store = FirmStore()
bridge = DBXBridge()

STATIC = Path(__file__).resolve().parent / "static"
if STATIC.is_dir():
    app.mount("/static", StaticFiles(directory=str(STATIC)), name="static")


class RegisterBody(BaseModel):
    email: str
    password: str = Field(min_length=8)
    firm_name: str = ""


class LoginBody(BaseModel):
    email: str
    password: str


class ClientBody(BaseModel):
    name: str = Field(min_length=1)


class IngestBody(BaseModel):
    doc_name: str = Field(min_length=1)
    text: str = Field(min_length=1)


class AuditBody(BaseModel):
    query: str = Field(min_length=1)
    top_k: int = Field(default=5, ge=1, le=20)


def current_user(authorization: Optional[str] = Header(default=None)) -> Dict[str, Any]:
    if not authorization or not authorization.lower().startswith("bearer "):
        raise HTTPException(status_code=401, detail="missing bearer token")
    token = authorization.split(" ", 1)[1].strip()
    user = store.user_for_token(token)
    if not user:
        raise HTTPException(status_code=401, detail="invalid session")
    return user


@app.get("/")
def index() -> FileResponse:
    return FileResponse(STATIC / "index.html")


@app.get("/api/health")
def health() -> Dict[str, str]:
    return {"status": "ok", "product": "legal-auditor", "parent": "dbx"}


@app.post("/api/register")
def register(body: RegisterBody) -> Dict[str, Any]:
    try:
        user = store.register(body.email, body.password, body.firm_name)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    token = store.login(body.email, body.password)
    return {"user": user, "token": token}


@app.post("/api/login")
def login(body: LoginBody) -> Dict[str, str]:
    try:
        token = store.login(body.email, body.password)
    except ValueError as exc:
        raise HTTPException(status_code=401, detail=str(exc)) from exc
    return {"token": token}


@app.get("/api/me")
def me(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return {"email": user["email"], "firm_name": user["firm_name"]}


@app.get("/api/clients")
def list_clients(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return {"clients": store.list_clients(user["email"])}


@app.post("/api/clients")
def create_client(
    body: ClientBody, user: Dict[str, Any] = Depends(current_user)
) -> Dict[str, Any]:
    try:
        creds = bridge.provision_client(body.name)
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    client = store.add_client(
        user["email"],
        body.name,
        creds["tenant_id"],
        creds["key_id"],
        creds["secret"],
    )
    return {"client": client}


@app.post("/api/clients/{client_id}/ingest")
def ingest(
    client_id: str,
    body: IngestBody,
    user: Dict[str, Any] = Depends(current_user),
) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        result = bridge.ingest_document(
            client["tenant_id"],
            client["key_id"],
            client["secret"],
            body.doc_name,
            body.text,
        )
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    return result


@app.post("/api/clients/{client_id}/audit")
def audit(
    client_id: str,
    body: AuditBody,
    user: Dict[str, Any] = Depends(current_user),
) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        hits = bridge.audit(
            client["tenant_id"],
            client["key_id"],
            client["secret"],
            body.query,
            top_k=body.top_k,
        )
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    return {"query": body.query, "citations": hits}


@app.get("/api/clients/{client_id}/usage")
def usage(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        return bridge.usage(client["tenant_id"])
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/api/clients/{client_id}/export")
def export_client(
    client_id: str, user: Dict[str, Any] = Depends(current_user)
) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        return bridge.export_client(client["tenant_id"])
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/api/clients/{client_id}/hibernate")
def hibernate(
    client_id: str, user: Dict[str, Any] = Depends(current_user)
) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        return bridge.hibernate_client(client["tenant_id"])
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/api/clients/{client_id}/wake")
def wake(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        return bridge.wake_client(client["tenant_id"])
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.delete("/api/clients/{client_id}")
def delete_client(
    client_id: str, user: Dict[str, Any] = Depends(current_user)
) -> Dict[str, Any]:
    client = store.get_client(user["email"], client_id)
    if not client:
        raise HTTPException(status_code=404, detail="client not found")
    try:
        receipt = bridge.delete_client(client["tenant_id"], purge=True)
    except Exception as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    store.remove_client(user["email"], client_id)
    return {"deleted": client_id, "tenant_id": client["tenant_id"], "receipt": receipt}
