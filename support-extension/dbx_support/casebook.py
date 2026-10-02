"""Versioned DBX symptom-to-test seed knowledge; never executable code."""

from __future__ import annotations

from dataclasses import dataclass


CASEBOOK_VERSION = "dbx-casebook-0.4.0"


@dataclass(frozen=True)
class Case:
    case_id: str
    title: str
    diagnosis: str
    tests: tuple[str, ...]
    recovery: str
    automatic: bool = False


CASES: dict[str, Case] = {
    "DBX_TENANT_DOWN": Case(
        "DBX-OPS-001", "Tenant unavailable",
        "The control plane reports no live tenant engine. This signal alone does not identify the cause.",
        ("internal/orchestrator/manager_test.go::TestListTenantViewsReportsRunningAndDown",),
        "No automatic restart is available through the current read-only support API.",
    ),
    "DBX_TENANT_STARTING": Case(
        "DBX-OPS-002", "Tenant startup taking longer than expected",
        "A tenant remains in the starting state. Check worker startup logs and recent resource pressure.",
        ("internal/orchestrator/manager_test.go::TestStartTenantWithoutCheckoutYAML",),
        "Observe one more interval, then collect service logs; do not loop restarts.",
    ),
    "DBX_TENANT_MEMORY_HIGH": Case(
        "DBX-OPS-008", "Tenant approaching its memory quota",
        "Usage is close to the configured tenant memory limit. DBX should reject over-quota writes before WAL append.",
        (
            "internal/query/executor_test.go::TestTenantQuotaRejectsBeforeWAL",
            "internal/query/executor_surface_test.go::TestNoisyNeighborQuotaDoesNotBlockQuietTenant",
        ),
        "No automatic quota change or key deletion is permitted.",
    ),
    "DBX_ERRORS_INCREASED": Case(
        "DBX-OPS-004", "DBX error counter increased",
        "The public usage API exposes a counter, not error codes or stack traces. Root cause is unclassified until local logs are inspected.",
        (),
        "Collect a time-bounded, redacted DBX log excerpt; do not infer a code fix from the counter alone.",
    ),
    "DBX_ERRORS_CUMULATIVE": Case(
        "DBX-OPS-007", "Existing cumulative DBX error counter",
        "A one-time snapshot cannot establish when these errors occurred.",
        (),
        "Compare future counters and correlate with local logs before classifying the issue.",
    ),
    "DBX_TENANT_HIBERNATED": Case(
        "DBX-OPS-005", "Tenant is hibernated",
        "Hibernate is a lifecycle state and may be intentional, not a failure.",
        ("internal/orchestrator/lifecycle_ux_test.go::TestHibernatePersistsAndSkipsAutostart",),
        "Do not wake by default. A manually started watch session can wake it once only when the operator explicitly allowlists its tenant ID.",
    ),
    "DBX_USAGE_MISSING": Case(
        "DBX-OPS-006", "Tenant usage snapshot missing",
        "Tenant listing and usage listing disagree. Repeat the read and inspect control-plane state if the mismatch persists.",
        ("internal/orchestrator/lifecycle_ux_test.go::TestTenantUsageReportsDiskForStoppedEngine",),
        "No tenant mutation is appropriate for this control-plane consistency signal.",
    ),
    "DBX_WAL_RECOVERY_ERROR": Case(
        "DBX-ENG-008", "WAL recovery rejected a record",
        "A recovery error can indicate a truncated or corrupt WAL record. The code alone does not establish whether later records are recoverable.",
        ("internal/persistence/recovery_test.go::TestRecoveryRejectsCorruptWAL", "internal/persistence/recovery_test.go::TestWALV2RejectsCRCFailure"),
        "Preserve the tenant directory and logs. Do not truncate, rewrite, or restore over live data automatically.",
    ),
    "DBX_VECTOR_GRAPH_REBUILD": Case(
        "DBX-ENG-007", "Vector graph rebuild after restart",
        "DBX rebuilds the in-memory search graph from persisted vector rows during open; validate search results after recovery.",
        ("internal/engine/vector_test.go::TestVectorStoreRebuildsGraphAfterCrash",),
        "The graph rebuild is a DBX engine startup path. The extension cannot invoke it on a running tenant; preserve evidence and validate the deployed build.",
    ),
    "DBX_TENANT_QUOTA_EXCEEDED": Case(
        "DBX-OPS-003", "Tenant write rejected by memory quota",
        "A quota rejection protects tenant and neighbor capacity and should not be bypassed by automatic limit changes.",
        ("internal/query/executor_test.go::TestTenantQuotaRejectsBeforeWAL", "internal/query/executor_surface_test.go::TestNoisyNeighborQuotaDoesNotBlockQuietTenant"),
        "Review workload and capacity. Do not delete tenant keys or raise the quota automatically.",
    ),
}

