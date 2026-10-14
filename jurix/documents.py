"""Bounded document imports and text-derived Word tracked revisions."""
from __future__ import annotations

import difflib
import hashlib
import io
from pathlib import PurePath
from xml.etree import ElementTree as ET
from zipfile import ZipFile, BadZipFile
from datetime import datetime, timezone

MAX_FILE = 20 * 1024 * 1024
MAX_TEXT = 200_000
MAX_PAGES = 200
W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
ET.register_namespace("w", W)


def validate_file(filename: str, data: bytes):
    name = filename.replace("\\", "/").rsplit("/", 1)[-1]
    if not name or len(name) > 200 or any(ord(c) < 32 for c in name):
        raise ValueError("Use a filename of 1–200 printable characters")
    if not data or len(data) > MAX_FILE:
        raise ValueError("Files must contain data and be no larger than 20 MB")
    return name, PurePath(name).suffix.lower()


def page_texts(pages: list[str]):
    out, offset, texts = [], 0, []
    for number, text in enumerate(pages, 1):
        if number > MAX_PAGES:
            raise ValueError("Document exceeds the 200-page limit")
        if texts:
            offset += 2
        out.append({"number": number, "start": offset, "end": offset + len(text), "text": text})
        texts.append(text)
        offset += len(text)
        if offset > MAX_TEXT:
            raise ValueError("Document exceeds the 200,000-character review limit")
    return "\n\n".join(texts), out


def extract(filename: str, data: bytes, providers, use_ocr=False):
    name, ext = validate_file(filename, data)
    if ext not in {".txt", ".md", ".pdf", ".docx", ".png", ".jpg", ".jpeg", ".tif", ".tiff"}:
        raise ValueError("Supported imports: TXT, Markdown, PDF, DOCX, PNG, JPEG and TIFF")
    method, warnings = "Local text extraction", []
    if ext in {".txt", ".md"}:
        try:
            text, pages = page_texts([data.decode("utf-8-sig")])
        except UnicodeDecodeError as exc:
            raise ValueError("Text imports must use UTF-8 encoding") from exc
    elif ext == ".docx":
        try:
            with ZipFile(io.BytesIO(data)) as archive:
                entries = archive.infolist()
                if len(entries) > 2000 or sum(i.file_size for i in entries) > 40 * 1024 * 1024:
                    raise ValueError("DOCX archive exceeds the decompression limit")
                raw = archive.read("word/document.xml")
                if len(raw) > 8 * 1024 * 1024 or b"<!DOCTYPE" in raw.upper() or b"<!ENTITY" in raw.upper():
                    raise ValueError("Unsupported DOCX XML content")
                root = ET.fromstring(raw)
                if root.find(".//{" + W + "}ins") is not None or root.find(".//{" + W + "}del") is not None:
                    raise ValueError("Resolve tracked changes in Word before importing this document")
                paragraphs = ["".join(node.text or "" for node in p.iter("{" + W + "}t")) for p in root.iter("{" + W + "}p")]
                text, pages = page_texts(["\n".join(paragraphs)])
        except (BadZipFile, KeyError, ET.ParseError) as exc:
            raise ValueError("The file is not a readable DOCX document") from exc
        warnings.append("DOCX layout and page breaks are not preserved in the review text. Compare the original file.")
    elif ext == ".pdf" and not use_ocr:
        if not data.startswith(b"%PDF-"):
            raise ValueError("The uploaded file is not a PDF")
        from pypdf import PdfReader
        try:
            reader = PdfReader(io.BytesIO(data), strict=True)
            if reader.is_encrypted:
                raise ValueError("Remove PDF password protection before importing")
            if len(reader.pages) > MAX_PAGES:
                raise ValueError("PDF exceeds the 200-page limit")
            # Prevent enormous compressed content streams from exhausting RAM.
            for page in reader.pages:
                contents = page.get_contents()
                if contents is not None and len(contents.get_data()) > 4 * 1024 * 1024:
                    raise ValueError("PDF page content exceeds the extraction limit")
            text, pages = page_texts([p.extract_text() or "" for p in reader.pages])
        except ValueError:
            raise
        except Exception as exc:
            raise ValueError("PDF extraction failed; try a smaller, repaired document") from exc
        if not text.strip():
            raise ValueError("No searchable text found. Enable configured OCR to process a scan")
        warnings.append("Verify reading order, tables and page text against the original PDF.")
    else:
        content_type = {".pdf": "application/pdf", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".tif": "image/tiff", ".tiff": "image/tiff"}[ext]
        if ext == ".pdf" and not data.startswith(b"%PDF-"):
            raise ValueError("The uploaded file is not a PDF")
        if ext != ".pdf":
            from PIL import Image
            try:
                with Image.open(io.BytesIO(data)) as im:
                    if im.width * im.height > 40_000_000 or getattr(im, "n_frames", 1) > MAX_PAGES:
                        raise ValueError("Image dimensions or page count exceed the import limit")
                    im.verify()
            except ValueError:
                raise
            except Exception as exc:
                raise ValueError("The uploaded image is invalid") from exc
        result = providers.ocr(data, content_type)
        return {"name": name, "source_sha256": hashlib.sha256(data).hexdigest(), "status": "needs_review", **result}
    if not text.strip() or "\x00" in text:
        raise ValueError("Document contains no usable text or contains binary characters")
    return {"name": name, "text": text, "pages": pages, "warnings": warnings, "source_sha256": hashlib.sha256(data).hexdigest(), "method": method, "status": "needs_review"}


