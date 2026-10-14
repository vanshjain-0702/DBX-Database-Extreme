"""Curated source-change watches, producing review tasks rather than legal verdicts."""
from __future__ import annotations

import difflib
import hashlib
import http.client
import ipaddress
import re
import secrets
import socket
import ssl
import time
from datetime import date
from html.parser import HTMLParser
from urllib.parse import urlsplit, urljoin

# Add authorities in code after reviewing their ownership and publication format.
AUTHORITY_HOSTS = {"eur-lex.europa.eu", "www.legislation.gov.uk", "legislation.gov.uk", "www.indiacode.nic.in", "indiacode.nic.in", "leginfo.legislature.ca.gov", "www.meity.gov.in", "meity.gov.in", "www.edpb.europa.eu", "edpb.europa.eu"}


def validate_url(url):
    p = urlsplit(url)
    if len(url) > 2000 or p.scheme != "https" or p.hostname not in AUTHORITY_HOSTS or p.username or p.password or p.port not in {None, 443} or any(ord(c) < 33 for c in url):
        raise ValueError("Use a direct HTTPS publication URL from a supported regulatory authority")
    return p


class Text(HTMLParser):
    def __init__(self):
        super().__init__()
        self.parts, self.skip = [], 0
    def handle_starttag(self, tag, attrs):
        if tag in {"script", "style", "nav", "header", "footer"}:
            self.skip += 1
    def handle_endtag(self, tag):
        if tag in {"script", "style", "nav", "header", "footer"}:
            self.skip = max(0, self.skip - 1)
    def handle_data(self, data):
        if not self.skip and data.strip():
            self.parts.append(data.strip())


def fetch_source(url):
    for _ in range(4):
        p = validate_url(url)
        addresses = socket.getaddrinfo(p.hostname, 443, type=socket.SOCK_STREAM)
        if not addresses or any(not ipaddress.ip_address(a[4][0]).is_global for a in addresses):
            raise ValueError("Regulatory source did not resolve to a public network address")
        # Pin the verified resolution while retaining TLS host verification/SNI.
        class Connection(http.client.HTTPSConnection):
            def connect(self):
                self.sock = self._context.wrap_socket(socket.create_connection(addresses[0][4][:2], timeout=15), server_hostname=self.host)
        conn = Connection(p.hostname, timeout=15, context=ssl.create_default_context())
        try:
            conn.request("GET", p.path + ("?" + p.query if p.query else ""), headers={"User-Agent": "JurixRegulatoryWatch/1.0", "Accept": "text/html,text/plain", "Accept-Encoding": "identity"})
            response = conn.getresponse()
            if response.status in {301, 302, 303, 307, 308}:
                url = urljoin(url, response.getheader("Location", ""))
                continue
            if response.status != 200:
                raise ValueError("Regulatory publication is unavailable")
            content_type = response.getheader("Content-Type", "").lower()
            if not any(t in content_type for t in ("text/html", "text/plain", "application/xhtml+xml")):
                raise ValueError("Source watches support HTML and plain text publications; choose the authority's text page")
            if response.getheader("Content-Encoding", "identity").lower() not in {"", "identity"}:
                raise ValueError("Source compression is unsupported")
            raw = response.read(2 * 1024 * 1024 + 1)
            if len(raw) > 2 * 1024 * 1024:
                raise ValueError("Publication exceeds the source size limit")
            charset = re.search(r"charset=([\w-]+)", content_type)
            text = raw.decode(charset[1] if charset else "utf-8", errors="replace")
            if "html" in content_type:
                parser = Text()
                parser.feed(text)
                text = "\n".join(parser.parts)
            text = "\n".join(re.sub(r"\s+", " ", line).strip() for line in text.splitlines() if line.strip())
            if not text or len(text) > 500_000:
                raise ValueError("Publication has no usable text or exceeds the review limit")
            return {"text": text, "sha256": hashlib.sha256(text.encode()).hexdigest(), "url": url, "fetched_at": time.time()}
        finally:
            conn.close()
    raise ValueError("Regulatory publication redirects too many times")


