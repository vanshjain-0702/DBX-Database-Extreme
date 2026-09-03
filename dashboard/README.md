# DBX control plane UI

React + Vite + Tailwind operator UI. It is compiled into `cmd/dbx-orchestrator`
via [`embed.go`](embed.go) (`//go:embed all:dist`). There is no separate
control-plane service.

This is not the public marketing site. That lives in [`website/`](../website/)
and is served at
[github.io/DBX-Database-Extreme](https://vanshjain-0702.github.io/DBX-Database-Extreme/).

## What it covers

- Tenant list, provision, backup/restore, hibernate/wake, delete
- **Tenant keys** at `/cluster/{id}/keys` — mint `reader` / `writer` /
  `tenant-admin`. The secret is shown once. Same API as
  `POST /api/v1/tenants/{id}/keys`.
- Interactive console, data explorer, vector playground (operator JWT on
  `:8000`; application data plane is AUTH'd RESP on `:6380`)

A reader key cannot `SET`, `SETEX`, `VADD`, or `VDEL`. Production `/metrics`
is not this UI; scrape Prometheus with a Bearer token.

## Develop

```bash
cd dashboard
npm install
npm run dev     # Vite on :5173, proxies /api and /t to :8000
npm run lint
npm run build   # writes dist/ for the Go embed
```

`make run-dev` from the repo root builds `dist/` if it is missing, then starts
the orchestrator on `:8000` with the UI embedded. Pair `make run-dashboard` with
that process when you want hot reload (proxy target is `http://127.0.0.1:8000`,
override with `DBX_ORCHESTRATOR_URL`).

CI builds this package before golangci-lint so a clean checkout embeds a real
`dist/`.
