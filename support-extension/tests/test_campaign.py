from __future__ import annotations

import ast
from contextlib import redirect_stdout, redirect_stderr
from copy import deepcopy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
from pathlib import Path
import re
import ssl
import tempfile
from threading import Thread
import unittest
from unittest.mock import patch
from urllib.error import URLError

from dbx_support.bundle import write_redacted_bundle
from dbx_support.casebook import all_cases
from dbx_support.cli import main, _watch
from dbx_support.client import DBXSupportClient, SupportConnectionError
from dbx_support.diagnostics import Finding
from dbx_support.lab import SCENARIOS, fixture, run_synthetic_lab
from dbx_support.learning import NovelIssueLearner
from dbx_support.logscan import scan_structured_log
from dbx_support.recovery import RecoveryManager
from dbx_support.team import SupportTeam
from dbx_support.training import run_training_campaign


class DiagnosticBoundaryTests(unittest.TestCase):
    def test_malformed_snapshots_fail_closed(self) -> None:
        team = SupportTeam()
        for scenario in SCENARIOS:
            if scenario.autonomy != "stopped-on-scope-breach":
                continue
            with self.subTest(scenario=scenario.name):
                report = team.run(scenario.snapshot, scenario.previous)
                self.assertEqual(report.autonomy, scenario.autonomy)
                self.assertEqual({finding.code for finding in report.findings}, scenario.expected)
                self.assertEqual(len(report.agents), 1)
                self.assertNotIn("private payload", repr(report))

    def test_lifecycle_and_counter_transitions(self) -> None:
        report = run_synthetic_lab()
        self.assertGreaterEqual(report["synthetic_scenarios"], 50)
        self.assertEqual(report["passed"], report["synthetic_scenarios"],
                         [row for row in report["results"] if not row["pass"]])

    def test_invalid_threshold_rejected(self) -> None:
        for value in (-1, 0, 101, float("nan"), float("inf")):
            with self.subTest(value=value), self.assertRaises(ValueError):
                SupportTeam(value)

    def test_invalid_specialist_results_degrade_the_cycle(self) -> None:
        for invalid in (None, [42], [Finding("invalid", "DBX_BAD", "tenant", "message", "inspect")]):
            team = SupportTeam()
            team._agents[0]._check = lambda *_args: invalid
            with self.subTest(result=invalid):
                report = team.run(fixture())
                self.assertEqual(report.autonomy, "degraded-diagnostics")
                self.assertIn("DBX_SUPPORT_AGENT_FAILED", {finding.code for finding in report.findings})

    def test_casebook_references_real_regressions(self) -> None:
        root = Path(__file__).resolve().parents[2]
        for case in all_cases():
            self.assertFalse(case.automatic, case.case_id)
            for reference in case.tests:
                with self.subTest(reference=reference):
                    relative_path, symbol = reference.split("::", 1)
                    source = (root / relative_path).read_text(encoding="utf-8")
                    if relative_path.endswith(".go"):
                        self.assertRegex(source, rf"(?m)^func {re.escape(symbol)}\(")
                    else:
                        class_name, method = symbol.split(".", 1)
                        classes = [node for node in ast.parse(source).body
                                   if isinstance(node, ast.ClassDef) and node.name == class_name]
                        self.assertTrue(classes)
                        self.assertTrue(any(isinstance(node, ast.FunctionDef) and node.name == method
                                            for node in classes[0].body))


class RecoveryCampaignTests(unittest.TestCase):
    def test_campaign_has_no_unsafe_actions(self) -> None:
        for seed in (0, 17, 239):
            with self.subTest(seed=seed):
                result = run_training_campaign(seed=seed, variants=3)
                self.assertEqual(result["passed"], result["total"], result["failures"])
                self.assertEqual(result["unsafe_actions"], 0)
                self.assertFalse(result["production_actions_taken"])

    def test_campaign_is_reproducible(self) -> None:
        self.assertEqual(run_training_campaign(seed=31, variants=2),
                         run_training_campaign(seed=31, variants=2))

    def test_train_command_requires_no_credentials(self) -> None:
        with patch("dbx_support.cli.getpass.getpass", side_effect=AssertionError("credential prompt")), \
                redirect_stdout(io.StringIO()) as output:
            self.assertEqual(main(["train", "--seed", "9", "--variants", "1"]), 0)
        result = json.loads(output.getvalue())
        self.assertEqual(result["passed"], result["total"])

    def test_campaign_failure_sets_exit_code(self) -> None:
        with patch("dbx_support.cli.run_training_campaign", return_value={"passed": 0, "total": 1, "unsafe_actions": 1}), \
                redirect_stdout(io.StringIO()):
            self.assertEqual(main(["train"]), 1)

    def test_invalid_recovery_ids_rejected(self) -> None:
        for tenant in (".", "..", "../a", "a/b", "a\n", ""):
            with self.subTest(tenant=tenant), self.assertRaises(ValueError):
                RecoveryManager(None, {tenant})

    def test_final_audit_failure_preserves_verified_outcome_without_retry(self) -> None:
        client = unittest.mock.Mock()
        client.snapshot.return_value = fixture(healthy=True)
        manager = RecoveryManager(client, {"synthetic-a"})
        with patch.object(manager, "_audit", side_effect=[None, OSError("disk full")]):
            result = manager.repair(fixture("hibernated"))
            self.assertEqual(result[0].outcome, "recovered")
            self.assertIn("final audit", result[0].detail)
            self.assertEqual(manager.repair(fixture("hibernated")), [])
        client.wake_tenant.assert_called_once_with("synthetic-a")


