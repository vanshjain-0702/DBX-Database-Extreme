# Deploy Jurix on a private server

Run commands from the repository root on the target server. The Compose deployment builds the React frontend, Python application/worker, private DBX service and HTTPS reverse proxy. DBX has no host port bindings; the public interface defaults to loopback until an operator supplies a private/VPN address.

## Configure the server

1. Copy `jurix/deploy/.env.example` to `jurix/.env`. Restrict access to the file and keep it outside source control. Set the hostname, HTTPS origin and private interface address.
2. Set distinct, random keys: `JURIX_ENCRYPTION_KEY` and `JURIX_AUDIT_KEY` must each be URL-safe base64-encoded 32-byte values; `DBX_KEK` must be 64 hexadecimal characters. Set a unique DBX password of at least 20 characters and independent JWT/internal API secrets. Store the original keys in the operator's secure backup system; replacing them makes encrypted data unreadable.
3. Put the DBX TLS certificate, private key and CA certificate at the paths specified in `.env`. The server certificate must cover the internal hostname `dbx`. Compose resolves default secret paths relative to `jurix/deploy/`.
4. Create the absolute backup directory on the server, owned by UID 10001 with mode 0700. Confirm adequate disk space and the DBX disk-encryption requirement for that host.
5. Leave optional external provider credentials empty unless the firm has approved processing. The app reports these features as unavailable until configured. Only administrators can enable matter consent.

`JURIX_REGISTRATION_OPEN=false` allows only the first bootstrap administrator to create an independent firm. That administrator invites subsequent members from Firm & connection. Invited accounts can register with the invitation link. Never publish a server before its bootstrap account is established on the private interface.

## Validate, build and start

```sh
docker compose --env-file jurix/.env -f jurix/deploy/compose.yml config --quiet
docker compose --env-file jurix/.env -f jurix/deploy/compose.yml build
docker compose --env-file jurix/.env -f jurix/deploy/compose.yml up -d
docker compose --env-file jurix/.env -f jurix/deploy/compose.yml ps
```

The proxy uses Caddy's private certificate authority. Install its root CA in authorized clients' trust stores through the firm's normal device-management process. Open the configured HTTPS hostname and verify sign-in, storage readiness, a matter import/audit/reviewer decision, and an export. `/api/health` is liveness only; signed-in users see DBX connectivity through `/api/readiness` and Firm & connection.

The worker checks due sources every five minutes by default and updates a local heartbeat. A publication outage retains its previous baseline and records an error for the reviewer; it does not generate a successful source-change claim. Source watches require a supported authority's HTML/text publication URL and nonempty keywords. An unknown effective date remains unknown.

## Existing records and backups

Preserve the existing DBX volume and legacy registries. Set explicit volume names before starting; a new empty volume does not contain existing tenants. Validate legacy JSON before migration:

```sh
python jurix/migrate.py --identity /private/firm_store.json --records /private/jurix_records.json --dry-run
```

Migration into an empty encrypted database is explicit and uses the target database's original configured keys. It preserves IDs and DBX credentials, creates a pre-migration database backup, and revokes old sessions. Do not run it automatically against a populated target.

Create and verify an authenticated encrypted SQLite backup:

```sh
docker compose --env-file jurix/.env -f jurix/deploy/compose.yml --profile maintenance run --rm maintenance python jurix/deploy/backup.py backup --database /data/jurix.sqlite3 --temp-dir /backups --out /backups/jurix-backup.jxb
docker compose --env-file jurix/.env -f jurix/deploy/compose.yml --profile maintenance run --rm maintenance python jurix/deploy/backup.py verify --archive /backups/jurix-backup.jxb --temp-dir /backups
```

Use a new archive filename for every backup. DBX tenant data needs its own volume backup while DBX is stopped; a SQLite backup alone does not contain indexed documents. Restore commands only publish into a new destination and require the original keys. Perform and verify a restore rehearsal before enabling real matters.

## Release checks

The Jurix CI workflow installs locked frontend dependencies, runs backend/API/security tests, builds the frontend and builds the application container. Local verification results are recorded in `docs/jurix-frontend-readiness.md`. A successful source build and configuration validation must be followed by target-server container startup, TLS, storage, backup/restore and any enabled-provider acceptance checks before a production release is confirmed.
