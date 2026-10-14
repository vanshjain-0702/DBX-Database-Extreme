"""Refresh configured authority watches, retaining per-source failure states."""
from __future__ import annotations

import argparse
import logging
import os
from pathlib import Path
import signal
import threading
import time

from filelock import FileLock, Timeout
from service import JurixService

logger = logging.getLogger("jurix.worker")


def run_cycle(service, interval=300, heartbeat=None, clock=time.time):
    seen = set()
    watches = service.regulatory_watches()
    for member in service.store.firm_admin_emails():
        if member["firm_id"] in seen:
            continue
        seen.add(member["firm_id"])
        try:
            sources = watches.list(member["email"])["watches"]
            for source in sources:
                if heartbeat:
                    heartbeat()
                if source.get("checked_at") and clock() - source["checked_at"] < interval:
                    continue
                try:
                    watches.check(member["email"], source["id"])
                except Exception as exc:
                    logger.error("Source refresh failed (%s)", type(exc).__name__)
        except Exception as exc:
            logger.error("Firm source refresh failed (%s)", type(exc).__name__)
    if heartbeat:
        heartbeat()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--once", action="store_true", help="Check due sources once and exit")
    args = parser.parse_args()
    interval = max(60, int(os.getenv("JURIX_WATCH_INTERVAL_SECONDS", "300")))
    service = JurixService()
    stop = threading.Event()
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, lambda *_: stop.set())
    heartbeat_path = Path(os.getenv("JURIX_WORKER_HEARTBEAT_PATH", "/tmp/jurix-worker-heartbeat"))
    def heartbeat():
        heartbeat_path.touch(mode=0o600)
    try:
        while not stop.is_set():
            heartbeat()
            try:
                # Prevent duplicate scheduled refreshes when the service restarts.
                with FileLock(str(service.database.path) + ".watch-worker.lock", timeout=1):
                    run_cycle(service, interval, heartbeat)
            except Timeout:
                logger.warning("Another regulatory worker is active")
            if args.once:
                break
            stop.wait(min(interval, 30))
    finally:
        service.providers.http.close()


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    main()
