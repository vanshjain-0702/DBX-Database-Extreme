"""Fail closed on weak production secrets; trust the private DBX CA with public CAs."""
from __future__ import annotations

import base64
import os
from pathlib import Path
import sys
from urllib.parse import urlsplit

from cryptography.fernet import Fernet


def validate_environment() -> None:
    if os.environ.get("JURIX_ENV") != "production":
        raise ValueError("this image requires JURIX_ENV=production")
    database = Path(os.environ.get("JURIX_DATABASE_PATH", ""))
    if not database.is_absolute() or database.parent != Path("/data"):
        raise ValueError("JURIX_DATABASE_PATH must name a database directly under /data")
    encryption = os.environ.get("JURIX_ENCRYPTION_KEY", "")
    Fernet(encryption.encode("ascii"))
    audit = os.environ.get("JURIX_AUDIT_KEY", "")
    try:
        audit_bytes = base64.b64decode(audit, altchars=b"-_", validate=True)
    except (ValueError, UnicodeError) as exc:
        raise ValueError("JURIX_AUDIT_KEY must be a base64 random key") from exc
    if len(audit_bytes) < 32 or audit == encryption:
        raise ValueError("JURIX_AUDIT_KEY must be distinct and at least 32 random bytes")
    origin = urlsplit(os.environ.get("JURIX_PUBLIC_ORIGIN", ""))
    if origin.scheme != "https" or not origin.hostname or origin.username or origin.password or origin.path not in {"", "/"} or origin.query or origin.fragment:
        raise ValueError("JURIX_PUBLIC_ORIGIN must be an HTTPS origin")
    password = os.environ.get("DBX_ADMIN_PASSWORD", "")
    if len(password) < 20 or password.lower() in {"adminadminadmin", "passwordpasswordpassword", "replace-with-a-strong-admin-password"} or password.startswith("replace-"):
        raise ValueError("DBX_PASSWORD must be a unique strong password of at least 20 characters")
    control = urlsplit(os.environ.get("DBX_CONTROL_URL", ""))
    if control.scheme != "https" or control.hostname != "dbx" or control.port != 8000:
        raise ValueError("this deployment requires the internal HTTPS DBX control plane")


def configure_trust() -> None:
    private_ca = Path(os.environ.get("JURIX_DBX_CA_PATH", "/run/secrets/dbx_tls_ca"))
    public_ca = Path("/etc/ssl/certs/ca-certificates.crt")
    bundle = Path("/tmp/jurix-trust.pem")
    private = private_ca.read_bytes()
    if b"-----BEGIN CERTIFICATE-----" not in private:
        raise ValueError("DBX trust file must contain a PEM CA certificate")
    bundle.write_bytes(public_ca.read_bytes() + b"\n" + private)
    bundle.chmod(0o600)
    os.environ["SSL_CERT_FILE"] = str(bundle)
    os.environ["REQUESTS_CA_BUNDLE"] = str(bundle)


def main() -> None:
    os.umask(0o077)
    try:
        validate_environment()
        configure_trust()
        if len(sys.argv) < 2:
            raise ValueError("no application command supplied")
    except (OSError, ValueError, UnicodeError) as exc:
        # Only validation messages are logged; configuration values are never echoed.
        print("JURIX deployment startup failed: " + str(exc), file=sys.stderr)
        raise SystemExit(1) from exc
    os.execvp(sys.argv[1], sys.argv[1:])


if __name__ == "__main__":
    main()
