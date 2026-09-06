# How to address DBX on DEV.to

Use this file as the checklist while you sit in the [DEV Community editor](https://dev.to/new). Every step lists the **exact value** to type or paste. Do not improvise titles, tags, or links: they are chosen to match the v1.1.0 Isolation Kernel and the [language guide](../positioning.md).

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
title: "Building Secure Multi-Tenant AI Memory in Go: Sandboxing Vector Stores with Linux Landlock"
published: false
description: "How DBX isolates multi-tenant agent memory in Go with Linux Landlock, envelope encryption, and SO_PEERCRED — without a microVM per user."
tags: golang, ai, database, security
cover_image: https://vanshjain-0702.github.io/DBX-Database-Extreme/assets/og-image.jpg
canonical_url:
series: DBX Isolation Kernel
---
```

### Field-by-field (copy these strings, nothing else)

| DEV.to field | Type it exactly as |
|---|---|
| `title` | `Building Secure Multi-Tenant AI Memory in Go: Sandboxing Vector Stores with Linux Landlock` |
| `published` | `false` until you have previewed. Flip to `true` (or click **Publish**) only after Step 6. |
| `description` | `How DBX isolates multi-tenant agent memory in Go with Linux Landlock, envelope encryption, and SO_PEERCRED — without a microVM per user.` |
| `tags` | `golang, ai, database, security` |
| Cover image URL | `https://vanshjain-0702.github.io/DBX-Database-Extreme/assets/og-image.jpg` |
| Cover image alt (if the uploader asks) | `DBX Isolation Kernel — per-tenant memory engine for AI products` |
| `canonical_url` | **Leave empty.** This post is the original. Do not point it at GitHub Pages unless you later republish the same text there first. |
| `series` | `DBX Isolation Kernel` |

### Tag rules

DEV accepts at most four tags. Enter them **without** `#`. These four are the ones the post is written for:

| Tag | Why it is here |
|---|---|
| `golang` | Landlock, `SO_PEERCRED`, and envelope encryption are shown as Go |
| `ai` | The failure mode is multi-tenant agent memory / prompt-injection bleed |
| `database` | Per-tenant WAL, SQ8 HNSW, checkpoints |
| `security` | Kernel LSM + cryptographic shredding + Unix peer credentials |

Do **not** add `redis`, `linux`, `devops`, or `opensource`. The fifth tag will be dropped.

---

## Step 3 — Paste the article body

1. Open [`building-secure-multi-tenant-ai-memory.md`](building-secure-multi-tenant-ai-memory.md).
2. If Step 2 already inserted front matter, paste **only the body** (from `Multi-tenant agent platforms…` onward).
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
| Isolation Kernel (source of truth) | `https://github.com/vanshjain-0702/DBX-Database-Extreme/blob/main/docs/isolation.md` |
| Architecture | `https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/architecture.html` |
| Quickstart | `https://vanshjain-0702.github.io/DBX-Database-Extreme/docs/quickstart.html` |
| Public site | `https://vanshjain-0702.github.io/DBX-Database-Extreme/` |
| Product walkthrough video | `https://vanshjain-0702.github.io/DBX-Database-Extreme/demo.html` |

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

`https://dev.to/<your-username>/building-secure-multi-tenant-ai-memory-in-go-sandboxing-vector-stores-with-linux-landlock`

If DEV truncates the slug, that is normal. Do not edit the title to force a shorter slug.

---

## Step 7 — Optional cross-post later

Only if you later put the same article on the GitHub Pages site:

| Field | Then set it to |
|---|---|
| DEV `canonical_url` | The Pages URL of that page (example: `https://vanshjain-0702.github.io/DBX-Database-Extreme/blog/landlock.html`) |
| Pages `<link rel="canonical">` | That same Pages URL (Pages remains canonical) |

Until that page exists, leave `canonical_url` blank so DEV is the original.

---

## Claims you must not add in the editor

These contradict [`docs/isolation.md`](../isolation.md) and [`docs/positioning.md`](../positioning.md). If a preview comment asks you to “punch it up,” refuse:

- “Drop-in Redis replacement”
- “The strongest sandbox ever built”
- “Sub-millisecond ANN under burst ingest” (certified search p50 at 100k × 128 is **2.304 ms**)
- “Embeddings are encrypted at rest” (`.vec` SQ8 rows stay mmap’d plaintext; use LUKS/fscrypt)
- “Landlock blocks `connect()` / sibling `stat()`” (it does not; Unix mode + `SO_PEERCRED` do)
- Extra tags beyond the four listed above
