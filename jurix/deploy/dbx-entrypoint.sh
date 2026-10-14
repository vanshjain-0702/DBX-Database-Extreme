#!/bin/sh
set -eu
umask 077
fail() { printf '%s\n' "DBX deployment startup failed: $1" >&2; exit 1; }
[ "${#DBX_ADMIN_PASSWORD}" -ge 20 ] || fail 'password must contain at least 20 characters'
case "$DBX_ADMIN_PASSWORD" in adminadminadmin|passwordpasswordpassword|replace-*) fail 'set a unique strong password';; esac
[ "${#DBX_JWT_SECRET}" -ge 32 ] || fail 'JWT secret must contain at least 32 random characters'
[ "${#DBX_INTERNAL_API_TOKEN}" -ge 32 ] || fail 'internal token must contain at least 32 random characters'
[ "$DBX_JWT_SECRET" != "$DBX_INTERNAL_API_TOKEN" ] || fail 'JWT and internal secrets must differ'
case "$DBX_JWT_SECRET:$DBX_INTERNAL_API_TOKEN" in *replace-*) fail 'replace secret placeholders';; esac
[ "${#DBX_KEK}" -eq 64 ] || fail 'wrapping key must contain 64 hexadecimal characters'
case "$DBX_KEK" in *[!0-9a-fA-F]*) fail 'wrapping key must be hexadecimal';; esac
exec dbx-orchestrator "$@"
