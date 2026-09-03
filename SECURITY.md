# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | ✅ Yes |
| < 1.0   | ❌ No |

## Reporting a Vulnerability

**Please do NOT open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately via email to: **security@dbxdb.io**

Include:
- A clear description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Any suggested mitigations

We will acknowledge receipt within **48 hours** and provide a fix timeline within **7 days** for critical issues.

## Security Best Practices

When self-hosting DBX in production:

1. **Never disable TLS** in production. Use `-tls-cert` and `-tls-key` flags.
2. **Set strong secrets:** `DBX_ADMIN_PASSWORD` (min 12 chars), `DBX_JWT_SECRET` (min 32 chars), `DBX_KEK` (64 hex chars) for envelope encryption.
3. **Run `DBX_ISOLATION_MODE=strict` on Linux.** The image defaults to this. Keep `dbx-server` on `PATH` next to the orchestrator.
4. **Firewall internal ports** (8081+). Only expose port 8000 (Orchestrator) and 6380 (RESP) to the public.
5. **Use environment variables** for secrets; never commit them to source control.
6. **Enable S3 backups** to ensure data durability.

See `docs/isolation.md` for what the Isolation Kernel does and does not claim.