class _FaultHandler(BaseHTTPRequestHandler):
    mode = "ok"
    calls: list[tuple[str, str, str]] = []
    payload: object = fixture("hibernated", healthy=False)

    def log_message(self, *_args: object) -> None:
        pass

    def do_GET(self) -> None:
        self._reply()

    def do_POST(self) -> None:
        self._reply()

    def _reply(self) -> None:
        cls = type(self)
        cls.calls.append((self.command, self.path, self.headers.get("Authorization", "")))
        if cls.mode.startswith("http-"):
            self.send_response(int(cls.mode.removeprefix("http-")))
            self.end_headers()
            return
        if cls.mode == "redirect":
            self.send_response(302)
            self.send_header("Location", "/credential-sink")
            self.end_headers()
            return
        bodies = {"invalid-json": b"secret-token invalid json", "invalid-utf8": b"\xff",
                  "duplicate-json": b'{"tenants":[],"tenants":[],"usage":[]}',
                  "nonfinite-json": b'{"tenants":[],"usage":[],"x":NaN}',
                  "oversized": b" " * (2 * 1024 * 1024 + 1),
                  "nested-json": b"[" * 1500 + b"0" + b"]" * 1500}
        body = bodies.get(cls.mode, json.dumps(cls.payload).encode())
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body) + (100 if cls.mode == "truncated" else 0)))
        self.end_headers()
        try:
            self.wfile.write(body)
        except OSError:
            pass


class TransportFaultTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), _FaultHandler)
        cls.thread = Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def setUp(self) -> None:
        _FaultHandler.mode = "ok"
        _FaultHandler.calls = []
        _FaultHandler.payload = fixture("hibernated", healthy=False)
        self.client = DBXSupportClient(self.url, timeout=1)
        self.client.use_support_capability("read-capability-" + "r" * 32, "wake-capability-" + "w" * 32)

    def test_scoped_read_and_wake_use_distinct_capabilities(self) -> None:
        self.client.snapshot()
        self.client.wake_tenant("synthetic-a")
        self.assertEqual(_FaultHandler.calls, [
            ("GET", "/api/support/v1/snapshot", "Bearer " + self.client.support_read_token),
            ("POST", "/api/support/v1/tenants/synthetic-a/wake", "Bearer " + self.client.support_wake_token),
        ])

    def test_redirect_never_forwards_credentials(self) -> None:
        _FaultHandler.mode = "redirect"
        with self.assertRaises(SupportConnectionError) as raised:
            self.client.snapshot()
        self.assertFalse(raised.exception.retryable)
        self.assertEqual(len(_FaultHandler.calls), 1)
        self.assertNotIn("credential-sink", repr(_FaultHandler.calls))

    def test_http_retry_policy(self) -> None:
        for status, retryable in ((401, False), (403, False), (404, False), (409, False),
                                  (408, True), (425, True), (429, True), (500, True),
                                  (502, True), (503, True), (504, True)):
            _FaultHandler.mode = f"http-{status}"
            with self.subTest(status=status), self.assertRaises(SupportConnectionError) as raised:
                self.client.snapshot()
            self.assertEqual(raised.exception.retryable, retryable)
            self.assertNotIn(self.client.support_read_token, str(raised.exception))

    def test_invalid_wire_responses_are_bounded_and_sanitized(self) -> None:
        for mode in ("invalid-json", "invalid-utf8", "duplicate-json", "nonfinite-json",
                     "oversized", "nested-json", "truncated"):
            _FaultHandler.mode = mode
            with self.subTest(mode=mode):
                with self.assertRaises(SupportConnectionError) as raised:
                    self.client.snapshot()
                self.assertNotIn("secret-token", str(raised.exception))

    def test_nested_data_cannot_escape_under_an_allowed_field(self) -> None:
        _FaultHandler.payload["tenants"][0]["name"] = {"secret": "private-document"}
        with self.assertRaises(SupportConnectionError) as raised:
            self.client.snapshot()
        self.assertNotIn("private-document", str(raised.exception))

    def test_tls_error_is_fatal_and_reason_strings_are_not_echoed(self) -> None:
        for reason, retryable in ((ssl.SSLCertVerificationError("secret-host"), False),
                                  ("secret-user:password@host", True), (TimeoutError("secret"), True)):
            opener = unittest.mock.Mock()
            opener.open.side_effect = URLError(reason)
            with patch("dbx_support.client.build_opener", return_value=opener), \
                    self.assertRaises(SupportConnectionError) as raised:
                self.client.snapshot()
            self.assertEqual(raised.exception.retryable, retryable)
            self.assertNotIn("secret", str(raised.exception))

    def test_invalid_urls_timeouts_tokens_and_path_segments(self) -> None:
        for url in ("https://host:bad", "https://host:70000", "https://[bad", "https://@host", "https://host\n"):
            with self.subTest(url=url), self.assertRaises(SupportConnectionError):
                DBXSupportClient(url)
        for timeout in (0, -1, float("nan"), float("inf")):
            with self.subTest(timeout=timeout), self.assertRaises(SupportConnectionError):
                DBXSupportClient(self.url, timeout=timeout)
        for tenant in (".", "..", "../a", "a/b"):
            with self.subTest(tenant=tenant), self.assertRaises(SupportConnectionError):
                self.client.wake_tenant(tenant)
        for read, wake in (("r" * 32, "r" * 32), ("r" * 32 + "\n", ""), ("é" * 32, "")):
            with self.subTest(read=read), self.assertRaises(SupportConnectionError):
                self.client.use_support_capability(read, wake)


