from __future__ import annotations

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import tempfile
from threading import Thread
import unittest

from dbx_support.casebook import HISTORICAL_CASES, case_for
from dbx_support.client import DBXSupportClient, SupportConnectionError
from dbx_support.diagnostics import Finding, analyze
from dbx_support.learning import NovelIssueLearner
from dbx_support.recovery import RecoveryManager
from dbx_support.team import SupportTeam


class MockDBXHandler(BaseHTTPRequestHandler):
    calls: list[tuple[str, str, str]] = []
    reject_usage = False

    def log_message(self, _format: str, *_args: object) -> None:
        pass

    def _json(self, status: int, payload: object) -> None:
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self) -> None:
        MockDBXHandler.calls.append(("POST", self.path, self.headers.get("Authorization", "")))
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if self.path != "/api/login":
            self._json(405, {"error": "writes are not allowed in this mock"})
            return
        credentials = json.loads(body.decode())
        if credentials != {"username": "operator", "password": "test-password"}:
            self._json(401, {"error": "secret-password-in-server-response"})
            return
        self._json(200, {"token": "mock-session-token"})

    def do_GET(self) -> None:
        MockDBXHandler.calls.append(("GET", self.path, self.headers.get("Authorization", "")))
        if self.headers.get("Authorization") != "Bearer mock-session-token":
            self._json(401, {"error": "secret-token-rejected"})
            return
        if self.path == "/api/tenants":
            self._json(200, [
                {"id": "matter-a", "status": "running", "healthy": True},
                {"id": "clinic-b", "status": "down", "healthy": False},
                {"id": "archive-c", "status": "hibernated", "healthy": False,
                 "secret": "payload-must-not-reach-agents", "embedding": [9, 8, 7]},
            ])
        elif self.path == "/api/usage":
            if MockDBXHandler.reject_usage:
                self._json(403, {"error": "secret-api-response"})
            else:
                self._json(200, [
                    {"tenant_id": "matter-a", "memory_used_bytes": 90,
                     "memory_limit_bytes": 100, "errors": 3},
                    {"tenant_id": "clinic-b", "memory_used_bytes": 20,
                     "memory_limit_bytes": 100, "errors": 0},
                    {"tenant_id": "archive-c", "memory_used_bytes": 0,
                     "memory_limit_bytes": 100, "errors": 0,
                     "document": "private-document", "secret": "private-secret"},
                ])
        else:
            self._json(404, {"error": "not found"})


class SupportExtensionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), MockDBXHandler)
        cls.thread = Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def setUp(self) -> None:
        MockDBXHandler.calls = []
        MockDBXHandler.reject_usage = False

    def test_login_and_snapshot_use_only_documented_read_endpoints(self) -> None:
        client = DBXSupportClient(self.url)
        client.login("operator", "test-password")
        snapshot = client.snapshot()

        self.assertEqual(len(snapshot["tenants"]), 3)
        self.assertEqual(len(snapshot["usage"]), 3)
        self.assertEqual(
            [(method, path) for method, path, _ in MockDBXHandler.calls],
            [("POST", "/api/login"), ("GET", "/api/tenants"), ("GET", "/api/usage")],
        )
        self.assertEqual(MockDBXHandler.calls[0][2], "")
        self.assertTrue(all(auth == "Bearer mock-session-token"
                            for _, _, auth in MockDBXHandler.calls[1:]))
        self.assertNotIn("test-password", repr(snapshot))
        self.assertNotIn("mock-session-token", repr(snapshot))
        self.assertNotIn("payload-must-not-reach-agents", repr(snapshot))
        self.assertNotIn("private-document", repr(snapshot))

    def test_diagnostics_flag_down_tenant_memory_and_new_errors(self) -> None:
        snapshot = {
            "tenants": [
                {"id": "matter-a", "status": "running"},
                {"id": "clinic-b", "status": "down"},
                {"id": "archive-c", "status": "hibernated"},
            ],
            "usage": [
                {"tenant_id": "matter-a", "memory_used_bytes": 90,
                 "memory_limit_bytes": 100, "errors": 5},
                {"tenant_id": "clinic-b", "memory_used_bytes": 10,
                 "memory_limit_bytes": 100, "errors": 0},
                {"tenant_id": "archive-c", "memory_used_bytes": 0,
                 "memory_limit_bytes": 100, "errors": 0},
            ],
        }
        previous = {"tenants": snapshot["tenants"], "usage": [
            {"tenant_id": "matter-a", "errors": 3},
            {"tenant_id": "clinic-b", "errors": 0},
            {"tenant_id": "archive-c", "errors": 0},
        ]}

        findings = analyze(snapshot, previous)
        codes = {finding.code for finding in findings}
        self.assertEqual(codes, {
            "DBX_TENANT_MEMORY_HIGH", "DBX_ERRORS_INCREASED", "DBX_TENANT_DOWN",
            "DBX_TENANT_HIBERNATED",
        })
        hibernated = next(f for f in findings if f.code == "DBX_TENANT_HIBERNATED")
        self.assertEqual(hibernated.severity, "info")

    def test_first_snapshot_does_not_claim_old_error_counter_is_new(self) -> None:
        findings = analyze({"tenants": [{"id": "matter-a", "status": "running"}],
                            "usage": [{"tenant_id": "matter-a", "errors": 7}]})
        self.assertEqual([finding.code for finding in findings], ["DBX_ERRORS_CUMULATIVE"])
        self.assertEqual(findings[0].severity, "info")

    def test_remote_plain_http_and_credential_urls_are_rejected(self) -> None:
        for url in ("http://dbx.example.com", "https://operator:password@dbx.example.com",
                    "https://dbx.example.com/prefix"):
            with self.subTest(url=url), self.assertRaises(SupportConnectionError):
                DBXSupportClient(url)

    def test_api_error_response_does_not_leak_server_body_or_token(self) -> None:
        MockDBXHandler.reject_usage = True
        client = DBXSupportClient(self.url)
        client.login("operator", "test-password")
        with self.assertRaises(SupportConnectionError) as raised:
            client.snapshot()
        self.assertNotIn("secret-api-response", str(raised.exception))
        self.assertNotIn("mock-session-token", str(raised.exception))

    def test_failed_login_does_not_leak_server_response(self) -> None:
        client = DBXSupportClient(self.url)
        with self.assertRaises(SupportConnectionError) as raised:
            client.login("operator", "wrong-password")
        self.assertNotIn("secret-password-in-server-response", str(raised.exception))
        self.assertNotIn("wrong-password", str(raised.exception))

    def test_specialists_run_and_casebook_attaches_regression_evidence(self) -> None:
        snapshot = {
            "tenants": [{"id": "clinic-b", "status": "down"}],
            "usage": [{"tenant_id": "clinic-b", "memory_used_bytes": 95,
                       "memory_limit_bytes": 100, "errors": 2}],
        }
        previous = {"tenants": snapshot["tenants"], "usage": [{"tenant_id": "clinic-b", "errors": 1}]}
        report = SupportTeam().run(snapshot, previous)

        self.assertEqual(report.autonomy, "diagnose-only")
        self.assertEqual({agent.agent for agent in report.agents}, {
            "tenant-lifecycle", "capacity", "error-trend",
            "control-plane-consistency", "support-scope-guard", "casebook-analyst",
        })
        down = next(finding for finding in report.findings if finding.code == "DBX_TENANT_DOWN")
        self.assertEqual(down.case_id, "DBX-OPS-001")
        self.assertTrue(down.test_refs)
        self.assertEqual(down.automated_recovery, "none")

    def test_scope_guard_stops_other_specialists_on_unreviewed_fields(self) -> None:
        snapshot = {
            "tenants": [{"id": "clinic-b", "status": "down", "vector": [1, 2, 3]}],
            "usage": [],
        }
        report = SupportTeam().run(snapshot)
        self.assertEqual(report.autonomy, "stopped-on-scope-breach")
        self.assertEqual([agent.agent for agent in report.agents], ["support-scope-guard"])
        self.assertEqual([finding.code for finding in report.findings], ["DBX_SUPPORT_SCOPE_BREACH"])

    def test_learner_aggregates_unclassified_signatures_without_tenant_or_message(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "learning" / "observations.json"
            learner = NovelIssueLearner(path)
            finding = Finding("warning", "DBX_ERRORS_INCREASED", "sensitive-tenant-id",
                              "raw internal detail", "inspect logs", agent="error-trend")
            self.assertEqual(learner.observe([finding]), 1)
            self.assertEqual(learner.observe([finding]), 1)
            stored = path.read_text(encoding="utf-8")
            self.assertIn('"occurrences": 2', stored)
            self.assertIn('"candidate-needs-reproduction"', stored)
            self.assertNotIn("sensitive-tenant-id", stored)
            self.assertNotIn("raw internal detail", stored)

    def test_casebook_includes_historical_vector_recovery_and_test(self) -> None:
        self.assertEqual(len(HISTORICAL_CASES), 1)
        self.assertIn("TestVectorStoreRebuildsGraphAfterCrash", HISTORICAL_CASES[0].tests[0])
        self.assertIsNotNone(case_for("DBX_TENANT_DOWN"))


class RecoveryFaultInjectionTests(unittest.TestCase):
    class FakeClient:
        def __init__(self, snapshots: list[dict], wake_error: Exception | None = None,
                     snapshot_error: bool = False) -> None:
            self.snapshots = iter(snapshots)
            self.wake_error = wake_error
            self.snapshot_error = snapshot_error
            self.wake_calls: list[str] = []
            self.snapshot_calls = 0

        def wake_tenant(self, tenant_id: str) -> None:
            self.wake_calls.append(tenant_id)
            if self.wake_error:
                raise self.wake_error

        def snapshot(self) -> dict:
            self.snapshot_calls += 1
            if self.snapshot_error:
                raise SupportConnectionError("synthetic verification outage", retryable=True)
            return next(self.snapshots)

    @staticmethod
    def snapshot(status: str, healthy: bool = False) -> dict:
        return {"tenants": [{"id": "tenant-a", "status": status, "healthy": healthy}],
                "usage": [{"tenant_id": "tenant-a", "status": status}]}

    def test_audit_failure_fails_closed_before_wake(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            blocker = Path(directory) / "not-a-directory"
            blocker.write_text("file blocks audit directory", encoding="utf-8")
            client = self.FakeClient([])
            manager = RecoveryManager(client, {"tenant-a"}, blocker / "repair.jsonl")

            events = manager.repair(self.snapshot("hibernated"))

        self.assertEqual([(event.outcome, event.action) for event in events], [("skipped", "wake")])
        self.assertEqual(client.wake_calls, [])
        self.assertEqual(client.snapshot_calls, 0)

    def test_lost_wake_response_is_verified_without_retry(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            client = self.FakeClient(
                [self.snapshot("running", healthy=True)],
                SupportConnectionError("synthetic lost response", retryable=True),
            )
            manager = RecoveryManager(client, {"tenant-a"}, Path(directory) / "audit.jsonl")

            events = manager.repair(self.snapshot("hibernated"))

            self.assertEqual(events[0].outcome, "recovered")
            self.assertIn("response was lost", events[0].detail)
            self.assertEqual(client.wake_calls, ["tenant-a"])
            self.assertEqual(client.snapshot_calls, 1)
            audit = (Path(directory) / "audit.jsonl").read_text(encoding="utf-8")
            self.assertIn('"outcome": "attempting"', audit)
            self.assertIn('"outcome": "recovered"', audit)

    def test_unhealthy_postcheck_stops_and_does_not_retry(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            client = self.FakeClient([self.snapshot("running", healthy=False)])
            manager = RecoveryManager(client, {"tenant-a"}, Path(directory) / "audit.jsonl")

            first = manager.repair(self.snapshot("hibernated"))
            second = manager.repair(self.snapshot("hibernated"))

        self.assertEqual(first[0].outcome, "unverified")
        self.assertEqual(second, [])
        self.assertEqual(client.wake_calls, ["tenant-a"])
        self.assertEqual(client.snapshot_calls, 1)

    def test_postcheck_outage_records_unverified_and_never_retries(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            client = self.FakeClient([], snapshot_error=True)
            manager = RecoveryManager(client, {"tenant-a"}, Path(directory) / "audit.jsonl")

            first = manager.repair(self.snapshot("hibernated"))
            second = manager.repair(self.snapshot("hibernated"))

        self.assertEqual(first[0].outcome, "unverified")
        self.assertEqual(second, [])
        self.assertEqual(client.wake_calls, ["tenant-a"])
        self.assertEqual(client.snapshot_calls, 1)

    def test_specialist_crash_degrades_diagnostics_and_holds_recovery(self) -> None:
        team = SupportTeam()
        team._agents[0]._check = lambda _snapshot, _previous: (_ for _ in ()).throw(RuntimeError("injected"))

        report = team.run(self.snapshot("hibernated"))

        failed = next(agent for agent in report.agents if agent.agent == "tenant-lifecycle")
        self.assertEqual(failed.state, "failed")
        self.assertEqual(report.autonomy, "degraded-diagnostics")
        self.assertIn("DBX_SUPPORT_AGENT_FAILED", {finding.code for finding in report.findings})


if __name__ == "__main__":
    unittest.main()
