#!/usr/bin/env python3
"""Fail CI when a Go package is below its coverage floor."""

from __future__ import annotations

import re
import subprocess


def coverage_percent(package: str) -> float:
    out = subprocess.check_output(["go", "test", "-cover", package], text=True)
    print(out, end="")
    match = re.search(r"coverage: ([0-9.]+)% of statements", out)
    if not match:
        raise SystemExit(f"missing coverage line for {package}")
    return float(match.group(1))


def main() -> None:
    checks = (
        ("./internal/protocol/", 80.0),
        ("./internal/persistence/", 70.0),
        ("./internal/query/", 65.0),
        ("./internal/engine/", 45.0),
    )
    failed = False
    for package, minimum in checks:
        pct = coverage_percent(package)
        print(f"{package} coverage={pct}% min={minimum}%")
        if pct < minimum:
            failed = True
    if failed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
