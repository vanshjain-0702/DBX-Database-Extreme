"""Central manager and independently scoped DBX diagnostic specialists."""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, replace
from datetime import datetime, timezone
import math
from typing import Callable

from .casebook import CASEBOOK_VERSION, case_for
from .diagnostics import (
    Finding,
    capacity_findings,
    consistency_findings,
    error_findings,
    lifecycle_findings,
    scope_findings,
)


@dataclass(frozen=True)
class AgentStatus:
    agent: str
    specialty: str
    state: str
    findings: int


@dataclass(frozen=True)
class TeamReport:
    created_at: str
    casebook_version: str
    autonomy: str
    agents: tuple[AgentStatus, ...]
    findings: tuple[Finding, ...]
    scope_note: str


class DiagnosticAgent:
    name = "base"
    specialty = "base"

    def __init__(self, check: Callable[[dict, dict | None], list[Finding]]) -> None:
        self._check = check

    def run(self, snapshot: dict, previous: dict | None) -> list[Finding]:
        return self._check(snapshot, previous)


class KnowledgeAgent:
    """Attaches test evidence and fixed-case context; cannot authorize actions."""

    name = "casebook-analyst"
    specialty = "matches symptoms to reviewed DBX cases and regression tests"

    @staticmethod
    def annotate(findings: list[Finding]) -> list[Finding]:
        annotated = []
        for finding in findings:
            case = case_for(finding.code)
            if case:
                annotated.append(replace(
                    finding,
                    case_id=case.case_id,
                    test_refs=case.tests,
                    next_step=f"{finding.next_step} Case {case.case_id}: {case.recovery}",
                    automated_recovery="none",
                ))
            else:
                annotated.append(replace(
                    finding,
                    case_id="UNCLASSIFIED",
                    automated_recovery="none",
                ))
        return annotated


class SupportTeam:
    """Runs domain-scoped read-only specialists and merges findings deterministically."""

    def __init__(self, memory_warn_percent: float = 80.0) -> None:
        if not math.isfinite(memory_warn_percent) or not 1 <= memory_warn_percent <= 100:
            raise ValueError("memory warning threshold must be between 1 and 100")
        self.memory_warn_percent = memory_warn_percent
        self._agents: tuple[DiagnosticAgent, ...] = (
            DiagnosticAgent(lambda snap, _prev: lifecycle_findings(snap)),
            DiagnosticAgent(lambda snap, _prev: capacity_findings(snap, self.memory_warn_percent)),
            DiagnosticAgent(error_findings),
            DiagnosticAgent(lambda snap, _prev: consistency_findings(snap)),
            DiagnosticAgent(lambda snap, _prev: scope_findings(snap)),
        )
        names = (
            ("tenant-lifecycle", "tenant state and availability"),
            ("capacity", "tenant memory quota and usage"),
            ("error-trend", "new changes in DBX error counters"),
            ("control-plane-consistency", "tenant inventory and usage consistency"),
            ("support-scope-guard", "refuses data fields outside the reviewed telemetry schema"),
        )
        for agent, (name, specialty) in zip(self._agents, names):
            agent.name = name
            agent.specialty = specialty
        self.knowledge_agent = KnowledgeAgent()

    def run(self, snapshot: dict, previous: dict | None = None) -> TeamReport:
        boundary_findings = scope_findings(snapshot)
        if previous is not None:
            boundary_findings += scope_findings(previous)
        if boundary_findings:
            return TeamReport(
                created_at=datetime.now(timezone.utc).isoformat(timespec="seconds"),
                casebook_version=CASEBOOK_VERSION,
                autonomy="stopped-on-scope-breach",
                agents=(AgentStatus("support-scope-guard", "diagnostic schema boundary", "blocked", len(boundary_findings)),),
                findings=tuple(boundary_findings),
                scope_note="Other specialists were not given this snapshot.",
            )
        with ThreadPoolExecutor(max_workers=len(self._agents), thread_name_prefix="dbx-support") as pool:
            results = list(pool.map(lambda agent: _run_agent(agent, snapshot, previous), self._agents))

        raw_findings = [finding for _, findings, _error in results for finding in findings]
        failures = []
        for agent, _findings, error_type in results:
            if error_type:
                failures.append(agent.name)
                raw_findings.append(Finding(
                    "warning", "DBX_SUPPORT_AGENT_FAILED", "support-team",
                    f"The {agent.name} specialist could not complete its check ({error_type}).",
                    "This diagnostic cycle is incomplete; support recovery is held until checks succeed.",
                    agent=agent.name,
                ))
        findings = self.knowledge_agent.annotate(raw_findings)
        rank = {"critical": 0, "warning": 1, "info": 2}
        findings.sort(key=lambda item: (rank.get(item.severity, 3), item.subject, item.code))
        agent_status = tuple(
            AgentStatus(agent.name, agent.specialty, "failed" if error_type else "complete", len(items))
            for agent, items, error_type in results
        ) + (AgentStatus(
            self.knowledge_agent.name,
            self.knowledge_agent.specialty,
            "complete",
            len(findings),
        ),)
        return TeamReport(
            created_at=datetime.now(timezone.utc).isoformat(timespec="seconds"),
            casebook_version=CASEBOOK_VERSION,
            autonomy="degraded-diagnostics" if failures else "diagnose-only",
            agents=agent_status,
            findings=tuple(findings),
            scope_note=(
                "Diagnostic specialists are read-only. A separate recovery manager can only "
                "request an allowlisted tenant wake when its dedicated capability is enabled."
            ),
        )


def _run_agent(agent: DiagnosticAgent, snapshot: dict, previous: dict | None) -> tuple[DiagnosticAgent, list[Finding], str]:
    try:
        findings = agent.run(snapshot, previous)
        if (not isinstance(findings, list)
                or any(not isinstance(item, Finding) or item.severity not in {"info", "warning", "critical"}
                       or not isinstance(item.code, str) or not isinstance(item.subject, str)
                       for item in findings)):
            raise TypeError("invalid specialist result")
        return agent, findings, ""
    except Exception as exc:
        return agent, [], type(exc).__name__