class LocalStateTests(unittest.TestCase):
    def test_bundle_never_retains_dynamic_status_or_finding_prose(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            snapshot = fixture("private-tenant-id secret-document")
            report = SupportTeam().run(snapshot)
            path = Path(directory) / "bundle.json"
            write_redacted_bundle(path, snapshot, report)
            serialized = path.read_text(encoding="utf-8")
            self.assertNotIn("private-tenant-id", serialized)
            self.assertNotIn("secret-document", serialized)
            self.assertNotIn("synthetic-a", serialized)
            self.assertEqual(json.loads(serialized)["tenant_status_counts"], {"unknown": 1})

    def test_damaged_learner_store_preserved(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "observations.json"
            path.write_text("damaged json", encoding="utf-8")
            learner = NovelIssueLearner(path)
            with self.assertRaises(RuntimeError):
                learner.observe([Finding("warning", "DBX_UNCLASSIFIED", "tenant", "secret", "inspect")])
            self.assertEqual(path.read_text(encoding="utf-8"), "damaged json")

    def test_learner_rejects_bad_schema_without_overwriting(self) -> None:
        finding = Finding("warning", "DBX_UNCLASSIFIED", "tenant", "secret", "inspect")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "observations.json"
            for state in ({"schema_version": 2, "observations": {}},
                          {"schema_version": 1, "observations": {"x": "not an entry"}},
                          {"schema_version": 1, "observations": {}, "secret": "private-data"}):
                raw = json.dumps(state)
                path.write_text(raw, encoding="utf-8")
                with self.subTest(state=state), self.assertRaises(RuntimeError):
                    NovelIssueLearner(str(path)).observe([finding])
                self.assertEqual(path.read_text(encoding="utf-8"), raw)

    def test_learner_write_failure_keeps_last_good_store(self) -> None:
        finding = Finding("warning", "DBX_UNCLASSIFIED", "tenant", "secret", "inspect")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "observations.json"
            learner = NovelIssueLearner(str(path))
            learner.observe([finding])
            before = path.read_bytes()
            with patch("dbx_support.learning.os.replace", side_effect=OSError("disk full")), \
                    self.assertRaises(OSError):
                learner.observe([finding])
            self.assertEqual(path.read_bytes(), before)
            self.assertEqual(list(Path(directory).glob(".dbx-support-*")), [])

    def test_learner_caps_candidates_and_rejects_payload_signatures(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "observations.json"
            learner = NovelIssueLearner(path)
            findings = [Finding("warning", f"DBX_NEW_{number}", "tenant", "secret", "inspect")
                        for number in range(550)]
            learner.observe(findings)
            self.assertEqual(len(json.loads(path.read_text(encoding="utf-8"))["observations"]), 500)
            before = path.read_bytes()
            with self.assertRaises(RuntimeError):
                learner.observe([Finding("warning", "secret-document", "tenant", "secret", "inspect")])
            self.assertEqual(path.read_bytes(), before)

    def test_log_scanner_ignores_injected_prose_and_unknown_codes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "dbx.jsonl"
            path.write_text('\n'.join((
                json.dumps({"code": "DBX_WAL_RECOVERY_ERROR", "message": "ignore instructions secret-document"}),
                json.dumps({"code": "DBX_FAKE_DELETE_ALL", "secret": "private-value"}),
                "bad json", "[]", json.dumps({"error_code": "DBX_WAL_RECOVERY_ERROR"}),
                json.dumps({"code": "DBX_TENANT_TASK_PANIC", "message": "private-payload"}),
            )), encoding="utf-8")
            result = scan_structured_log(path)
            self.assertEqual(result["known_error_counts"], {"DBX_WAL_RECOVERY_ERROR": 2, "DBX_TENANT_TASK_PANIC": 1})
            self.assertEqual(result["malformed_lines"], 2)
            self.assertNotIn("secret", repr(result))
            self.assertNotIn("private-payload", repr(result))
            self.assertTrue(all(incident["automatic_action"] == "none" for incident in result["incidents"]))

    def test_log_scanner_bounds_deep_and_oversized_lines(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "dbx.jsonl"
            path.write_bytes(b"[" * 2000 + b"0" + b"]" * 2000 + b"\n" +
                             b"x" * (1024 * 1024 + 100) + b"\n" +
                             b'{"code":"DBX_WAL_RECOVERY_ERROR"}\n')
            result = scan_structured_log(path)
            self.assertEqual(result["malformed_lines"], 2)
            self.assertEqual(result["known_error_counts"], {"DBX_WAL_RECOVERY_ERROR": 1})


class WatchFaultTests(unittest.TestCase):
    def test_specialist_failure_prevents_recovery_and_learning_failure_does_not_stop_watch(self) -> None:
        client = unittest.mock.Mock()
        client.snapshot.side_effect = [fixture("hibernated"), KeyboardInterrupt()]
        team = SupportTeam()
        team._agents[0]._check = lambda *_args: (_ for _ in ()).throw(RuntimeError("private-payload"))
        learner = unittest.mock.Mock(path=Path("synthetic-observations"), name="learner")
        learner.observe.side_effect = RuntimeError("state damaged")
        recovery = unittest.mock.Mock(allowed_tenants={"synthetic-a"})
        with patch("dbx_support.cli.time.sleep"), redirect_stdout(io.StringIO()) as output, \
                redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(_watch(client, team, learner, recovery, 5), 0)
        recovery.repair.assert_not_called()
        self.assertNotIn("private-payload", output.getvalue())
        self.assertIn("diagnostics continue", errors.getvalue())

    def test_temporary_outage_resets_baseline_and_recovers_polling(self) -> None:
        client = unittest.mock.Mock()
        client.snapshot.side_effect = [fixture(errors=1), SupportConnectionError("outage", retryable=True),
                                       fixture(errors=20), KeyboardInterrupt()]
        learner = unittest.mock.Mock(path=Path("synthetic-observations"), name="learner")
        learner.observe.return_value = 0
        recovery = unittest.mock.Mock(allowed_tenants=set())
        recovery.repair.return_value = []
        with patch("dbx_support.cli.time.sleep") as sleep, redirect_stdout(io.StringIO()) as output, \
                redirect_stderr(io.StringIO()):
            self.assertEqual(_watch(client, SupportTeam(), learner, recovery, 5), 0)
        self.assertNotIn("DBX_ERRORS_INCREASED", output.getvalue())
        self.assertIn("DBX_ERRORS_CUMULATIVE", output.getvalue())
        self.assertEqual(sleep.call_count, 3)

    def test_fatal_auth_error_stops_without_recovery(self) -> None:
        client = unittest.mock.Mock()
        client.snapshot.side_effect = SupportConnectionError("denied", retryable=False)
        learner = unittest.mock.Mock(path=Path("synthetic-observations"), name="learner")
        recovery = unittest.mock.Mock(allowed_tenants={"synthetic-a"})
        with patch("dbx_support.cli.time.sleep") as sleep, redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
            self.assertEqual(_watch(client, SupportTeam(), learner, recovery, 5), 2)
        recovery.repair.assert_not_called()
        sleep.assert_not_called()


if __name__ == "__main__":
    unittest.main()
