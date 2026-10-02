package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	supportReadTestToken = "support-read-token-with-at-least-32-bytes"
	supportWakeTestToken = "support-wake-token-with-at-least-32-bytes"
)

func TestNewSupportAPIRejectsUnsafeCapabilityConfiguration(t *testing.T) {
	tests := []struct {
		name, readToken, wakeToken, readTenants, wakeTenants string
	}{
		{"short read token", "short", "", "acme", ""},
		{"short wake token", supportReadTestToken, "short", "acme", "acme"},
		{"same capabilities", supportReadTestToken, supportReadTestToken, "acme", "acme"},
		{"wake without read", "", supportWakeTestToken, "", "acme"},
		{"wake tenant not readable", supportReadTestToken, supportWakeTestToken, "acme", "globex"},
		{"unsafe tenant id", supportReadTestToken, "", "../acme", ""},
		{"duplicate tenant id", supportReadTestToken, "", "acme,acme", ""},
		{"dot tenant id", supportReadTestToken, "", ".", ""},
		{"dotdot tenant id", supportReadTestToken, "", "..", ""},
		{"token whitespace", supportReadTestToken + "\n", "", "acme", ""},
		{"missing read scope", supportReadTestToken, "", "", ""},
		{"missing wake scope", supportReadTestToken, supportWakeTestToken, "acme", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewSupportAPI(nil, test.readToken, test.wakeToken, test.readTenants, test.wakeTenants, ""); err == nil {
				t.Fatal("expected invalid support configuration to be rejected")
			}
		})
	}
}

func TestSupportCapabilitiesMethodsAndUnavailableManager(t *testing.T) {
	manager, _, _ := newTestManager(t)
	api, err := NewSupportAPI(manager, supportReadTestToken, supportWakeTestToken, "acme", "acme", filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, path, token string
		status                    int
	}{
		{"snapshot write denied", http.MethodPost, "/api/support/v1/snapshot", supportReadTestToken, http.StatusMethodNotAllowed},
		{"wake read denied", http.MethodGet, "/api/support/v1/tenants/acme/wake", supportWakeTestToken, http.StatusMethodNotAllowed},
		{"wake token cannot read", http.MethodGet, "/api/support/v1/snapshot", supportWakeTestToken, http.StatusUnauthorized},
		{"read token cannot wake", http.MethodPost, "/api/support/v1/tenants/acme/wake", supportReadTestToken, http.StatusUnauthorized},
		{"invalid wake suffix", http.MethodPost, "/api/support/v1/tenants/acme/restart", supportWakeTestToken, http.StatusNotFound},
		{"missing tenant path", http.MethodPost, "/api/support/v1/tenants/wake", supportWakeTestToken, http.StatusNotFound},
		{"unknown tenant denied", http.MethodPost, "/api/support/v1/tenants/other/wake", supportWakeTestToken, http.StatusForbidden},
		{"running or down tenant denied", http.MethodPost, "/api/support/v1/tenants/acme/wake", supportWakeTestToken, http.StatusConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, nil)
			req.Header.Set("Authorization", "Bearer "+test.token)
			response := httptest.NewRecorder()
			if strings.HasSuffix(test.path, "snapshot") {
				api.Snapshot(response, req)
			} else {
				api.Wake(response, req)
			}
			if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
	api.manager = nil
	for _, read := range []bool{true, false} {
		method, path, token := http.MethodPost, "/api/support/v1/tenants/acme/wake", supportWakeTestToken
		if read {
			method, path, token = http.MethodGet, "/api/support/v1/snapshot", supportReadTestToken
		}
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		if read {
			api.Snapshot(response, req)
		} else {
			api.Wake(response, req)
		}
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("missing manager response = %d", response.Code)
		}
	}
}

func TestSupportSnapshotRedactsOutOfScopeReplicationRelationships(t *testing.T) {
	manager, acme, _ := newTestManager(t)
	acme.ReplicaOf = "globex"
	acme.Replicas = []string{"globex", "acme"}
	api, err := NewSupportAPI(manager, supportReadTestToken, "", "acme", "", "")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/support/v1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+supportReadTestToken)
	response := httptest.NewRecorder()
	api.Snapshot(response, request)
	if strings.Contains(response.Body.String(), "globex") {
		t.Fatalf("outside-scope relationship leaked: %s", response.Body.String())
	}
	if acme.ReplicaOf != "globex" || len(acme.Replicas) != 2 {
		t.Fatal("snapshot redaction changed tenant state")
	}
}

func TestSupportConcurrentWakeReservesOneAuditedAttempt(t *testing.T) {
	manager, acme, _ := newTestManager(t)
	acme.Hibernated = true
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("blocks persistence"), 0600); err != nil {
		t.Fatal(err)
	}
	manager.stateFile = filepath.Join(blocker, "state.json")
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	api, err := NewSupportAPI(manager, supportReadTestToken, supportWakeTestToken, "acme", "acme", auditPath)
	if err != nil {
		t.Fatal(err)
	}
	const requests = 20
	statuses := make(chan int, requests)
	var group sync.WaitGroup
	for i := 0; i < requests; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/support/v1/tenants/acme/wake", nil)
			req.Header.Set("Authorization", "Bearer "+supportWakeTestToken)
			response := httptest.NewRecorder()
			api.Wake(response, req)
			statuses <- response.Code
		}()
	}
	group.Wait()
	close(statuses)
	counts := make(map[int]int)
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusConflict] != 1 || counts[http.StatusTooManyRequests] != requests-1 {
		t.Fatalf("concurrent statuses = %v", counts)
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(audit), `"outcome":"attempt"`) != 1 || strings.Count(string(audit), `"outcome":"failed"`) != 1 {
		t.Fatalf("audit = %s", audit)
	}
	if !acme.Hibernated {
		t.Fatal("failed wake changed hibernated state")
	}
}