# Catalogue-only cases identify verified regression evidence. They are not
# claims that every build emits these codes or that snapshots detect them.
CASES.update({
    "DBX_TENANT_TASK_PANIC": Case(
        "DBX-OPS-015", "Tenant background task panic",
        "A tenant task panicked and DBX marked worker readiness false and notified its supervisor.",
        ("internal/server/recovery_support_test.go::TestRecoverTenantTaskMarksUnhealthyAndSignalsError",
         "internal/query/executor_test.go::TestExecuteIsolatesPanic"),
        "Observe the orchestrator's bounded restart policy; preserve evidence if it exhausts attempts. Never add a second restart loop.",
    ),
    "DBX_VECTOR_INTEGRITY": Case(
        "DBX-ENG-009", "Vector seal or graph mismatch",
        "Vector recovery must reject inconsistent seals and validate rebuilt graph search results.",
        ("internal/query/executor_test.go::TestRecoveryRejectsMismatchedVectorSeals",
         "internal/engine/vector_test.go::TestVectorStoreRebuildsCorruptOrMismatchedGraph",
         "internal/engine/vector_test.go::TestVectorSearchRecallMatchesBruteForce"),
        "Preserve vector rows and seals; validate deployed build and recall without rewriting live indexes speculatively.",
    ),
    "DBX_VECTOR_MIGRATION": Case(
        "DBX-ENG-010", "Vector migration interrupted",
        "Recovery must restore the correct live index and in-progress shadow after swap or cancellation.",
        ("internal/query/executor_test.go::TestVMIGRATEDurablePromotesAcrossRecovery",
         "internal/query/executor_test.go::TestVMIGRATEDurableRestoresInProgressShadow",
         "internal/query/executor_test.go::TestVMIGRATEDurableCancelAcrossRecovery"),
        "Preserve live and shadow stores and inspect migration state; never repeat swap or delete a shadow without verified state.",
    ),
    "DBX_TENANT_UNHEALTHY": Case(
        "DBX-OPS-009", "Running tenant fails health",
        "Lifecycle and health disagree; waking an already running engine is inappropriate.",
        ("internal/orchestrator/manager_test.go::TestListTenantViewsReportsRunningAndDown",),
        "Retain worker evidence and stop wake attempts for this tenant until its lifecycle is known.",
    ),
    "DBX_TENANT_MEMORY_EXHAUSTED": Case(
        "DBX-OPS-010", "Memory quota exhausted",
        "Memory is at or above the quota; further writes may be rejected before WAL append.",
        ("internal/query/executor_test.go::TestTenantQuotaRejectsBeforeWAL",),
        "Respect quota rejections. Capacity or workload changes require an operator decision.",
    ),
    "DBX_TENANT_STATUS_UNKNOWN": Case(
        "DBX-OPS-011", "Unrecognized lifecycle state",
        "The deployed API exposes a lifecycle value absent from this support release.",
        ("support-extension/tests/test_campaign.py::DiagnosticBoundaryTests.test_lifecycle_and_counter_transitions",),
        "Hold lifecycle actions; check version compatibility without echoing arbitrary status text.",
    ),
    "DBX_ERROR_COUNTER_RESET": Case(
        "DBX-OPS-012", "Error counter reset",
        "A reset can follow restart or replacement; it does not prove a repair succeeded.",
        ("support-extension/tests/test_campaign.py::DiagnosticBoundaryTests.test_lifecycle_and_counter_transitions",),
        "Establish a new counter baseline and compare the following interval.",
    ),
    "DBX_USAGE_TENANT_UNLISTED": Case(
        "DBX-OPS-013", "Usage references an unlisted tenant",
        "Inventory and usage do not describe the same tenant set.",
        ("support-extension/tests/test_campaign.py::DiagnosticBoundaryTests.test_lifecycle_and_counter_transitions",),
        "Repeat read-only diagnostics; hold recovery while snapshots disagree.",
    ),
    "DBX_LIFECYCLE_INCONSISTENT": Case(
        "DBX-OPS-014", "Conflicting lifecycle evidence",
        "Tenant and usage state, hibernation flags, or health are inconsistent.",
        ("support-extension/tests/test_campaign.py::RecoveryCampaignTests.test_campaign_has_no_unsafe_actions",),
        "Hold recovery and obtain a consistent snapshot; never select a favorable conflicting row.",
    ),
    "DBX_SUPPORT_SNAPSHOT_INVALID": Case(
        "DBX-SUP-001", "Malformed support telemetry",
        "Missing collections, duplicate identities, invalid counters, or unexpected field types invalidate diagnostics.",
        ("support-extension/tests/test_campaign.py::DiagnosticBoundaryTests.test_malformed_snapshots_fail_closed",),
        "Reject the snapshot before specialist checks or any recovery call.",
    ),
    "DBX_SUPPORT_SCOPE_BREACH": Case(
        "DBX-SUP-002", "Support data boundary violation",
        "Unreviewed fields reached the support boundary.",
        ("support-extension/tests/test_support.py::SupportExtensionTests.test_scope_guard_stops_other_specialists_on_unreviewed_fields",),
        "Discard the response and stop automation until the schema boundary is restored.",
    ),
    "DBX_SUPPORT_AGENT_FAILED": Case(
        "DBX-SUP-003", "Diagnostic specialist crashed",
        "The cycle is incomplete even when other specialists succeed.",
        ("support-extension/tests/test_support.py::RecoveryFaultInjectionTests.test_specialist_crash_degrades_diagnostics_and_holds_recovery",),
        "Keep available diagnostics and hold recovery for this cycle; do not expose exception payloads.",
    ),
    "DBX_BACKUP_INVALID": Case(
        "DBX-DUR-001", "Backup integrity or tenant mismatch",
        "A recovery archive must pass integrity and tenant identity checks before restoration.",
        ("internal/persistence/backup_test.go::TestBackupArchiveRoundTrip",
         "internal/persistence/backup_test.go::TestBackupRejectsWrongTenant"),
        "Preserve live data. Restore only a verified, correctly scoped archive under an approved recovery plan.",
    ),
    "DBX_SNAPSHOT_RECOVERY": Case(
        "DBX-DUR-002", "Snapshot recovery and checkpoint selection",
        "Recovery must select the committed checkpoint rather than the most recently modified file.",
        ("internal/persistence/snapshot_test.go::TestSnapshotSaveRestoreRoundTrip",
         "internal/persistence/snapshot_test.go::TestSnapshotCurrentPointerSurvivesNewerMtime",
         "internal/persistence/recovery_test.go::TestWALV2TransactionAndCheckpointRecovery"),
        "Preserve snapshot and WAL files; never overwrite current pointers speculatively.",
    ),
    "DBX_REPLICATION_INTEGRITY": Case(
        "DBX-REP-001", "Replication tampering or replay",
        "Replica streams must reject forged, replayed, or oversized frames without applying them.",
        ("internal/replication/stream_test.go::TestReplicaRejectsTamperedFrame",
         "internal/replication/stream_test.go::TestReplicaRejectsReplayedFrame",
         "internal/replication/stream_test.go::TestReplicaStreamRejectsOversizedFrame"),
        "Stop failover automation and preserve evidence; do not promote a replica of unknown integrity.",
    ),
    "DBX_REPLICATION_AUTH": Case(
        "DBX-REP-002", "Replication authentication failure",
        "Peers with missing or wrong credentials must receive no tenant stream.",
        ("internal/replication/handshake_test.go::TestPrimaryStreamsNothingToUnauthenticatedPeer",
         "internal/replication/handshake_test.go::TestReplicaWithWrongTokenReceivesNothing",
         "internal/replication/handshake_test.go::TestReplicaRejectsImpostorPrimary"),
        "Hold promotion and verify peer identity and scoped configuration; do not disable authentication.",
    ),
    "DBX_ENCRYPTION_KEY_MISMATCH": Case(
        "DBX-SEC-001", "Encrypted engine cannot open with supplied key",
        "An encrypted tenant must reject an incorrect key while preserving its data.",
        ("internal/server/isolation_test.go::TestSealedEngineRefusesWrongKey",
         "internal/isolation/envelope_test.go::TestEnvelopeRoundTripAndShred"),
        "Retain encrypted files and restore the authorized key through the operator's secret manager.",
    ),
    "DBX_ISOLATION_FAILURE": Case(
        "DBX-SEC-002", "Tenant isolation policy failure",
        "Production must reject insufficient isolation and Linux workers must enforce filesystem and socket restrictions.",
        ("internal/isolation/enforce_test.go::TestEnforceRefusesInprocessInProduction",
         "internal/isolation/landlock_linux_test.go::TestRestrictFilesystemBlocksSibling",
         "internal/isolation/lockdown_linux_test.go::TestLockDownAllowsOnlyUnixSockets"),
        "Stop automation and preserve evidence. Never downgrade isolation to make a health check pass.",
    ),
    "DBX_PROTOCOL_MALFORMED": Case(
        "DBX-PRO-001", "Malformed protocol request",
        "RESP parsing must reject invalid terminators while supporting pipelining and binary payloads.",
        ("internal/protocol/parser_test.go::TestRESPParserRejectsMalformedTerminator",
         "internal/protocol/parser_test.go::TestRESPParserPipeliningAndBinaryPayload"),
        "Reject invalid requests and correct the client encoding; a tenant restart is unnecessary.",
    ),
    "DBX_CONFIGURATION_INVALID": Case(
        "DBX-CFG-001", "Unsafe engine or replication configuration",
        "Invalid replication modes and missing replication credentials should fail validation.",
        ("internal/config/validator_test.go::TestValidateRejectsReplicationWithoutToken",
         "internal/config/validator_test.go::TestValidateRejectsDataPlaneRaft"),
        "Keep the rejected configuration and apply a reviewed correction; never disable validation.",
    ),
})

# Verified historical regression record from DBX's source documentation and test.
HISTORICAL_CASES: tuple[Case, ...] = (
    Case(
        "DBX-ENG-007-HISTORY", "Vector graph missing after restart",
        "A prior recovery defect reopened vector rows and ids without a searchable HNSW graph; search could return no results after restart.",
        ("internal/engine/vector_test.go::TestVectorStoreRebuildsGraphAfterCrash",),
        "Known fix: rebuild the graph from persisted vector rows on open. On a live node, preserve data and verify the deployed build before considering recovery.",
    ),
)


def case_for(code: str) -> Case | None:
    return CASES.get(code)


def all_cases() -> tuple[Case, ...]:
    return tuple(CASES.values()) + HISTORICAL_CASES