class RegulatoryWatches:
    def __init__(self, database, store, records, fetch=fetch_source):
        self.db, self.store, self.records, self.fetch = database, store, records, fetch

    def list(self, email):
        owner = self.store.workspace_owner(email)
        watches = self.db.list_objects(owner, "watches")
        alerts = self.db.list_objects(owner, "alerts")
        return {"watches": [{k: v for k, v in w.items() if k != "source_text"} for w in watches], "alerts": alerts,
                "supported_hosts": sorted(AUTHORITY_HOSTS)}

    def create(self, email, body):
        self.store.require_role(email, "admin", "reviewer")
        owner = self.store.workspace_owner(email)
        validate_url(body["source_url"])
        if len(self.db.list_objects(owner, "watches")) >= 50:
            raise ValueError("Firm watch limit reached (50 sources)")
        if body.get("effective_date") is not None:
            try:
                date.fromisoformat(body["effective_date"])
            except (ValueError, TypeError) as exc:
                raise ValueError("Use an ISO effective date (YYYY-MM-DD), or leave it unknown") from exc
        if not body["keywords"] or any(not k.strip() or len(k) > 100 for k in body["keywords"]):
            raise ValueError("Provide keywords for matching relevant document passages")
        row = {**body, "id": secrets.token_hex(12), "versions": [], "checked_at": None, "changed_at": None, "error": "", "author": email, "created_at": time.time()}
        self.db.put_object(owner, "watches", row["id"], row)
        return {k: v for k, v in row.items() if k != "source_text"}

    def check(self, email, watch_id):
        self.store.require_role(email, "admin", "reviewer")
        owner = self.store.workspace_owner(email)
        row = self.db.get_object(owner, "watches", watch_id)
        if not row:
            raise KeyError("Regulatory watch not found")
        try:
            source = self.fetch(row["source_url"])
        except Exception:
            row["checked_at"], row["error"] = time.time(), "Publication could not be checked. The last successful baseline remains unchanged."
            self.db.put_object(owner, "watches", row["id"], row)
            return {k: v for k, v in row.items() if k != "source_text"}
        old_hash = row.get("source_sha256")
        changed = old_hash and old_hash != source["sha256"]
        if old_hash != source["sha256"]:
            row["versions"].append({"sha256": source["sha256"], "fetched_at": source["fetched_at"], "url": source["url"]})
            row["versions"] = row["versions"][-100:]
        if changed:
            row["changed_at"] = time.time()
            diff = [line[2:] for line in difflib.ndiff(row.get("source_text", "").splitlines(), source["text"].splitlines()) if line.startswith("+ ") or line.startswith("- ")]
            excerpt = "\n".join(diff)[:2000]
            for client in self.store.list_clients(email):
                if not self.records.run(client["id"]):
                    continue
                docs = self.records.documents(client["id"])
                matches = [d for d in docs if any(k.lower() in d["text"].lower() for k in row["keywords"])]
                if not matches:
                    continue
                aid = hashlib.sha256(f"{watch_id}:{source['sha256']}:{client['id']}".encode()).hexdigest()[:24]
                if not self.db.get_object(owner, "alerts", aid):
                    self.db.put_object(owner, "alerts", aid, {"id": aid, "watch_id": watch_id, "matter_id": client["id"], "matter_name": client["name"], "title": row["name"], "source_url": source["url"], "jurisdiction": row["jurisdiction"], "effective_date": row["effective_date"], "source_sha256": source["sha256"], "previous_sha256": old_hash, "excerpt": excerpt, "document_ids": [d["id"] for d in matches], "status": "open", "note": "", "created_at": time.time(), "basis": "Source changed; firm keywords match audited documents. Counsel must assess applicability and effective dates."})
        row.update(source_sha256=source["sha256"], source_text=source["text"], checked_at=source["fetched_at"], error="")
        self.db.put_object(owner, "watches", watch_id, row)
        return {k: v for k, v in row.items() if k != "source_text"}

    def review(self, email, alert_id, status, note):
        self.store.require_role(email, "admin", "reviewer")
        owner = self.store.workspace_owner(email)
        row = self.db.get_object(owner, "alerts", alert_id)
        if not row:
            raise KeyError("Regulatory alert not found")
        if status not in {"open", "reviewed", "dismissed"} or (status != "open" and not note.strip()):
            raise ValueError("Record a rationale when reviewing or dismissing an alert")
        row.update(status=status, note=note.strip(), reviewed_by=email, reviewed_at=time.time())
        self.db.put_object(owner, "alerts", alert_id, row)
        return row
