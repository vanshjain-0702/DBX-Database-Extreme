"""Same-origin cookie protection, bounded bodies and conservative HTTP headers."""
from __future__ import annotations

import hashlib
import hmac
import os
import secrets
import time
from collections import deque
from urllib.parse import urlsplit

from starlette.datastructures import Headers
from starlette.requests import Request
from starlette.responses import JSONResponse

PRODUCTION = os.getenv("JURIX_ENV", "development").lower() == "production"
PUBLIC_ORIGIN = os.getenv("JURIX_PUBLIC_ORIGIN", "").rstrip("/")
if PRODUCTION and (urlsplit(PUBLIC_ORIGIN).scheme != "https" or not urlsplit(PUBLIC_ORIGIN).hostname):
    raise ValueError("Production requires an HTTPS JURIX_PUBLIC_ORIGIN")


def csrf_for(token):
    return hashlib.sha256(("jurix-csrf-v1:" + token).encode()).hexdigest()


def session_cookies(response, token, max_age=43200):
    response.set_cookie("jurix_session", token, max_age=max_age, httponly=True, secure=PRODUCTION, samesite="strict", path="/")
    response.set_cookie("jurix_csrf", csrf_for(token), max_age=max_age, httponly=False, secure=PRODUCTION, samesite="strict", path="/")


def clear_cookies(response):
    response.delete_cookie("jurix_session", path="/", secure=PRODUCTION, httponly=True, samesite="strict")
    response.delete_cookie("jurix_csrf", path="/", secure=PRODUCTION, samesite="strict")


class SecurityMiddleware:
    def __init__(self, app):
        self.app = app
        self.attempts = {}
        self.processing = 0

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            return await self.app(scope, receive, send)
        request, headers = Request(scope), Headers(scope=scope)
        path, method = scope.get("path", ""), scope["method"]
        origin = headers.get("origin", "")
        expected = PUBLIC_ORIGIN or str(request.base_url).rstrip("/")
        async def reject(status, message):
            return await JSONResponse({"detail": message}, status_code=status, headers={"Cache-Control": "no-store"})(scope, receive, send)
        if method not in {"GET", "HEAD", "OPTIONS"} and path.startswith("/api/"):
            if origin and origin != expected:
                return await reject(403, "Request origin is not permitted")
            cookie = request.cookies.get("jurix_session", "")
            bearer = headers.get("authorization", "").lower().startswith("bearer ")
            if cookie and not bearer and path not in {"/api/login", "/api/register"}:
                supplied = headers.get("x-csrf-token", "")
                if not supplied or not hmac.compare_digest(supplied, csrf_for(cookie)):
                    return await reject(403, "Session verification failed; reload the page")
        if path in {"/api/login", "/api/register"} and method == "POST":
            now = time.monotonic()
            peer = str((scope.get("client") or ("unknown",))[0])
            if len(self.attempts) > 5000:
                self.attempts = {k: v for k, v in self.attempts.items() if v and v[-1] > now - 300}
            attempts = self.attempts.setdefault(peer, deque())
            while attempts and attempts[0] < now - 300:
                attempts.popleft()
            if len(attempts) >= 15:
                return await reject(429, "Too many sign-in attempts; try again in five minutes")
            attempts.append(now)
        max_body = 21 * 1024 * 1024 if path.endswith(("/upload", "/dictation")) else 2 * 1024 * 1024
        length = headers.get("content-length", "0")
        if not length.isdigit() or int(length) > max_body:
            return await reject(413, "Request exceeds the upload limit")
        # Read in bounded increments, including requests with no Content-Length.
        chunks, size = [], 0
        if method not in {"GET", "HEAD", "OPTIONS"}:
            while True:
                message = await receive()
                if message["type"] == "http.disconnect":
                    return
                chunk = message.get("body", b"")
                size += len(chunk)
                if size > max_body:
                    return await reject(413, "Request exceeds the upload limit")
                chunks.append(chunk)
                if not message.get("more_body", False):
                    break
            body = b"".join(chunks)
            replayed = False
            async def replay():
                nonlocal replayed
                if not replayed:
                    replayed = True
                    return {"type": "http.request", "body": body, "more_body": False}
                return await receive()
        else:
            replay = receive
        async def protected_send(message):
            if message["type"] == "http.response.start":
                response_headers = list(message.get("headers", []))
                response_headers.extend([(b"x-content-type-options", b"nosniff"), (b"referrer-policy", b"same-origin"), (b"x-frame-options", b"DENY"),
                    (b"permissions-policy", b"camera=(), geolocation=(), microphone=(self)"),
                    (b"content-security-policy", b"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; media-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")])
                if path.startswith("/api/") or path in {"/", "/index.html"}:
                    response_headers.append((b"cache-control", b"no-store"))
                if PRODUCTION:
                    response_headers.append((b"strict-transport-security", b"max-age=31536000"))
                message["headers"] = response_headers
            await send(message)
        return await self.app(scope, replay, protected_send)
