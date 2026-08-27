#!/usr/bin/env python3
"""Fail CI when the vector harness misses a GitHub-runner smoke gate."""

from __future__ import annotations

import re
import sys


def num(text: str, name: str) -> float:
    match = re.search(name + r"=([0-9.]+)", text)
    if not match:
        raise SystemExit("missing " + name)
    return float(match.group(1))


def main() -> None:
    path = sys.argv[1] if len(sys.argv) > 1 else "bench.txt"
    text = open(path, encoding="utf-8").read()
    ingest = num(text, "ingest_vectors_per_sec")
    p50 = num(text, "search_p50")
    p95 = num(text, "search_p95")
    p99 = num(text, "search_p99")
    recall = num(text, "recall_mean")
    p05 = num(text, "recall_p05")
    print("parsed", ingest, p50, p95, p99, recall, p05)
    failed = []
    if ingest < 400:
        failed.append("ingest")
    if p50 > 25:
        failed.append("p50")
    if p95 > 40:
        failed.append("p95")
    if p99 > 80:
        failed.append("p99")
    if recall < 0.80:
        failed.append("recall")
    if p05 < 0.50:
        failed.append("p05")
    if failed:
        raise SystemExit("failed gates: " + ",".join(failed))


if __name__ == "__main__":
    main()
