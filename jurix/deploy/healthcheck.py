"""Minimal local probes. Operator readiness uses the protected application API."""
from __future__ import annotations

import json
import os
from pathlib import Path
import sys
import time
from urllib.request import urlopen


def main() -> None:
    if sys.argv[1:] == ["worker"]:
        heartbeat = Path(os.environ.get("JURIX_WORKER_HEARTBEAT_PATH", "/tmp/jurix-worker-heartbeat"))
        age = time.time() - heartbeat.stat().st_mtime
        if not 0 <= age <= 180:
            raise ValueError("regulatory worker heartbeat is stale")
    elif sys.argv[1:] == ["app"]:
        with urlopen("http://127.0.0.1:8091/api/health", timeout=3) as response:
            body = json.loads(response.read(2048))
        if body.get("status") != "ok":
            raise ValueError("application liveness failed")
    else:
        raise ValueError("expected app or worker")


if __name__ == "__main__":
    try:
        main()
    except Exception:
        # Do not include provider/control-plane credentials or internal URLs.
        raise SystemExit(1)
