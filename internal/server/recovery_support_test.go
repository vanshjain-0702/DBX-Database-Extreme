package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dbx/dbx/internal/observability"
)

func TestRecoverTenantTaskMarksUnhealthyAndSignalsError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "panic.jsonl")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	previousStderr := os.Stderr
	os.Stderr = logFile
	logger := observability.NewLogger("error", "json")
	os.Stderr = previousStderr
	defer logFile.Close()
	instance := &Instance{logger: logger, metrics: &observability.Metrics{}, serverErr: make(chan error, 1)}
	instance.metrics.TenantReady.Store(1)
	crash := func() {
		defer instance.recoverTenantTask("synthetic-task")
		panic("private-payload-must-not-reach-support-log")
	}
	crash()
	// A second task panic must not block when the supervisor queue is full.
	crash()
	if instance.metrics.TenantReady.Load() != 0 {
		t.Fatal("panicked worker is still ready")
	}
	select {
	case failure := <-instance.serverErr:
		if !strings.Contains(failure.Error(), "synthetic-task") {
			t.Fatalf("missing task identity: %v", failure)
		}
	default:
		t.Fatal("supervisor did not receive task failure")
	}
	if err := logFile.Sync(); err != nil {
		t.Fatal(err)
	}
	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logs), "private-payload") {
		t.Fatal("panic payload leaked into structured support logs")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(logs)), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["code"] != "DBX_TENANT_TASK_PANIC" {
			t.Fatalf("missing stable panic code: %v", event)
		}
	}
}
