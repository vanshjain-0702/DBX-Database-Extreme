package orchestrator

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/isolation"
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
	httpSock := isolation.HTTPSocket(tenant.DataDir)
	useUnix := tenant.DataDir != ""
	if useUnix {
		if _, err := os.Stat(httpSock); err != nil {
			useUnix = false
		}
	}
	cacheKey := tenant.ID + ":" + strconv.Itoa(tenant.HTTPPort)
	if useUnix {
		cacheKey = tenant.ID + ":unix:" + httpSock
	}
	if cached, ok := p.proxies.Load(cacheKey); ok {
		return cached.(*httputil.ReverseProxy)
	}

	targetURL, _ := url.Parse("http://127.0.0.1")
	if !useUnix {
		targetURL, _ = url.Parse(fmt.Sprintf("http://127.0.0.1:%d", tenant.HTTPPort))
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	transport := &http.Transport{
		MaxIdleConns:          10000,
		MaxIdleConnsPerHost:   1000,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if useUnix {
		sock := httpSock
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		}
	}
	proxy.Transport = transport

	originalDirector := proxy.Director
	tenantID := tenant.ID
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Never trust a caller-provided internal token. The proxy injects
		// the service credential for the engine on every request. A sandboxed
		// worker has its own token, resolved per request so a worker restart
		// does not leave the cached proxy holding a stale one.
		req.Header.Del("X-DBX-Internal-Token")
		token := p.internalAPIToken
		if workerToken, ok := p.manager.WorkerToken(tenantID); ok {
			token = workerToken
		}
		req.Header.Set("X-DBX-Internal-Token", token)
	}

	p.proxies.Store(cacheKey, proxy)
	proxy.ModifyResponse = rewriteForeignTenantForbidden
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error": "tenant unavailable"}`))
	}
	return proxy
}

// rewriteForeignTenantForbidden maps engine 403s (wrong internal token / foreign
// process bound to the tenant port) to 502 so the control plane can treat them
// as down instead of leaking an auth error to the dashboard.
func rewriteForeignTenantForbidden(resp *http.Response) error {
	if resp.StatusCode != http.StatusForbidden {
		return nil
	}
	if resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	const body = `{"error":"tenant unavailable"}`
	resp.StatusCode = http.StatusBadGateway
	resp.Status = "502 Bad Gateway"
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Www-Authenticate")
	resp.Body = io.NopCloser(strings.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		if !p.manager.TenantRunning(tenantID) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error": "tenant unavailable"}`))
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
