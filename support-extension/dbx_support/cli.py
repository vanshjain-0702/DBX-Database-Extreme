"""Manually invoked CLI; this package installs no service or startup task."""

from __future__ import annotations

import argparse
import getpass
import json
import sys
import time
from pathlib import Path

from . import __version__
from .bundle import write_redacted_bundle
from .casebook import CASEBOOK_VERSION, all_cases
from .client import DBXSupportClient, SupportConnectionError
from .lab import run_synthetic_lab
from .learning import NovelIssueLearner
from .logscan import scan_structured_log
from .recovery import RecoveryManager
from .schema import safe_tenant_id
from .team import SupportTeam, TeamReport
from .training import run_training_campaign


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dbx-support",
        description="Manually connect to DBX for diagnostics and optional, policy-gated recovery.",
    )
    parser.add_argument("--version", action="version", version=f"dbx-support {__version__}")
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("cases", help="show the bundled DBX bug/fix/test casebook")
    commands.add_parser("lab", help="run synthetic diagnostic scenarios without connecting to DBX")
    train = commands.add_parser("train", help="replay diagnosis and recovery-policy drills offline")
    train.add_argument("--seed", type=int, default=17, help="reproducible scenario seed")
    train.add_argument("--variants", type=int, default=5, help="scenario variations (1 to 100)")
    logs = commands.add_parser("logs", help="scan a local structured log for allowlisted error codes")
    logs.add_argument("--file", required=True, type=Path, help="local JSON-lines log file; raw lines are never retained")
    for name in ("doctor", "collect", "watch"):
        command = commands.add_parser(name, help="connect to DBX and inspect its control-plane health")
        command.add_argument("--url", required=True, help="DBX control-plane origin, for example https://dbx.example.com")
        command.add_argument("--memory-warn-percent", type=float, default=80.0)
        if name == "collect":
            command.add_argument("--out", required=True, type=Path, help="write a redacted local bundle; it is not uploaded")
        if name == "watch":
            command.add_argument("--interval", type=int, default=30, help="poll interval in seconds (minimum 5)")
            command.add_argument("--learning-state", help="local candidate store path; contains only incident codes and timestamps")
            command.add_argument("--auto-wake-tenant", action="append", default=[], metavar="ID",
                                 help="allow one automatic wake for this hibernated tenant during this session; repeat to allow more")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.command == "cases":
        _print_cases()
        return 0
    if args.command == "lab":
        result = run_synthetic_lab()
        print(json.dumps(result, indent=2))
        return 0 if result["passed"] == result["synthetic_scenarios"] else 1
    if args.command == "logs":
        try:
            print(json.dumps(scan_structured_log(args.file), indent=2))
            return 0
        except (OSError, ValueError) as exc:
            print(f"Local log scan could not complete: {exc}", file=sys.stderr)
            return 2
    if args.command == "train":
        if not 1 <= args.variants <= 100:
            parser.error("--variants must be between 1 and 100")
        result = run_training_campaign(seed=args.seed, variants=args.variants)
        print(json.dumps(result, indent=2))
        return 0 if result["passed"] == result["total"] and result["unsafe_actions"] == 0 else 1
    if not 1 <= args.memory_warn_percent <= 100:
        parser.error("--memory-warn-percent must be between 1 and 100")
    if args.command == "watch" and args.interval < 5:
        parser.error("--interval must be at least 5 seconds")
    if args.command == "watch":
        if any(not safe_tenant_id(tenant_id) for tenant_id in args.auto_wake_tenant):
            parser.error("--auto-wake-tenant IDs may contain only letters, digits, dot, underscore, and hyphen")

    try:
        client = DBXSupportClient(args.url)
        read_token = getpass.getpass("DBX support read capability: ")
        wake_token = ""
        if args.command == "watch" and args.auto_wake_tenant:
            wake_token = getpass.getpass("DBX support wake capability: ")
        client.use_support_capability(read_token, wake_token)
        print(f"Connected to DBX at {client.base_url} using tenant-allowlisted support capabilities.")
        team = SupportTeam(args.memory_warn_percent)
        if args.command in ("doctor", "collect"):
            snapshot = client.snapshot()
            report = team.run(snapshot)
            _print_snapshot(snapshot, report)
            if args.command == "collect":
                try:
                    write_redacted_bundle(args.out, snapshot, report)
                except OSError as exc:
                    print(f"Could not write the local incident bundle: {exc}", file=sys.stderr)
                    return 2
                print(f"Wrote redacted local incident bundle: {args.out}. It was not uploaded.")
            return 1 if any(f.severity == "critical" for f in report.findings) else 0
        learner = NovelIssueLearner(args.learning_state) if args.learning_state else NovelIssueLearner()
        recovery = RecoveryManager(client, set(args.auto_wake_tenant))
        return _watch(client, team, learner, recovery, args.interval)
    except (SupportConnectionError, KeyboardInterrupt) as exc:
        if isinstance(exc, KeyboardInterrupt):
            print("\nSupport session stopped.")
            return 0
        print(f"DBX support could not complete the check: {exc}", file=sys.stderr)
        return 2