def compare(before: dict, after: dict):
    a, b = before["text"].splitlines(), after["text"].splitlines()
    changes = [{"op": op, "before": "\n".join(a[i:j]), "after": "\n".join(b[k:l])} for op, i, j, k, l in difflib.SequenceMatcher(None, a, b, autojunk=False).get_opcodes() if op != "equal"]
    return {"before": {"id": before["id"], "name": before["name"], "text": before["text"]}, "after": {"id": after["id"], "name": after["name"], "text": after["text"]}, "changes": changes}


def tracked_docx(text: str, items: list[dict], author: str):
    replacements = []
    for item in items:
        anchor = item.get("anchor")
        if not anchor or not item.get("proposed_text") or text[anchor["start"]:anchor["end"]] != item.get("quote"):
            raise ValueError("Accepted proposals need an exact source anchor before Word export")
        replacements.append((anchor["start"], anchor["end"], item["proposed_text"]))
    replacements.sort()
    if not replacements:
        raise ValueError("Accept an anchored clause proposal before exporting Word revisions")
    for left, right in zip(replacements, replacements[1:]):
        if left[1] > right[0]:
            raise ValueError("Accepted proposals overlap. Choose one proposal for each passage")
    root = ET.Element("{" + W + "}document")
    body = ET.SubElement(root, "{" + W + "}body")
    paragraph = ET.SubElement(body, "{" + W + "}p")
    def run(parent, value, deleted=False):
        # Explicit line breaks retain the complete extracted source text.
        lines = value.split("\n")
        for index, line in enumerate(lines):
            r = ET.SubElement(parent, "{" + W + "}r")
            if index:
                ET.SubElement(r, "{" + W + "}br")
            t = ET.SubElement(r, "{" + W + "}" + ("delText" if deleted else "t"), {"{http://www.w3.org/XML/1998/namespace}space": "preserve"})
            t.text = line
    position = 0
    stamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    for index, (start, end, proposed) in enumerate(replacements):
        run(paragraph, text[position:start])
        deleted = ET.SubElement(paragraph, "{" + W + "}del", {"{" + W + "}id": str(2*index), "{" + W + "}author": author, "{" + W + "}date": stamp})
        run(deleted, text[start:end], True)
        inserted = ET.SubElement(paragraph, "{" + W + "}ins", {"{" + W + "}id": str(2*index+1), "{" + W + "}author": author, "{" + W + "}date": stamp})
        run(inserted, proposed)
        position = end
    run(paragraph, text[position:])
    content_types = '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/></Types>'
    rels = '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>'
    settings_rels = '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/></Relationships>'
    stream = io.BytesIO()
    with ZipFile(stream, "w") as archive:
        archive.writestr("[Content_Types].xml", content_types)
        archive.writestr("_rels/.rels", rels)
        archive.writestr("word/_rels/document.xml.rels", settings_rels)
        archive.writestr("word/document.xml", ET.tostring(root, encoding="utf-8", xml_declaration=True))
        archive.writestr("word/settings.xml", f'<w:settings xmlns:w="{W}"><w:trackRevisions/></w:settings>')
    return stream.getvalue()
