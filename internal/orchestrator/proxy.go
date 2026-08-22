package orchestrator

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Proxy struct {
	manager          *Manager
	ring             *HashRing
	internalAPIToken string
	proxies          sync.Map // tenantID -> *httputil.ReverseProxy
}

func NewProxy(manager *Manager, internalAPIToken string) *Proxy {
	// Initialize a Hash Ring with 100 virtual nodes for even distribution
	ring := NewHashRing(100)
	return &Proxy{
		manager:          manager,
		ring:             ring,
		internalAPIToken: internalAPIToken,
	}
}

func (p *Proxy) getTenantProxy(tenant *Tenant) *httputil.ReverseProxy {
	if cached, ok := p.proxies.Load(tenant.ID); ok {
		return cached.(*httputil.ReverseProxy)
	}

	targetAddr := fmt.Sprintf("http://127.0.0.1:%d", tenant.HTTPPort)
	targetURL, _ := url.Parse(targetAddr)

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Configure connection pooling to prevent TCP socket exhaustion
	proxy.Transport = &http.Transport{
		MaxIdleConns:          10000,
		MaxIdleConnsPerHost:   1000,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Never trust a caller-provided internal token. The proxy injects
		// the service credential for the engine on every request.
		req.Header.Del("X-DBX-Internal-Token")
		req.Header.Set("X-DBX-Internal-Token", p.internalAPIToken)
	}

	p.proxies.Store(tenant.ID, proxy)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error": "tenant unavailable"}`))
	}
	return proxy
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for dashboard
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Path: /t/<tenant_id>/<endpoint>
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 3)

	if len(parts) >= 2 && parts[0] == "t" {
		tenantID := parts[1]
		tenant, ok := p.manager.GetTenant(tenantID)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "tenant not found"}`))
			return
		}

		// Strip /t/<tenant_id> from the request path so the backend sees /query
		r.URL.Path = "/"
		if len(parts) == 3 {
			r.URL.Path = "/" + parts[2]
		}

		proxy := p.getTenantProxy(tenant)
		proxy.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"error": "not found"}`))
}
