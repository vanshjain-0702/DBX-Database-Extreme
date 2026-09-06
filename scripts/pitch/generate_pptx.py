#!/usr/bin/env python3
"""Generate website/assets/dbx-incubation-pitch.pptx (16:9, ice-on-navy)."""
from __future__ import annotations

from pathlib import Path

from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.enum.shapes import MSO_SHAPE
from pptx.enum.text import PP_ALIGN
from pptx.util import Inches, Pt

BG = RGBColor(0x04, 0x09, 0x10)
ICE = RGBColor(0x9E, 0xE7, 0xFF)
TEXT = RGBColor(0xF1, 0xF6, 0xFB)
MUTED = RGBColor(0x8E, 0xA8, 0xBD)
EMBER = RGBColor(0xC4, 0xF0, 0xFF)
SAGE = RGBColor(0x5E, 0xE4, 0xB0)
WARN = RGBColor(0xFF, 0xC4, 0x6B)

OUT = Path(__file__).resolve().parents[2] / "website" / "assets" / "dbx-incubation-pitch.pptx"
W, H = Inches(13.333), Inches(7.5)


def _fill(shape, rgb):
    shape.fill.solid()
    shape.fill.fore_color.rgb = rgb
    shape.line.fill.background()


def _set_run(run, size, color, bold=False):
    run.font.size = Pt(size)
    run.font.color.rgb = color
    run.font.bold = bold
    run.font.name = "Calibri"


def add_bg(slide):
    shape = slide.shapes.add_shape(MSO_SHAPE.RECTANGLE, 0, 0, W, H)
    _fill(shape, BG)
    spTree = slide.shapes._spTree
    sp = shape._element
    spTree.remove(sp)
    spTree.insert(2, sp)


def footer(slide, n, total=20):
    box = slide.shapes.add_textbox(Inches(0.55), Inches(7.12), Inches(10.5), Inches(0.28))
    p = box.text_frame.paragraphs[0]
    run = p.add_run()
    run.text = f"DBX  ·  Incubation deck  ·  Confidential  ·  v1.1.0  ·  {n}/{total}"
    _set_run(run, 10, MUTED)
    box2 = slide.shapes.add_textbox(Inches(11.6), Inches(7.12), Inches(1.1), Inches(0.28))
    p2 = box2.text_frame.paragraphs[0]
    p2.alignment = PP_ALIGN.RIGHT
    run2 = p2.add_run()
    run2.text = "BSL 1.1"
    _set_run(run2, 10, ICE)


def kicker(slide, text, top=0.38):
    box = slide.shapes.add_textbox(Inches(0.7), Inches(top), Inches(12), Inches(0.32))
    p = box.text_frame.paragraphs[0]
    run = p.add_run()
    run.text = text.upper()
    _set_run(run, 12, ICE, True)


def title(slide, text, top=0.7, size=32):
    box = slide.shapes.add_textbox(Inches(0.7), Inches(top), Inches(12), Inches(1.15))
    tf = box.text_frame
    tf.word_wrap = True
    p = tf.paragraphs[0]
    run = p.add_run()
    run.text = text
    _set_run(run, size, TEXT, True)


def body(slide, text, top=2.0, height=1.2, size=16):
    box = slide.shapes.add_textbox(Inches(0.7), Inches(top), Inches(12), Inches(height))
    tf = box.text_frame
    tf.word_wrap = True
    p = tf.paragraphs[0]
    run = p.add_run()
    run.text = text
    _set_run(run, size, MUTED)


def bullets(slide, items, top, width=12.0, left=0.7, size=15):
    box = slide.shapes.add_textbox(Inches(left), Inches(top), Inches(width), Inches(5.8 - top))
    tf = box.text_frame
    tf.word_wrap = True
    for i, item in enumerate(items):
        p = tf.paragraphs[0] if i == 0 else tf.add_paragraph()
        p.level = 0
        p.space_after = Pt(8)
        run = p.add_run()
        run.text = "•  " + item
        _set_run(run, size, MUTED)


