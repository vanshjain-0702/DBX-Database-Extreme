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

	manager := &Manager{
		tenants: map[string]*Tenant{
			"tenant-a": {ID: "tenant-a", HTTPPort: port},
		},
		instances: map[string]*server.Instance{
			"tenant-a": {},
		},
	}
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

func TestProxyRejectsUnknownTenant(t *testing.T) {
	proxy := NewProxy(&Manager{tenants: map[string]*Tenant{}}, "internal-secret")
	request := httptest.NewRequest(http.MethodGet, "/t/missing/metrics", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestProxyUnavailableWhenEngineNotRunning(t *testing.T) {
	manager := &Manager{
		tenants:   map[string]*Tenant{"ghost": {ID: "ghost", HTTPPort: 8099}},
		instances: map[string]*server.Instance{},
	}
	proxy := NewProxy(manager, "internal-secret")
	request := httptest.NewRequest(http.MethodGet, "/t/ghost/metrics", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestProxyInjectsPerWorkerToken(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-DBX-Internal-Token"); got != "worker-secret" {
			t.Fatalf("internal token = %q, want worker-secret", got)
		}
		fmt.Fprint(w, "ok")
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

	manager := &Manager{
		tenants: map[string]*Tenant{
			"tenant-a": {ID: "tenant-a", HTTPPort: port},
		},
		workers: map[string]*isolatedWorker{
			"tenant-a": {token: "worker-secret"},
		},
	}
	proxy := NewProxy(manager, "orchestrator-secret")
	request := httptest.NewRequest(http.MethodGet, "/t/tenant-a/info", nil)
	response := httptest.NewRecorder()

	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "ok" {
		t.Fatalf("body = %q", response.Body.String())
	}
}

func TestProxyRewritesBackendForbidden(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "engine HTTP API requires orchestrator authentication", http.StatusForbidden)
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
	manager := &Manager{
		tenants:   map[string]*Tenant{"tenant-a": {ID: "tenant-a", HTTPPort: port}},
		instances: map[string]*server.Instance{"tenant-a": {}},
	}
	proxy := NewProxy(manager, "wrong-token")
	request := httptest.NewRequest(http.MethodGet, "/t/tenant-a/metrics", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
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
