"""Streaming authenticated backups of encrypted SQLite or a stopped DBX volume.

An HKDF domain-separated key encrypts the complete archive, including metadata.
No key material is included. Restore always targets a new path and checks the
GCM tag before publishing any plaintext. SQLite restore also verifies integrity,
the database keys, and the retained audit chain.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import stat
import sys
import tarfile
import tempfile

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.kdf.hkdf import HKDF

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from persistence import Database  # noqa: E402

MAGIC = b"JURIXBK1"
HEADER_SIZE = len(MAGIC) + 1 + 16 + 12
CHUNK = 1024 * 1024
KINDS = {"sqlite": b"S", "directory": b"D"}


def _key(salt: bytes) -> bytes:
    raw = base64.b64decode(os.environ.get("JURIX_ENCRYPTION_KEY", ""), altchars=b"-_", validate=True)
    if len(raw) != 32:
        raise ValueError("configure the original 32-byte encryption key")
    return HKDF(algorithm=hashes.SHA256(), length=32, salt=salt,
                info=b"jurix-private-server-backup-v1").derive(raw)


def _encrypt(source: Path, destination: Path, kind: str) -> None:
    salt, nonce = os.urandom(16), os.urandom(12)
    header = MAGIC + KINDS[kind] + salt + nonce
    encryptor = Cipher(algorithms.AES(_key(salt)), modes.GCM(nonce)).encryptor()
    encryptor.authenticate_additional_data(header)
    created = False
    try:
        with destination.open("xb") as output:
            created = True
            destination.chmod(0o600)
            output.write(header)
            with source.open("rb") as incoming:
                while chunk := incoming.read(CHUNK):
                    output.write(encryptor.update(chunk))
            output.write(encryptor.finalize())
            output.write(encryptor.tag)
            output.flush()
            os.fsync(output.fileno())
    except BaseException:
        if created:
            destination.unlink(missing_ok=True)
        raise


def _decrypt(source: Path, destination: Path, kind: str) -> None:
    size = source.stat().st_size
    if size < HEADER_SIZE + 16:
        raise ValueError("backup is truncated")
    with source.open("rb") as incoming:
        header = incoming.read(HEADER_SIZE)
        if header[:8] != MAGIC or header[8:9] != KINDS[kind]:
            raise ValueError("backup type or format does not match the requested operation")
        incoming.seek(-16, os.SEEK_END)
        tag = incoming.read(16)
        decryptor = Cipher(algorithms.AES(_key(header[9:25])), modes.GCM(header[25:37], tag)).decryptor()
        decryptor.authenticate_additional_data(header)
        incoming.seek(HEADER_SIZE)
        remaining = size - HEADER_SIZE - 16
        with destination.open("xb") as output:
            destination.chmod(0o600)
            while remaining:
                chunk = incoming.read(min(CHUNK, remaining))
                if not chunk:
                    raise ValueError("backup is truncated")
                remaining -= len(chunk)
                output.write(decryptor.update(chunk))
            output.write(decryptor.finalize())
            output.flush()
            os.fsync(output.fileno())


def _check_sqlite(path: Path) -> dict:
    with sqlite3.connect(path) as connection:
        if connection.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
            raise ValueError("SQLite integrity check failed")
        if connection.execute("PRAGMA foreign_key_check").fetchone():
            raise ValueError("SQLite foreign key check failed")
    database = Database(str(path))  # also validates the encryption and audit keys
    audit = database.verify_audit()
    if not audit.get("valid"):
        raise ValueError("audit verification failed")
    # Close and merge a possible WAL before copying the restored single-file DB.
    with database.connection() as connection:
        checkpoint = connection.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchone()
        if checkpoint and checkpoint[0] != 0:
            raise ValueError("database checkpoint is busy")
    return audit


def _digest(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as incoming:
        while chunk := incoming.read(CHUNK):
            digest.update(chunk)
    return digest.hexdigest()


def _safe_member(info: tarfile.TarInfo) -> tarfile.TarInfo | None:
    if info.issym() or info.islnk() or info.isdev() or info.isfifo():
        if info.issym() or info.islnk():
            raise ValueError("volume snapshot cannot include symbolic or hard links")
        return None  # Unix sockets and devices are recreated at startup.
    return info


def main() -> None:
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=["backup", "restore", "verify", "snapshot-directory", "restore-directory"])
    parser.add_argument("--database", default=os.environ.get("JURIX_DATABASE_PATH"))
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--out", type=Path)
    parser.add_argument("--source", type=Path)
    parser.add_argument("--temp-dir", type=Path, default=Path("/backups"))
    args = parser.parse_args()
    if not args.temp_dir.is_dir():
        raise ValueError("the private scratch/backup directory must already exist")
    with tempfile.TemporaryDirectory(prefix=".jurix-backup-", dir=args.temp_dir) as scratch:
        temporary = Path(scratch) / "snapshot"
        result: dict = {"operation": args.operation}
        if args.operation == "backup":
            if not args.database or not Path(args.database).is_file() or args.out is None:
                raise ValueError("backup requires an existing database and a new --out archive")
            Database(args.database).backup(str(temporary))
            result["audit"] = _check_sqlite(temporary)
            _encrypt(temporary, args.out, "sqlite")
            result.update(archive=str(args.out), sha256=_digest(args.out))
        elif args.operation in {"restore", "verify"}:
            if args.archive is None:
                raise ValueError("restore/verify requires --archive")
            if args.operation == "restore" and (args.out is None or args.out.exists()):
                raise ValueError("restore requires a new --out database path; existing data is never overwritten")
            _decrypt(args.archive, temporary, "sqlite")
            result["audit"] = _check_sqlite(temporary)
            if args.operation == "restore":
                with temporary.open("rb") as incoming, args.out.open("xb") as output:
                    args.out.chmod(0o600)
                    shutil.copyfileobj(incoming, output, CHUNK)
                    output.flush()
                    os.fsync(output.fileno())
                result["database"] = str(args.out)
        elif args.operation == "snapshot-directory":
            if args.source is None or not args.source.is_dir() or args.out is None:
                raise ValueError("snapshot-directory requires an existing --source and a new --out archive")
            with tarfile.open(temporary, "w", dereference=False) as archive:
                archive.add(args.source, arcname="data", filter=_safe_member)
            _encrypt(temporary, args.out, "directory")
            result.update(archive=str(args.out), sha256=_digest(args.out))
        else:
            if args.archive is None or args.out is None or args.out.exists():
                raise ValueError("restore-directory requires --archive and a new --out directory")
            _decrypt(args.archive, temporary, "directory")
            staged = Path(scratch) / "restore"
            staged.mkdir(mode=0o700)
            with tarfile.open(temporary, "r") as archive:
                for member in archive.getmembers():
                    if not (member.name == "data" or member.name.startswith("data/")) or not (member.isfile() or member.isdir()):
                        raise ValueError("unexpected path or entry in directory backup")
                archive.extractall(staged, filter="data")
            shutil.copytree(staged / "data", args.out)
            result["directory"] = str(args.out)
        print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        # Provider material and document contents are not part of these messages.
        print("Backup operation failed: " + type(exc).__name__, file=sys.stderr)
        raise SystemExit(1) from exc