def _watch(client: DBXSupportClient, team: SupportTeam, learner: NovelIssueLearner,
           recovery: RecoveryManager, interval: int) -> int:
    previous = None
    retry_delay = 5
    print(f"Watching DBX every {interval}s. Press Ctrl+C to stop; no background process is installed.")
    if recovery.allowed_tenants:
        print("Guarded recovery enabled: one wake attempt only for explicitly allowlisted hibernated tenants.")
    print(f"Learner agent: {learner.name}; new signatures are quarantined for reproduction, never auto-promoted.")
    print(f"Unclassified incident signatures are saved locally at {learner.path}.")
    try:
        while True:
            try:
                snapshot = client.snapshot()
                retry_delay = 5
            except SupportConnectionError as exc:
                if not exc.retryable:
                    raise
                previous = None
                print(f"  Temporary DBX connection failure; retrying in {retry_delay}s: {exc}", file=sys.stderr)
                time.sleep(retry_delay)
                retry_delay = min(retry_delay * 2, 300)
                continue
            report = team.run(snapshot, previous)
            print(f"\n[{report.created_at}] tenants={len(snapshot['tenants'])} findings={len(report.findings)}")
            _print_report(report)
            if report.autonomy == "diagnose-only":
                for event in recovery.repair(snapshot):
                    print(f"  repair {event.action} tenant={event.tenant_id}: {event.outcome} ({event.detail})")
            elif recovery.allowed_tenants:
                print("  recovery skipped because this diagnostic cycle did not complete cleanly")
            try:
                candidates = learner.observe(report.findings)
                if candidates:
                    print(f"  Learner recorded {candidates} unclassified observation(s); no fix was inferred or enabled.")
            except (OSError, RuntimeError) as exc:
                print(f"  Learner store unavailable; diagnostics continue without persistence: {exc}", file=sys.stderr)
            # Incomplete cycles and connection gaps cannot supply a trustworthy
            # baseline for a new-incident claim on the next successful cycle.
            previous = snapshot if report.autonomy == "diagnose-only" else None
            time.sleep(interval)
    except KeyboardInterrupt:
        print("\nSupport session stopped.")
        return 0
    except SupportConnectionError as exc:
        print(f"DBX connection was lost; monitoring stopped: {exc}", file=sys.stderr)
        return 2


def _print_snapshot(snapshot: dict, report: TeamReport) -> None:
    print(f"Tenants: {len(snapshot['tenants'])}")
    usage = {str(row.get("tenant_id", "")): row for row in snapshot["usage"]}
    for tenant in sorted(snapshot["tenants"], key=lambda row: str(row.get("id", ""))):
        tenant_id = str(tenant.get("id", "unknown"))
        row = usage.get(tenant_id, {})
        used = int(row.get("memory_used_bytes", 0) or 0)
        limit = int(row.get("memory_limit_bytes", 0) or 0)
        memory = f"{used / (1024 * 1024):.1f}/{limit / (1024 * 1024):.1f} MiB" if limit else "unknown"
        print(f"  {tenant_id}: {tenant.get('status', 'unknown')} | memory {memory} | errors {row.get('errors', 0)}")
    _print_report(report)
    if not report.findings:
        print("No configured health rules fired. This is not a complete DBX correctness audit.")


def _print_report(report: TeamReport) -> None:
    print(f"Agent manager: {len(report.agents)} specialists | casebook {report.casebook_version} | mode {report.autonomy}")
    for agent in report.agents:
        print(f"  agent {agent.agent}: {agent.state}; findings={agent.findings}")
    if not report.findings:
        print("  No findings.")
        return
    for finding in report.findings:
        print(f"  [{finding.severity.upper()}] {finding.code} ({finding.subject}) via {finding.agent}: {finding.message}")
        if finding.case_id:
            print(f"    Case: {finding.case_id}; tests: {', '.join(finding.test_refs) or 'not yet mapped'}")
        print(f"    Next: {finding.next_step}")
        print(f"    Automated recovery: {finding.automated_recovery}")
    print(f"  Scope: {report.scope_note}")


def _print_cases() -> None:
    print(f"DBX support casebook {CASEBOOK_VERSION} ({len(all_cases())} seed entries; casebook actions disabled)")
    for case in all_cases():
        print(f"\n{case.case_id}: {case.title}")
        print(f"  Diagnosis: {case.diagnosis}")
        print(f"  Regression evidence: {', '.join(case.tests) if case.tests else 'not yet mapped to a specific DBX test'}")
        print(f"  Recovery: {case.recovery}")
        print(f"  Autonomous action enabled: {'yes' if case.automatic else 'no'}")
