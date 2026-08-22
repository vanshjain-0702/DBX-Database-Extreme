package orchestrator

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dbx/dbx/internal/server"
)

func TestProxyRoutesWithinTenantAndInjectsCredential(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-DBX-Internal-Token"); got != "internal-secret" {
			t.Fatalf("internal token = %q", got)
		}
		fmt.Fprint(w, "tenant-a")
	}))
	defer backend.Close()

	_, portText, err := net.SplitHostPort(backend.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	manager := &Manager{tenants: map[string]*Tenant{
		"tenant-a": {ID: "tenant-a", HTTPPort: port},
	}}
	proxy := NewProxy(manager, "internal-secret")
	request := httptest.NewRequest(http.MethodGet, "/t/tenant-a/info", nil)
	response := httptest.NewRecorder()

	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "tenant-a" {
		t.Fatalf("body = %q", response.Body.String())
	}
}

func TestStartTenantSkipsConcurrentStart(t *testing.T) {
	manager := &Manager{
		instances: make(map[string]*server.Instance),
		starting:  map[string]bool{"tenant-a": true},
	}
	if err := manager.StartTenant(&Tenant{ID: "tenant-a"}); err != nil {
		t.Fatalf("concurrent start returned error: %v", err)
	}
}
