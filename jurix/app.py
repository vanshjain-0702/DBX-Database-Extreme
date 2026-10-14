"""Jurix — legal audit site on DBX.

From the repo root, with the orchestrator already listening:

  pip install -r legal-auditor/requirements.txt
  $env:DBX_ADMIN_PASSWORD = "adminadminadmin"
  python -m uvicorn app:app --app-dir jurix --port 8091
"""

from __future__ import annotations

from contextlib import asynccontextmanager
from pathlib import Path
import logging
import re
import secrets
from typing import Any, Dict, List, Optional, Literal

from fastapi import Depends, FastAPI, Header, HTTPException, Request, Response, UploadFile, File, Form
from fastapi.responses import PlainTextResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from service import JurixService
from providers import ProviderUnavailable
from documents import MAX_TEXT
from web_security import SecurityMiddleware, session_cookies, clear_cookies

svc = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global svc
    if svc is None:
        svc = JurixService()
    yield


app = FastAPI(title="Jurix", version="1.0.0", lifespan=lifespan)
app.add_middleware(SecurityMiddleware)
logger = logging.getLogger("jurix.api")

STATIC = Path(__file__).resolve().parent / "static"
ASSETS = Path(__file__).resolve().parents[1] / "website" / "assets"
if ASSETS.is_dir():
    app.mount("/assets", StaticFiles(directory=str(ASSETS)), name="assets")


class RegisterBody(BaseModel):
    email: str = Field(min_length=3, max_length=254)
    password: str = Field(min_length=12, max_length=1024)
    firm_name: str = Field(default="", max_length=200)
    invite_token: str = Field(default="", max_length=200)


class LoginBody(BaseModel):
    email: str = Field(min_length=3, max_length=254)
    password: str = Field(min_length=1, max_length=1024)
    code: str = Field(default="", max_length=100)


class MatterBody(BaseModel):
    name: str = Field(min_length=1, max_length=200)
    matter: str = Field(default="Matter", min_length=1, max_length=200)


class IngestBody(BaseModel):
    doc_name: str = Field(min_length=1, max_length=240)
    text: str = Field(min_length=1, max_length=MAX_TEXT)
    supersedes: str = Field(default="", max_length=200)


class SampleBody(BaseModel):
    kind: str = Field(pattern="^(msa|dpa)$")


class SearchBody(BaseModel):
    query: str = Field(min_length=1, max_length=2000)


class NoteBody(BaseModel):
    item_key: str = Field(min_length=1, max_length=200)
    text: str = Field(min_length=1, max_length=10000)


class CustomRuleBody(BaseModel):
    id: str = Field(min_length=1, max_length=40, pattern="^[a-z0-9_-]+$")
    name: str = Field(min_length=1, max_length=100)
    kind: Literal["nda_term", "liability_amount", "semantic"]
    instructions: str = Field(default="", max_length=4000)
    threshold: int = Field(ge=1, le=1_000_000_000, strict=True)
    currency: Literal["USD", "INR", "EUR", "GBP"] = "USD"
    applies_to: Literal["all", "nda"] = "all"
    preferred_text: str = Field(default="", max_length=4000)
    authority_title: str = Field(default="", max_length=200)
    authority_url: str = Field(default="", max_length=2000)