def card(slide, left, top, w, h, heading, text, accent=ICE):
    shape = slide.shapes.add_shape(MSO_SHAPE.ROUNDED_RECTANGLE, Inches(left), Inches(top), Inches(w), Inches(h))
    shape.adjustments[0] = 0.08
    shape.fill.solid()
    shape.fill.fore_color.rgb = RGBColor(0x0D, 0x1C, 0x2B)
    shape.line.color.rgb = accent
    box = slide.shapes.add_textbox(Inches(left + 0.18), Inches(top + 0.16), Inches(w - 0.36), Inches(h - 0.28))
    tf = box.text_frame
    tf.word_wrap = True
    p = tf.paragraphs[0]
    run = p.add_run()
    run.text = heading
    _set_run(run, 15, TEXT, True)
    p2 = tf.add_paragraph()
    p2.space_before = Pt(6)
    run2 = p2.add_run()
    run2.text = text
    _set_run(run2, 13, MUTED)


def new_slide(prs):
    slide = prs.slides.add_slide(prs.slide_layouts[6])
    add_bg(slide)
    return slide


def main():
    prs = Presentation()
    prs.slide_width = W
    prs.slide_height = H

    s = new_slide(prs)
    kicker(s, "Incubation briefing  ·  Confidential  ·  September 2026")
    title(s, "The tenant is the database.", 1.6, 44)
    body(s, "DBX is the per-tenant memory engine for AI products: one isolated store per customer, holding working state and vector memory. Not a shared cluster with a prefix.", 3.0, 1.4, 20)
    body(s, "v1.1.0   ·   100 tenants/node   ·   BSL 1.1   ·   Founder: Vansh Jain, DTU", 5.8, 0.4, 14)
    footer(s, 1)

    s = new_slide(prs)
    kicker(s, "Thesis")
    title(s, "One isolated engine per customer.")
    body(s, "Every customer of an AI product needs private session state and private semantic recall. DBX makes the tenant the unit of the database — directory, WAL, HNSW index, process, and key.")
    card(s, 0.7, 3.3, 3.8, 2.5, "01  Working state", "KV with TTL: session, scratchpad, rate counters. Same connection as recall.")
    card(s, 4.75, 3.3, 3.8, 2.5, "02  Vector memory", "SQ8 HNSW, mmap'd, sized for per-tenant working sets — not a billion-vector corpus.", SAGE)
    card(s, 8.8, 3.3, 3.8, 2.5, "03  Sealed domain", "Linux strict: process, Landlock, envelope encryption, Unix SO_PEERCRED.", RGBColor(0xC4, 0xB5, 0xFD))
    footer(s, 2)

    s = new_slide(prs)
    kicker(s, "Why now")
    title(s, "Agents shipped. Isolation did not.")
    bullets(s, [
        "Agent platforms now give every end-customer a memory, not a chat transcript.",
        "Indirect prompt injection can leak neighbor context into the reasoning chain.",
        "Regulated buyers need delete-my-data as an API (GDPR / DPDP-shaped off-boarding).",
        "Redis plus a vector DB doubles on-call, dual-writes, and bills.",
        "Firecracker per user is a fleet. Prefix filters are a hope. DBX is the third path (~14–17 MiB RSS per idle worker).",
    ], 2.05)
    footer(s, 3)

    s = new_slide(prs)
    kicker(s, "Problem")
    title(s, "Shared clusters make tenancy your job.")
    bullets(s, [
        "Tenancy is a key prefix — one forgotten tenant_id is a cross-customer leak.",
        "Cache + vector DB — dual writes, drift after crashes, two on-call surfaces.",
        "Backup is cluster-wide — cannot restore or export a single customer.",
        "Deletion is a scan — “delete my data” becomes an engineering project.",
        "Shared memory pool — a noisy tenant evicts a quiet tenant’s working set.",
        "Cost is guessed — no meter for what one customer actually consumes.",
    ], 2.05)
    footer(s, 4)

    s = new_slide(prs)
    kicker(s, "Solution")
    title(s, "Isolation is a directory, a WAL, and a process.")
    body(s, "HTTP :8000 is tenant lifecycle, JWT, and the dashboard. RESP :6380 is AUTH tenant:key — redis-py, ioredis, and go-redis already speak it. GET on harbor cannot open acme’s files or sockets. Purge shreds the wrap file.")
    bullets(s, [
        "On-ramp, not a USP: existing RESP clients work. Not a drop-in Redis replacement.",
        "Linux production (DBX_ISOLATION_MODE=strict) seals each tenant as dbx-server + Landlock + DEK.",
    ], 3.4)
    footer(s, 5)

    s = new_slide(prs)
    kicker(s, "USPs 01–03")
    title(s, "Five claims. Each is true in the binary.")
    card(s, 0.7, 2.15, 3.8, 4.3, "USP 01  Tenant is first-class", "POST /api/provision returns a live engine: directory, WAL, HNSW, snapshots, process. Backup, restore, hibernate, and purge operate on exactly one customer.")
    card(s, 4.75, 2.15, 3.8, 4.3, "USP 02  State + memory", "Session KV and semantic recall share a connection, a directory, and a backup zip. Limit: periodic .rdb serializes KV; vectors live in mmap + WAL + seals.", SAGE)
    card(s, 8.8, 2.15, 3.8, 4.3, "USP 03  Cost of active tenants", "SQ8 rows are ~4× smaller than float32. Idle tenants live in page cache. The metric is cost per active tenant.", WARN)
    footer(s, 6)

    s = new_slide(prs)
    kicker(s, "USPs 04–05")
    title(s, "Self-hosted. Kernel-sealed on Linux.")
    card(s, 0.7, 2.15, 6.05, 2.4, "USP 04  One binary", "Embeddings never leave the customer network. Dashboard, console, explorer, and vector playground compile into the orchestrator.")
    card(s, 6.95, 2.15, 5.65, 2.4, "USP 05  Isolation Kernel", "Process + Landlock + AES-256-GCM (WAL, checkpoints, ids, HNSW) + Unix sockets gated by orchestrator PID.")
    body(s, "~15 MiB idle worker RSS   ·   O(1) DEK shred   ·   SQ8 rows stay mmap plaintext (use LUKS)   ·   Not the strongest sandbox — Firecracker is stronger.", 4.85, 1.2, 15)
    footer(s, 7)

    s = new_slide(prs)
    kicker(s, "Product")
    title(s, "The lifecycle is the product.")
    bullets(s, [
        "Provision — isolated engine in milliseconds; optional 0–2 async WAL replicas (primary acks locally).",
        "Keys — reader / writer / tenant-admin. Reader cannot SET or VADD. Revoke is immediate.",
        "Query — public data plane RESP :6380. Python SDK: remember / recall / forget.",
        "Backup — one-customer zip, SHA-256 manifest, restore with rollback. Hibernate keeps the directory.",
        "Shred — purge=true zeros the wrapped DEK, then deletes the directory. Blast radius: one tenant.",
        "Certified v1 surface: durable strings + vectors. Raft, cluster, and non-string mutations fail closed.",
    ], 2.05)
    footer(s, 8)

    s = new_slide(prs)
    kicker(s, "Who buys")
    title(s, "Teams whose customers each need memory.")
    card(s, 0.7, 2.15, 6.05, 4.2, "Ideal customer", "AI agent platforms · Vertical AI SaaS (legal, medical, support, finance) · On-prem / regulated one-binary deployments · Teams hand-rolling tenancy over a cache and a vector store.")
    card(s, 6.95, 2.15, 5.65, 4.2, "We say no quickly", "One tenant, one workload. Billion-vector ANN. SQL / joins / system of record. Buying DBX as a Pinecone-style fully managed service. We sit in front of the ledger and underneath the agent.", WARN)
    footer(s, 9)

    s = new_slide(prs)
    kicker(s, "White space")
    title(s, "Giants optimize the cluster. We sell a different unit.")
    bullets(s, [
        "Peak KV throughput → Redis / Dragonfly. Their design target.",
        "Billion-vector ANN + sharding → Qdrant / Milvus. Wrong index size for our thesis.",
        "Fully managed vectors → Pinecone. We are software you run, deliberately.",
        "Joins and SQL → Postgres + pgvector. DBX is memory, not the ledger.",
        "Per-tenant isolation is not a row on those charts. We do not take the Redis fight.",
    ], 2.05)
    footer(s, 10)

    s = new_slide(prs)
    kicker(s, "Status  ·  v1.1.0")
    title(s, "Shipped. Certified. Open.")
    bullets(s, [
        "GO for single-node production: 100 tenants/node, 100k vectors/tenant.",
        "Isolation Kernel, Unix data plane, usage meter, scoped keys, Python SDK, dashboard.",
        "Public site, 5m18s walkthrough, DEV.to Isolation Kernel article kit.",
        "No ARR, no customer logos, no SOC 2. Not a cluster. hello@dbxdb.io has no MX yet.",
        "BSL 1.1: free inside your SaaS; commercial license if you sell managed DBX.",
    ], 2.05)
    footer(s, 11)

    s = new_slide(prs)
    kicker(s, "Proof  ·  27 Aug 2026  ·  one node  ·  one tenant")
    title(s, "Isolation is not expensive.")
    bullets(s, [
        "SET 186,147 ops/s · GET 284,785 ops/s · pipeline 64 · WAL everysec.",
        "Ingest 7,233 vec/s (100k × 128, 8-way HNSW). Search p50/p95/p99 2.304 / 3.132 / 3.730 ms.",
        "Recall@10 mean 0.920 / p05 0.800 (SQ8 vs float32 brute force).",
        "Strict idle worker ~14–17 MiB RSS. Quiet GET isolated under noisy writers.",
        "Not a competitor ranking. Linux CI re-runs the harness.",
    ], 2.05)
    footer(s, 12)

    s = new_slide(prs)
    kicker(s, "Market")
    title(s, "A category the cluster vendors do not serve.")
    card(s, 0.7, 2.15, 3.8, 4.2, "TAM", "Vector DB ~$3.2B (2025) → ~$8.95B (2030), 27.5% CAGR (MarketsandMarkets). Agent memory infra ~$1.62B → ~$15.2B by 2034 (HTF).")
    card(s, 4.75, 2.15, 3.8, 4.2, "SAM", "Agentic AI in vector DBs ~$0.57B (2026) → ~$1.73B (2031), 24.86% CAGR (Mordor). Self-hosted per-customer isolation is the unnamed slice.", SAGE)
    card(s, 8.8, 2.15, 3.8, 4.2, "SOM · 24 months", "Operational target, not a forecast: 10 vertical-AI design partners; 3 commercial-license conversations. No invented ARR.", WARN)
    footer(s, 13)

    s = new_slide(prs)
    kicker(s, "Business model")
    title(s, "Free in your product. Paid when you sell ours.")
    card(s, 0.7, 2.15, 3.8, 4.2, "Self-host · Free", "BSL 1.1. Production and embed-in-your-SaaS permitted. Apache 2.0 after four years.")
    card(s, 4.75, 2.15, 3.8, 4.2, "Commercial · Talk", "Required if DBX itself is the service sold to third parties. Billable unit: the tenant.", SAGE)
    card(s, 8.8, 2.15, 3.8, 4.2, "Support · Talk", "Named contacts and SLA language. No fake public SKUs. Managed DBX remains a company decision the BSL reserved.")
    footer(s, 14)

    s = new_slide(prs)
    kicker(s, "Go-to-market  ·  18 months")
    title(s, "Land in the binary. Expand on the tenant.")
    bullets(s, [
        "Developer-led: GitHub, RESP clients, 15-minute Python path, Isolation Kernel article.",
        "Vertical design partners: legal, support, on-prem AI SaaS with a residency story.",
        "Trust path: live MX, commercial paper, SOC 2 readiness (not a badge we have).",
        "Product next: Linux density published, engine coverage, promote-without-restart.",
        "Managed DBX only when design partners pull — BSL keeps that our decision.",
    ], 2.05)
    footer(s, 15)

    s = new_slide(prs)
    kicker(s, "The ask")
    title(s, "Incubation to first commercial licenses.")
    body(s, "18-month operating partnership. Scale the currency envelope to the incubator; keep the mix.")
    bullets(s, [
        "45%  Core engineering — density beyond 100/node, replica promote, coverage, optional float32 mode.",
        "20%  Trust & compliance path — domain + MX, commercial license paper, SOC 2 readiness, LUKS ops.",
        "20%  Design partners / GTM — first 10 teams, not a paid-ads plan.",
        "10%  Legal · IP · contracts.",
        "5%   CI · demo hardware · domain.",
        "Gates: 10 design partners · 3 commercial conversations · Linux density published · mailbox live.",
    ], 2.55)
    footer(s, 16)

    s = new_slide(prs)
    kicker(s, "Team")
    title(s, "Founder-built systems software.")
    card(s, 0.7, 2.15, 6.05, 4.2, "Vansh Jain — Founder", "Engineering undergraduate, Delhi Technological University. Designed and shipped the Go 1.25 engine, Isolation Kernel, SQ8 HNSW, orchestrator, dashboard, Python SDK, and public site. github.com/vanshjain-0702")
    card(s, 6.95, 2.15, 5.65, 4.2, "What incubation adds", "Legal templates and IP hygiene. Design-partner introductions. Domain, mailbox, security-review process. Not a rewrite of the kernel — the product is the proof of work.", SAGE)
    footer(s, 17)

    s = new_slide(prs)
    kicker(s, "Risks we already name")
    title(s, "Diligence, not a badge wall.")
    bullets(s, [
        "SQ8 .vec rows stay mmap plaintext so idle tenants stay cheap — use LUKS/fscrypt.",
        "Landlock governs opens, not connect()/stat(). Sockets: SO_PEERCRED + mode 0600.",
        "No worker network restriction. Data in use is plaintext. cgroup is best-effort in containers.",
        "Certified cap 100 tenants/node. Single-node. Not SOC 2. Not a Redis replacement.",
        "Solo founder concentration. Pre-revenue. Domain MX not live.",
    ], 2.05)
    footer(s, 18)

    s = new_slide(prs)
    kicker(s, "Close")
    title(s, "Give every customer a database, not a prefix.", 1.6, 36)
    body(s, "We are asking incubation to take a shipped Isolation Kernel to the first ten design partners and the first commercial licenses — with the same honesty we use in the code.", 3.2, 1.4, 18)
    body(s, "github.com/vanshjain-0702/DBX-Database-Extreme    ·    github.io/DBX-Database-Extreme/pitch.html    ·    demo.html", 5.6, 0.5, 14)
    footer(s, 19)

    s = new_slide(prs)
    kicker(s, "Appendix  ·  Non-goals")
    title(s, "Coherence is a feature.")
    bullets(s, [
        "Beating Redis on single-instance KV — wrong buyer, unwinnable axis.",
        "Sharded billion-vector ANN — wrong index size for our thesis.",
        "Managed DBX cloud as a default — BSL exists so this stays a company decision.",
        "SQL / joins / cross-entity ACID — we are memory, not the system of record.",
        "OLAP over history — wrong shape entirely.",
        "Sources: docs/positioning.md · docs/isolation.md · LICENSE (BSL 1.1 → Apache 2.0 after four years).",
    ], 2.05)
    footer(s, 20)

    OUT.parent.mkdir(parents=True, exist_ok=True)
    prs.save(OUT)
    print(f"wrote {OUT}")


if __name__ == "__main__":
    main()
