# Public site

**Live (custom domain):** [https://dbxdb.co.in/](https://dbxdb.co.in/)

GitHub Pages origin: [https://vanshjain-0702.github.io/DBX-Database-Extreme/](https://vanshjain-0702.github.io/DBX-Database-Extreme/) (redirects here once the custom domain is saved and DNS is live).

**Show demo video:** [Walkthrough](https://dbxdb.co.in/demo.html) (`demo.html` + [`assets/vector-search.mp4`](assets/vector-search.mp4) recall clip + [`assets/demo.mp4`](assets/demo.mp4) product tour). **Updates:** [posts.html](https://dbxdb.co.in/posts.html) (header badge until you open it; RSS [`posts.xml`](posts.xml)). Also linked from the root [README](../README.md).

**Incubation pitch:** [pitch.html](https://dbxdb.co.in/pitch.html) — live 16:9 deck (arrow keys, full screen). How to present: [PITCH.md](PITCH.md).

Source for the site is this folder. GitHub Actions deploys it from `main`
([Deploy site](../.github/workflows/pages.yml)). Local preview from the repo
root: `make site` (http://127.0.0.1:8765/).

The repo must stay **public** for GitHub Pages on a Free plan.

## Connect `dbxdb.co.in`

Nameservers today are GoDaddy (`ns33.domaincontrol.com` /
`ns34.domaincontrol.com`). Do **GitHub first**, then DNS in the same sitting,
or github.io will redirect at a parking page.

### 1. GitHub Pages custom domain

1. Open [Settings → Pages](https://github.com/vanshjain-0702/DBX-Database-Extreme/settings/pages).
2. **Custom domain:** `dbxdb.co.in` → **Save**.
3. Leave **Enforce HTTPS** unchecked until GitHub shows a certificate (often
   10 minutes to a few hours after DNS is correct). Then check it.

This file [`CNAME`](CNAME) is published with the site so the host stays
`dbxdb.co.in`.

### 2. GoDaddy DNS

GoDaddy → **My Products** → `dbxdb.co.in` → **DNS**.

Turn **off** domain forwarding / parking if it is on. Delete the existing
apex **A** records that point at GoDaddy parking (`3.33.130.190`,
`15.197.148.33`). Do not keep a CNAME on `@`.

Then create:

| Type | Name | Value | TTL |
|---|---|---|---|
| A | `@` | `185.199.108.153` | 600 |
| A | `@` | `185.199.109.153` | 600 |
| A | `@` | `185.199.110.153` | 600 |
| A | `@` | `185.199.111.153` | 600 |
| AAAA | `@` | `2606:50c0:8000::153` | 600 |
| AAAA | `@` | `2606:50c0:8001::153` | 600 |
| AAAA | `@` | `2606:50c0:8002::153` | 600 |
| AAAA | `@` | `2606:50c0:8003::153` | 600 |
| CNAME | `www` | `vanshjain-0702.github.io` | 600 |

The `www` CNAME must be the GitHub user site (`vanshjain-0702.github.io`),
**not** `vanshjain-0702.github.io/DBX-Database-Extreme` and **not**
`dbxdb.co.in`.

IPv6 (AAAA) is optional. All four A records are required for HTTPS.

### 3. Check

From PowerShell, after DNS has moved off parking:

```powershell
Resolve-DnsName dbxdb.co.in -Type A
Resolve-DnsName www.dbxdb.co.in -Type CNAME
```

Apex A records should be the four `185.199.108–111.153` addresses. Then open
http://dbxdb.co.in/ (HTTP first). HTTPS works after **Enforce HTTPS**.

Mail (`hello@dbxdb.io` in the license) is a **different domain** and still
needs MX on that name. Pointing the website at `dbxdb.co.in` does not create
a mailbox. GitHub issues remain the channel that delivers until you add MX
and a mailbox on whichever domain you want for email.
