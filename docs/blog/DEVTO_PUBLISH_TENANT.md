# How to publish the DBX intro post on DEV.to

Use this file as the checklist while you sit in the [DEV Community editor](https://dev.to/new). Every step lists the **exact value** to type or paste. Do not improvise titles, tags, or links: they are chosen to match v1.1.0 and the [language guide](../positioning.md).

This is the **product / tutorial** post (series `DBX`). The Isolation Kernel engineering post has its own kit: [DEVTO_PUBLISH.md](DEVTO_PUBLISH.md).

Official editor reference: [dev.to/p/editor_guide](https://dev.to/p/editor_guide). DEV uses Markdown plus Jekyll front matter. Maximum **four** tags. Cover images work best at **1000 × 420**.

---

## Before you open the editor

1. Sign in at [https://dev.to/enter](https://dev.to/enter) with GitHub (recommended).
2. Confirm you will publish as a **personal account** (no DEV Organization is required).
3. Optional: Settings → Customization → editor version.
   - **Basic markdown** (default): you fill YAML front matter at the top of the post.
   - **Rich + markdown**: title, tags, cover, canonical, and series appear as separate fields. Paste only the body from the article file (everything below the second `---`).

---

## Step 1 — Create the post

| UI control | Value to enter |
|---|---|
| Top nav | Click **Create Post** (pencil icon) |
| Destination URL | `https://dev.to/new` |

---

## Step 2 — Front matter / post details

If you use the default markdown editor, paste this block as the **first** lines of the post. If you use rich + markdown, copy each row into the matching field and **do not** paste the `---` fences.

```yaml
---
title: "The Tenant Is the Database: Isolated Memory for Multi-Customer AI Products"
published: false
description: "DBX makes each customer an isolated engine — working state and vector recall in one process, one WAL, one delete. Here's why, and how to try it on your laptop."
tags: ai, golang, database, tutorial
cover_image: https://vanshjain-0702.github.io/DBX-Database-Extreme/assets/og-image.jpg
canonical_url:
series: DBX
---
```

### Field-by-field (copy these strings, nothing else)

| DEV.to field | Type it exactly as |
|---|---|
| `title` | `The Tenant Is the Database: Isolated Memory for Multi-Customer AI Products` |
| `published` | `false` until you have previewed. Flip to `true` (or click **Publish**) only after Step 6. |
| `description` | `DBX makes each customer an isolated engine — working state and vector recall in one process, one WAL, one delete. Here's why, and how to try it on your laptop.` |
| `tags` | `ai, golang, database, tutorial` |
| Cover image URL | `https://vanshjain-0702.github.io/DBX-Database-Extreme/assets/og-image.jpg` |
| Cover image alt (if the uploader asks) | `DBX — per-tenant memory engine for AI products` |
| `canonical_url` | **Leave empty.** This post is the original. Do not point it at GitHub Pages unless you later republish the same text there first. |
| `series` | `DBX` |

### Tag rules

DEV accepts at most four tags. Enter them **without** `#`. These four are the ones the post is written for:

| Tag | Why it is here |
|---|---|
| `ai` | The buyer is shipping agents / copilots / per-client RAG |
| `golang` | The engine is Go; `strict` isolation is a Go worker |
| `database` | Per-tenant WAL, SQ8 HNSW, checkpoints |
| `tutorial` | Fifteen-minute path, Python SDK, RESP AUTH |

Do **not** add `redis`, `python`, `security`, or `opensource`. The fifth tag will be dropped. (`python` is tempting because of `TenantMemory`; the Isolation Kernel post already owns `security`.)

---

## Step 3 — Paste the article body

1. Open [`the-tenant-is-the-database.md`](the-tenant-is-the-database.md).
2. If Step 2 already inserted front matter, paste **only the body** (from `You are shipping an agent platform…` onward).
3. If the editor is empty, paste the **entire file** including the `---` front matter.
4. Do not retitle H2s. DEV already uses the `title` field as the page `<h1>`; the article starts at `##`.

Liquid tags in the body are intentional. Leave them as-is:

```
{% embed https://github.com/vanshjain-0702/DBX-Database-Extreme %}
{% cta https://github.com/vanshjain-0702/DBX-Database-Extreme %} Clone the DBX repo {% endcta %}
{% cta https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/ %} Read the technical docs {% endcta %}
```

---

## Step 4 — Links that must appear (verify in Preview)

| Label in the post | URL |
|---|---|
| GitHub repository | `https://github.com/vanshjain-0702/DBX-Database-Extreme` |
| GitHub Release v1.1.0 | `https://github.com/vanshjain-0702/DBX-Database-Extreme/releases/tag/v1.1.0` |
| Technical documentation (site) | `https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/` |
| Positioning (source of truth) | `https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/positioning.md` |
| Isolation Kernel | `https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/isolation.md` |
| Isolation Kernel article (repo copy) | `https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/blog/building-secure-multi-tenant-ai-memory.md` |
| Quickstart | `https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/quickstart.html` |
| Public site | `https://vanshjain-0702.github.io/DBX-Database-Extreme/` |
| Product walkthrough video | `https://vanshjain-0702.github.io/DBX-Database-Extreme/demo.html` |
| `examples/quickstart.py` | `https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/examples/quickstart.py` |

If you later publish the Isolation Kernel post, replace the GitHub blob link to `building-secure-multi-tenant-ai-memory.md` with that DEV.to URL. Until then, keep the repo link so readers are not sent to a 404.

---

## Step 5 — Save draft and preview

| UI control | Value / action |
|---|---|
| **Save draft** | Click once. `published` stays `false`. |
| Draft URL | Share only if you want review. DEV marks it “public but secret.” |
| **Preview** | Confirm cover image, four tags, code fences, and both CTA buttons. |
| Check | Title is not duplicated as a `#` heading in the body. |

---

## Step 6 — Publish

| UI control | Value to enter |
|---|---|
| `published` (markdown editor) | Change `false` → `true` |
| **Publish** button (either editor) | Click after Preview looks right |
| Schedule (optional) | Leave unset for immediate publish |
| Comments | Leave enabled |
| Canonical URL | Still empty |

After publish, the live slug will look like:

`https://dev.to/<your-username>/the-tenant-is-the-database-isolated-memory-for-multi-customer-ai-products`

If DEV truncates the slug, that is normal. Do not edit the title to force a shorter slug.

---

## Step 7 — Optional cross-post later

Only if you later put the same article on the GitHub Pages site:

| Field | Then set it to |
|---|---|
| DEV `canonical_url` | The Pages URL of that page (example: `https://vanshjain-0702.github.io/DBX-Database-Extreme/blog/tenant.html`) |
| Pages `<link rel="canonical">` | That same Pages URL (Pages remains canonical) |

Until that page exists, leave `canonical_url` blank so DEV is the original.

---

## Claims you must not add in the editor

These contradict [`docs/isolation.md`](../isolation.md) and [`docs/positioning.md`](../positioning.md). If a preview comment asks you to “punch it up,” refuse:

- “Drop-in Redis replacement”
- “The strongest sandbox ever built”
- “Sub-millisecond ANN under burst ingest” (certified search p50 at 100k × 128 is **2.304 ms**)
- “Embeddings are encrypted at rest” (`.vec` SQ8 rows stay mmap’d plaintext; use LUKS/fscrypt)
- “Cheaper than Pinecone” / “All-in-one AI database” / “Scales to billions of vectors”
- Extra tags beyond the four listed above