func TestSupportPythonClientIntegration(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skip("Python is required for the support extension integration test")
	}
	manager, acme, _ := newTestManager(t)
	acme.HTTPPort = freeTCPPort(t)
	acme.RESPPort = freeTCPPort(t)
	acme.Hibernated = true
	defer manager.StopAll()
	api, err := NewSupportAPI(manager, supportReadTestToken, supportWakeTestToken, "acme", "acme", filepath.Join(t.TempDir(), "server-audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/support/v1/snapshot", api.Snapshot)
	mux.HandleFunc("/api/support/v1/tenants/", api.Wake)
	server := httptest.NewServer(mux)
	defer server.Close()
	const script = `
import sys
from pathlib import Path
from dbx_support.client import DBXSupportClient
from dbx_support.recovery import RecoveryManager
from dbx_support.team import SupportTeam
client = DBXSupportClient(sys.argv[1])
client.use_support_capability(sys.argv[2], sys.argv[3])
before = client.snapshot()
assert [row["id"] for row in before["tenants"]] == ["acme"]
assert [row["tenant_id"] for row in before["usage"]] == ["acme"]
assert SupportTeam().run(before).autonomy == "diagnose-only"
recovery = RecoveryManager(client, {"acme"}, Path(sys.argv[4]))
events = recovery.repair(before)
assert len(events) == 1 and events[0].outcome == "recovered", events
assert recovery.repair(before) == []
after = client.snapshot()
assert after["tenants"][0]["status"] == "running"
assert after["tenants"][0]["healthy"] is True
print("Scoped snapshot, audited wake, post-check, and duplicate suppression passed")
`
	command := exec.Command(python, "-c", script, server.URL, supportReadTestToken, supportWakeTestToken, filepath.Join(t.TempDir(), "client-audit.jsonl"))
	command.Dir = filepath.Join("..", "..", "support-extension")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Python support integration failed: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(string(output)))
}

func TestSupportSnapshotRequiresCapabilityAndFiltersTenants(t *testing.T) {
	manager, _, _ := newTestManager(t)
	api, err := NewSupportAPI(manager, supportReadTestToken, "", "acme", "", filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	api.Snapshot(unauthorized, httptest.NewRequest(http.MethodGet, "/api/support/v1/snapshot", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/support/v1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+supportReadTestToken)
	response := httptest.NewRecorder()
	api.Snapshot(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d: %s", response.Code, response.Body.String())
	}
	var snapshot struct {
		Tenants []TenantView  `json:"tenants"`
		Usage   []TenantUsage `json:"usage"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tenants) != 1 || snapshot.Tenants[0].ID != "acme" {
		t.Fatalf("tenant scope leaked or missing: %+v", snapshot.Tenants)
	}
	if len(snapshot.Usage) != 1 || snapshot.Usage[0].TenantID != "acme" {
		t.Fatalf("usage scope leaked or missing: %+v", snapshot.Usage)
	}
}

func TestSupportWakeFailsClosedOnAuditFailureAndOutsideAllowlist(t *testing.T) {
	manager, acme, _ := newTestManager(t)
	acme.Hibernated = true
	blocker := filepath.Join(t.TempDir(), "audit-parent-is-a-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	api, err := NewSupportAPI(manager, supportReadTestToken, supportWakeTestToken, "acme,globex", "acme", filepath.Join(blocker, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	wake := func(tenant string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/support/v1/tenants/"+tenant+"/wake", nil)
		req.Header.Set("Authorization", "Bearer "+supportWakeTestToken)
		response := httptest.NewRecorder()
		api.Wake(response, req)
		return response
	}
	if got := wake("globex"); got.Code != http.StatusForbidden {
		t.Fatalf("outside-allowlist wake status = %d", got.Code)
	}
	if got := wake("acme"); got.Code != http.StatusServiceUnavailable || !strings.Contains(got.Body.String(), "no recovery action") {
		t.Fatalf("audit failure response = %d %s", got.Code, got.Body.String())
	}
	if !acme.Hibernated {
		t.Fatal("tenant was woken despite audit failure")
	}

	// Repair the audit destination but make state persistence fail, so the
	// second request proves it passed cooldown and still failed safely.
	api.auditPath = filepath.Join(t.TempDir(), "audit.jsonl")
	manager.stateFile = filepath.Join(blocker, "state.json")
	if got := wake("acme"); got.Code != http.StatusConflict {
		t.Fatalf("retry after audit recovery = %d %s; want state persistence failure", got.Code, got.Body.String())
	}
	if !acme.Hibernated {
		t.Fatal("tenant lost hibernated state after failed persistence")
	}
}