class PolicyBody(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    liability_floor_months: int = Field(ge=1, le=120, strict=True)
    renewal_notice_days: int = Field(ge=1, le=365, strict=True)
    breach_notice_hours: int = Field(ge=1, le=720, strict=True)
    accepted_laws: List[str] = Field(min_length=1, max_length=20)
    enabled_checks: List[str] = Field(max_length=8)
    custom_rules: List[CustomRuleBody] = Field(default_factory=list, max_length=20)


class ReviewBody(BaseModel):
    status: str = Field(pattern="^(open|in_review|resolved|accepted_risk)$")
    note: str = Field(default="", max_length=10000)
    suggestion_decision: Literal["pending", "accepted", "rejected"] = "pending"


class BillingPolicyBody(BaseModel):
    name: str = Field(default="Client pre-bill guidelines", min_length=1, max_length=100)
    require_task_code: bool = Field(default=True, strict=True)
    require_activity_code: bool = Field(default=True, strict=True)
    flag_block_billing: bool = Field(default=True, strict=True)
    min_narrative_words: int = Field(default=5, ge=1, le=100, strict=True)
    max_entry_hours: float = Field(default=8, gt=0, le=24, strict=True)
    routine_max_hours: float = Field(default=1, gt=0, le=24, strict=True)
    routine_terms: List[str] = Field(default_factory=lambda: ["scheduling", "calendaring", "schedule meeting", "calendar deadline"], max_length=30)


class BillingBody(BaseModel):
    csv_text: str = Field(min_length=1, max_length=1_000_000)
    policy: BillingPolicyBody = Field(default_factory=BillingPolicyBody)


def session_token(request: Request, authorization: Optional[str]) -> str:
    if authorization and authorization.lower().startswith("bearer "):
        return authorization.split(" ", 1)[1].strip()
    return request.cookies.get("jurix_session", "")


def current_user(request: Request, authorization: Optional[str] = Header(default=None)) -> Dict[str, Any]:
    token = session_token(request, authorization)
    if not token:
        raise HTTPException(status_code=401, detail="Sign in to access this workspace")
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
    except PermissionError as exc:
        raise HTTPException(status_code=403, detail=str(exc)) from exc
    except ProviderUnavailable as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    except Exception as exc:
        reference = secrets.token_hex(6)
        logger.error("Jurix operation failed [%s] (%s)", reference, type(exc).__name__)
        raise HTTPException(status_code=502, detail=f"The operation could not be completed. Try again or contact your administrator with reference {reference}.") from exc



@app.get("/api/health")
def health() -> Dict[str, Any]:
    return {"status": "ok", "product": "jurix"}


@app.get("/api/readiness")
def readiness(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    connected = bool(svc.dbx_status().get("ok"))
    return {"status": "ok" if connected else "unavailable", "dbx": {
        "ok": connected, "detail": "" if connected else "Document storage is unavailable. Contact your administrator.",
    }}


@app.get("/api/session")
def session_status(request: Request, authorization: Optional[str] = Header(default=None)) -> Dict[str, Any]:
    token = session_token(request, authorization)
    if not token:
        return {"authenticated": False}
    user = svc.store.user_for_token(token)
    if not user:
        return {"authenticated": False}
    return {"authenticated": True, "user": svc.user_view(user["email"])}


@app.get("/api/capabilities")
def capabilities() -> Dict[str, Any]:
    return svc.capabilities()


@app.get("/api/playbook")
def playbook() -> Dict[str, Any]:
    return svc.playbook()


@app.post("/api/register")
def register(body: RegisterBody, response: Response) -> Dict[str, Any]:
    result = _call(svc.register, body.email, body.password, body.firm_name, body.invite_token)
    session_cookies(response, result["token"])
    return {"user": result["user"]}


@app.post("/api/login")
def login(body: LoginBody, response: Response) -> Dict[str, Any]:
    try:
        result = _call(svc.login, body.email, body.password, body.code)
        session_cookies(response, result["token"])
        return {"user": result["user"]}
    except HTTPException as exc:
        if exc.status_code == 400:
            raise HTTPException(status_code=401, detail=exc.detail) from exc
        raise


@app.post("/api/logout")
def logout(request: Request, response: Response, authorization: Optional[str] = Header(default=None)) -> Dict[str, Any]:
    token = session_token(request, authorization)
    if token:
        _call(svc.store.revoke_token, token)
    clear_cookies(response)
    return {"authenticated": False}


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
    return _call(svc.ingest_version, user["email"], client_id, body.doc_name, body.text, body.supersedes)


@app.post("/api/matters/{client_id}/sample")
def ingest_sample(client_id: str, body: SampleBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.ingest_sample, user["email"], client_id, body.kind)


@app.post("/api/matters/{client_id}/audit")
def audit(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.audit, user["email"], client_id)


@app.post("/api/matters/{client_id}/search")
def search(client_id: str, body: SearchBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.search, user["email"], client_id, body.query)


@app.get("/api/matters/{client_id}/playbook")
def matter_playbook(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.matter_playbook, user["email"], client_id)


@app.put("/api/matters/{client_id}/playbook")
def set_playbook(client_id: str, body: PolicyBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.set_playbook, user["email"], client_id, body.model_dump())


@app.patch("/api/matters/{client_id}/findings/{finding_id}/review")
def review_finding(client_id: str, finding_id: str, body: ReviewBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.review_finding, user["email"], client_id, finding_id, body.status, body.note, body.suggestion_decision)


@app.get("/api/firm/playbooks")
def firm_playbooks(user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.firm_playbooks, user["email"])


@app.post("/api/firm/playbooks")
def save_firm_playbook(body: PolicyBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.save_firm_playbook, user["email"], body.model_dump())


@app.post("/api/matters/{client_id}/export-proposals")
def export_proposals(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.export_proposals, user["email"], client_id)


@app.get("/api/matters/{client_id}/billing")
def billing_history(client_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.billing_history, user["email"], client_id)


@app.post("/api/matters/{client_id}/billing/audit")
def audit_billing(client_id: str, body: BillingBody, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.audit_billing, user["email"], client_id, body.csv_text, body.policy.model_dump())


@app.post("/api/matters/{client_id}/billing/{run_id}/export")
def export_billing(client_id: str, run_id: str, user: Dict[str, Any] = Depends(current_user)) -> Dict[str, Any]:
    return _call(svc.export_billing, user["email"], client_id, run_id)


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
    client_name = re.sub(r"[^A-Za-z0-9._-]+", "-", row["client_name"]).strip(".-")[:80] or "matter"
    kind = re.sub(r"[^A-Za-z0-9_-]", "", row["kind"]) or "memorandum"
    filename = f"jurix-{client_name}-{kind}.md"
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


class InvitationBody(BaseModel):
    email: str = Field(min_length=3, max_length=254)
    role: Literal["admin", "reviewer", "viewer"] = "reviewer"


class InvitationAcceptBody(BaseModel):
    token: str = Field(min_length=1, max_length=200)


class MembershipBody(BaseModel):
    role: Optional[Literal["admin", "reviewer", "viewer"]] = None
    active: Optional[bool] = Field(default=None, strict=True)


class ProcessingBody(BaseModel):
    enabled: bool = Field(strict=True)


class CommitBody(IngestBody):
    pass


class GlossaryEntry(BaseModel):
    source: str = Field(min_length=1, max_length=200)
    target: str = Field(min_length=1, max_length=200)


class TranslationBody(BaseModel):
    target_language: str = Field(min_length=1, max_length=80)
    glossary: List[GlossaryEntry] = Field(default_factory=list, max_length=100)


class WatchBody(BaseModel):
    name: str = Field(min_length=1, max_length=200)
    source_url: str = Field(min_length=1, max_length=2000)
    jurisdiction: str = Field(min_length=1, max_length=200)
    effective_date: Optional[str] = Field(default=None, max_length=10)
    keywords: List[str] = Field(min_length=1, max_length=30)


class AlertReviewBody(BaseModel):
    status: Literal["open", "reviewed", "dismissed"]
    note: str = Field(default="", max_length=10000)


@app.get("/api/firm/members")
def firm_members(user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.store.list_members, user["email"])


@app.patch("/api/firm/members/{email}")
def update_member(email: str, body: MembershipBody, user: Dict[str, Any] = Depends(current_user)):
    _call(svc.authorize, user["email"], "admin")
    members = _call(svc.store.list_members, user["email"])["members"]
    member = next((row for row in members if row["email"] == email.strip().lower()), None)
    if not member:
        raise HTTPException(status_code=404, detail="Firm member not found")
    if body.role is None and body.active is None:
        raise HTTPException(status_code=400, detail="Provide a role or membership state")
    return _call(svc.store.set_role, user["email"], email,
                 body.role if body.role is not None else member["role"],
                 body.active if body.active is not None else member["active"])


@app.post("/api/firm/invitations")
def invite_member(body: InvitationBody, user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.store.invite_member, user["email"], body.email, body.role)


@app.post("/api/firm/invitations/accept")
def accept_invitation(body: InvitationAcceptBody, response: Response, user: Dict[str, Any] = Depends(current_user)):
    result = _call(svc.store.accept_invite, user["email"], body.token)
    clear_cookies(response)
    return result


@app.put("/api/matters/{client_id}/processing")
def processing(client_id: str, body: ProcessingBody, user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.set_processing, user["email"], client_id, body.enabled)


def upload_bytes(file: UploadFile) -> bytes:
    data = file.file.read(20 * 1024 * 1024 + 1)
    if len(data) > 20 * 1024 * 1024:
        raise HTTPException(status_code=413, detail="Files must be no larger than 20 MB")
    return data


@app.post("/api/matters/{client_id}/documents/upload")
def upload_document(client_id: str, file: UploadFile = File(...), use_ocr: bool = Form(False), user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.upload_document, user["email"], client_id, file.filename or "document", upload_bytes(file), use_ocr)


@app.get("/api/matters/{client_id}/documents/extractions/{extraction_id}")
def extraction(client_id: str, extraction_id: str, user: Dict[str, Any] = Depends(current_user)):
    row = _call(svc.extraction, user["email"], client_id, extraction_id)
    return {key: value for key, value in row.items() if key != "blob_id"}


@app.post("/api/matters/{client_id}/documents/extractions/{extraction_id}/commit")
def commit_extraction(client_id: str, extraction_id: str, body: CommitBody, user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.commit_extraction, user["email"], client_id, extraction_id, body.text, body.doc_name, body.supersedes)


def attachment(data: bytes, name: str, media_type: str) -> Response:
    safe_name = re.sub(r"[^A-Za-z0-9._-]+", "-", name).strip(".-")[:160] or "jurix-document"
    return Response(data, media_type=media_type, headers={"Content-Disposition": f'attachment; filename="{safe_name}"'})


@app.get("/api/matters/{client_id}/documents/extractions/{extraction_id}/original")
def original_document(client_id: str, extraction_id: str, user: Dict[str, Any] = Depends(current_user)):
    row = _call(svc.original, user["email"], client_id, extraction_id)
    return attachment(row["data"], row["filename"], row["content_type"])


@app.post("/api/matters/{client_id}/documents/{document_id}/translate")
def translate_document(client_id: str, document_id: str, body: TranslationBody, user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.translate_document, user["email"], client_id, document_id, body.target_language, [row.model_dump() for row in body.glossary])


@app.get("/api/matters/{client_id}/documents/{document_id}/compare")
def compare_documents(client_id: str, document_id: str, other_id: str, user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.compare_documents, user["email"], client_id, document_id, other_id)


@app.post("/api/matters/{client_id}/documents/{document_id}/redline")
def word_redline(client_id: str, document_id: str, user: Dict[str, Any] = Depends(current_user)):
    data = _call(svc.word_redline, user["email"], client_id, document_id)
    return attachment(data, "jurix-redline.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")


@app.post("/api/matters/{client_id}/dictation")
def dictation(client_id: str, file: UploadFile = File(...), lexicon: str = Form("", max_length=4000), language: str = Form("", max_length=80), user: Dict[str, Any] = Depends(current_user)):
    return _call(svc.dictation, user["email"], client_id, file.filename or "review-note.webm", upload_bytes(file), lexicon, language)


@app.get("/api/regulations")
def regulations(user: Dict[str, Any] = Depends(current_user)):
    return _call(lambda: svc.regulatory_watches().list(user["email"]))


@app.post("/api/regulations")
def create_watch(body: WatchBody, user: Dict[str, Any] = Depends(current_user)):
    _call(svc.authorize, user["email"], "admin", "reviewer")
    return _call(lambda: svc.regulatory_watches().create(user["email"], body.model_dump()))


@app.post("/api/regulations/{watch_id}/check")
def check_watch(watch_id: str, user: Dict[str, Any] = Depends(current_user)):
    _call(svc.authorize, user["email"], "admin", "reviewer")
    return _call(lambda: svc.regulatory_watches().check(user["email"], watch_id))


@app.patch("/api/regulations/alerts/{alert_id}")
def review_alert(alert_id: str, body: AlertReviewBody, user: Dict[str, Any] = Depends(current_user)):
    _call(svc.authorize, user["email"], "admin", "reviewer")
    return _call(lambda: svc.regulatory_watches().review(user["email"], alert_id, body.status, body.note))


# Unknown API routes must return JSON, never the application HTML.
@app.api_route("/api/{path:path}", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])
def missing_api(path: str):
    raise HTTPException(status_code=404, detail="API route not found")


# Mount static files at the root level (fallback after all API routes)
if STATIC.is_dir():
    app.mount("/", StaticFiles(directory=str(STATIC), html=True), name="static")
