// Package api provides the RESP, HTTP, and gRPC API servers for DBX.
package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dbx/dbx/internal/auth"
	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/query"
	"github.com/dbx/dbx/internal/security"
)

// Client represents an active TCP connection.
type Client struct {
	ID         uint64
	Conn       net.Conn
	RemoteAddr string
	User       *auth.User
	reader     *bufio.Reader
	writer     *protocol.Writer
	createdAt  time.Time
}

// RESPServer is the main TCP RESP server.
type RESPServer struct {
	cfg       *config.ServerConfig
	tlsCfg    *config.TLSConfig
	executor  *query.Executor
	acl       *auth.ACLStore
	enforcer  *security.ACLEnforcer
	rateLimit *security.RateLimiter
	audit     *security.AuditGuard
	metrics   *observability.Metrics
	logger    *observability.Logger

	mu       sync.RWMutex
	clients  map[uint64]*Client
	nextID   uint64
	listener net.Listener
	done     chan struct{}
	connSem  chan struct{}
}

// NewRESPServer creates a new RESP server.
func NewRESPServer(
	cfg *config.ServerConfig,
	tlsCfg *config.TLSConfig,
	executor *query.Executor,
	acl *auth.ACLStore,
	enforcer *security.ACLEnforcer,
	rateLimit *security.RateLimiter,
	audit *security.AuditGuard,
	metrics *observability.Metrics,
	logger *observability.Logger,
) *RESPServer {
	var sem chan struct{}
	if cfg.MaxConnections > 0 {
		sem = make(chan struct{}, cfg.MaxConnections)
	}
	return &RESPServer{
		cfg:       cfg,
		tlsCfg:    tlsCfg,
		executor:  executor,
		acl:       acl,
		enforcer:  enforcer,
		rateLimit: rateLimit,
		audit:     audit,
		metrics:   metrics,
		logger:    logger,
		clients:   make(map[uint64]*Client),
		done:      make(chan struct{}),
		connSem:   sem,
	}
}

// ListenAndServe starts the TCP listener.
func (s *RESPServer) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	if s.cfg.Socket != "" {
		addr = s.cfg.Socket
	}
	var ln net.Listener
	var err error

	if s.tlsCfg != nil && s.tlsCfg.Enabled && s.cfg.Socket == "" {
		cert, err := tls.LoadX509KeyPair(s.tlsCfg.CertFile, s.tlsCfg.KeyFile)
		if err != nil {
			return fmt.Errorf("resp: load key pair: %w", err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}

		if s.tlsCfg.RequireClientCert {
			caCert, err := os.ReadFile(s.tlsCfg.CAFile)
			if err != nil {
				return fmt.Errorf("resp: load CA file: %w", err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)

			tlsConfig.ClientCAs = caCertPool
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}

		ln, err = tls.Listen("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("resp: tls listen %s: %w", addr, err)
		}
		s.logger.Info("DBX RESP server listening on %s (mTLS ENABLED)", addr)
	} else {
		ln, err = isolation.Listen(addr, s.cfg.PeerPIDs)
		if err != nil {
			return fmt.Errorf("resp: listen %s: %w", addr, err)
		}
		if s.cfg.Socket != "" {
			s.logger.Info("DBX RESP server listening on unix %s", addr)
		} else {
			s.logger.Info("DBX RESP server listening on %s (PLAINTEXT)", addr)
		}
	}

	s.mu.Lock()
	select {
	case <-s.done:
		s.mu.Unlock()
		_ = ln.Close()
		return nil
	default:
		s.listener = ln
	}
	s.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			ln.Close()
		case <-s.done:
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				s.logger.Error("accept error: %v", err)
				continue
			}
		}
		go s.handleConn(conn)
	}
}

func (s *RESPServer) handleConn(conn net.Conn) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("connection panic isolated: %v", recovered)
			_ = conn.Close()
		}
	}()
	if s.connSem != nil {
		select {
		case s.connSem <- struct{}{}:
			defer func() { <-s.connSem }()
		default:
			conn.Write([]byte("-ERR max number of clients reached\r\n"))
			conn.Close()
			return
		}
	}

	id := atomic.AddUint64(&s.nextID, 1)
	bw := bufio.NewWriterSize(conn, 32*1024)
	client := &Client{
		ID:         id,
		Conn:       conn,
		RemoteAddr: conn.RemoteAddr().String(),
		User:       s.acl.GetUser("default"), // start as default user
		reader:     bufio.NewReader(conn),
		writer:     protocol.NewWriter(bw),
		createdAt:  time.Now(),
	}

	s.mu.Lock()
	s.clients[id] = client
	s.mu.Unlock()
	s.metrics.ActiveConns.Add(1)

	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.clients, id)
		s.mu.Unlock()
		s.metrics.ActiveConns.Add(-1)
	}()

	parser := protocol.NewRESPParser(conn)
	for {
		cmd, err := parser.ReadCommand()
		if err != nil {
			return
		}
		cmd.ClientID = id

		if !s.rateLimit.Allow(client.RemoteAddr) {
			client.writer.WriteErrorRaw("ERR rate limit exceeded")
			_ = client.writer.Flush()
			continue
		}

		if cmd.Normalized() == "AUTH" {
			s.handleAuth(client, cmd)
			_ = client.writer.Flush()
			continue
		}

		if client.User != nil {
			// Resolve on every command so revocation or role changes apply to
			// already-authenticated connections immediately.
			client.User = s.acl.GetUser(client.User.Name)
		}
		if err := s.enforcer.Enforce(client.User, cmd); err != nil {
			s.audit.Log(security.AuditEvent{
				ClientID:   id,
				UserName:   userNameOrEmpty(client.User),
				Command:    cmd.Name,
				Result:     "denied",
				Reason:     err.Error(),
				RemoteAddr: client.RemoteAddr,
			})
			client.writer.WriteErrorRaw(err.Error())
			_ = client.writer.Flush()
			continue
		}

		info, _ := protocol.Lookup(cmd.Normalized())
		if !info.ReadOnly && protocol.ShouldAudit(cmd.Normalized()) {
			s.audit.Log(security.AuditEvent{
				ClientID:   id,
				UserName:   userNameOrEmpty(client.User),
				Command:    cmd.Name,
				Result:     "ok",
				RemoteAddr: client.RemoteAddr,
			})
		}

		if err := s.executor.Execute(id, cmd, client.writer); err != nil {
			if err.Error() == "quit" {
				_ = client.writer.Flush()
				return
			}
		}
		if parser.Buffered() == 0 {
			_ = client.writer.Flush()
			conn.SetDeadline(time.Now().Add(s.cfg.ReadTimeout))
		}
	}
}

func (s *RESPServer) handleAuth(client *Client, cmd *protocol.Command) {
	if cmd.NumArgs() < 1 {
		client.writer.WriteError(protocol.WrongNumArgsError("AUTH"))
		return
	}
	var username, password string
	if cmd.NumArgs() == 1 {
		username = "default"
		password = cmd.Arg(0)
	} else {
		username = cmd.Arg(0)
		password = cmd.Arg(1)
	}
	user := s.acl.GetUser(username)
	if user == nil {
		client.writer.WriteErrorRaw("WRONGPASS invalid username-password pair")
		return
	}
	if err := auth.Authenticate(user, password); err != nil {
		client.writer.WriteErrorRaw(err.Error())
		return
	}
	client.User = user
	client.writer.WriteOK()
}

// Shutdown gracefully stops the server.
func (s *RESPServer) Shutdown() {
	s.mu.Lock()
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	ln := s.listener
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	s.mu.RLock()
	for _, c := range s.clients {
		c.Conn.Close()
	}
	s.mu.RUnlock()
}

// ActiveConnections returns the count of active connections.
func (s *RESPServer) ActiveConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func userNameOrEmpty(u *auth.User) string {
	if u == nil {
		return "anonymous"
	}
	return u.Name
}

// HTTPServer provides a minimal HTTP API for health checks and metrics.
type HTTPServer struct {
	cfg              *config.ServerConfig
	metrics          *observability.Metrics
	executor         *query.Executor
	internalAPIToken string
	enforcer         *security.ACLEnforcer
	auditGuard       *security.AuditGuard
	backupFn         func(tenantID, outputPath string) (persistence.BackupManifest, error)
	reloadACL        func() error
	tenantID         string
}

// NewHTTPServer creates an HTTP server.
func NewHTTPServer(cfg *config.ServerConfig, metrics *observability.Metrics, executor *query.Executor, internalAPIToken string, enforcer *security.ACLEnforcer, auditGuard *security.AuditGuard) *HTTPServer {
	return &HTTPServer{cfg: cfg, metrics: metrics, executor: executor, internalAPIToken: internalAPIToken, enforcer: enforcer, auditGuard: auditGuard}
}

// SetBackup exposes a maintenance-locked backup endpoint for sandboxed workers.
func (h *HTTPServer) SetBackup(tenantID string, fn func(tenantID, outputPath string) (persistence.BackupManifest, error)) {
	h.tenantID = tenantID
	h.backupFn = fn
}

// SetACLReload lets the orchestrator push credential changes synchronously.
func (h *HTTPServer) SetACLReload(fn func() error) {
	h.reloadACL = fn
}

// withCORS adds CORS headers to allow the dashboard to connect.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && corsOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")
		if r.Method == "OPTIONS" {
			if origin != "" && !corsOriginAllowed(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func corsOriginAllowed(origin string) bool {
	for _, configured := range strings.Split(os.Getenv("DBX_ALLOWED_ORIGINS"), ",") {
		if strings.TrimSpace(configured) == origin {
			return true
		}
	}
	return loopbackDashboardOrigin(origin)
}

func loopbackDashboardOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func recoverHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				http.Error(w, "internal request failure", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// internalAPIOnly protects engine control endpoints with a shared secret. It
// works whether the orchestrator is colocated or deployed in another pod.
func (h *HTTPServer) internalAPIOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-DBX-Internal-Token")
		if h.internalAPIToken == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(h.internalAPIToken)) != 1 {
			http.Error(w, "engine HTTP API requires orchestrator authentication", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// ListenAndServe starts the HTTP server.
func (h *HTTPServer) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/keyspace", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := h.executor.KV().KeyspaceStats()
		json.NewEncoder(w).Encode(stats)
	})))

	mux.HandleFunc("/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if h.metrics.TenantReady.Load() == 0 {
			http.Error(w, `{"status":"not_ready"}`, http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	mux.HandleFunc("/metrics", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snap := h.metrics.Snapshot()
		fmt.Fprintf(w, `{`)
		first := true
		for k, v := range snap {
			if !first {
				fmt.Fprintf(w, `,`)
			}
			fmt.Fprintf(w, `"%s":%d`, k, v)
			first = false
		}
		fmt.Fprintf(w, `}`)
	})))
	mux.HandleFunc("/usage", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(h.executorUsage())
	})))
	mux.HandleFunc("/internal/acl/reload", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if h.reloadACL == nil {
			http.Error(w, "acl reload unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := h.reloadACL(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})))
	mux.HandleFunc("/internal/backup", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if h.backupFn == nil {
			http.Error(w, "backup unavailable", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Output string `json:"output"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Output == "" {
			http.Error(w, "output path required", http.StatusBadRequest)
			return
		}
		manifest, err := h.backupFn(h.tenantID, req.Output)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	})))
	mux.HandleFunc("/metrics/prometheus", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var buf strings.Builder
		observability.WritePrometheus(&buf, "", h.metrics.Snapshot())
		w.Write([]byte(buf.String()))
	})))
	mux.HandleFunc("/info", withCORS(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":"1.1.0","engine":"DBX"}`)
	}))
	mux.HandleFunc("/vaddbin", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		dim := r.URL.Query().Get("dim")
		if key == "" || dim == "" {
			http.Error(w, "Missing key or dim query parameters", http.StatusBadRequest)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 8*1024*1024)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cmd := &protocol.Command{Name: "VADDBIN", Args: [][]byte{[]byte(key), []byte(dim), body}}
		var buf bytes.Buffer
		writer := protocol.NewWriter(&buf)

		if err := h.executor.Execute(0, cmd, writer); err != nil {
			if buf.Len() == 0 {
				writer.WriteErrorRaw(err.Error())
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(buf.Bytes())
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	})))
	mux.HandleFunc("/query", withCORS(h.internalAPIOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Enforce maximum payload size (512KB) to prevent DoS attacks
		r.Body = http.MaxBytesReader(w, r.Body, 512*1024)

		var req struct {
			Command []string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if err.Error() == "http: request body too large" {
				http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.Command) == 0 {
			http.Error(w, "empty command", http.StatusBadRequest)
			return
		}
		args := make([][]byte, len(req.Command)-1)
		for i, arg := range req.Command[1:] {
			args[i] = []byte(arg)
		}
		cmd := &protocol.Command{Name: req.Command[0], Args: args}
		var buf bytes.Buffer
		writer := protocol.NewWriter(&buf)

		// For internal HTTP API proxy requests, we assume it's acting on behalf of the internal dashboard.
		// We execute the command under the identity of "dashboard_internal" to ensure
		// it is subjected to ACL rules and written to the audit log.
		dummyUser := &auth.User{
			Name:        "dashboard_internal",
			Enabled:     true,
			Permissions: auth.PermAll,
			AllowedKeys: []string{"*"},
		}

		if err := h.enforcer.Enforce(dummyUser, cmd); err != nil {
			if buf.Len() == 0 {
				writer.WriteErrorRaw(err.Error())
			}
		} else {
			// Write to audit log
			if h.auditGuard != nil && protocol.ShouldAudit(cmd.Normalized()) {
				clientIP := r.RemoteAddr
				if host, _, err := net.SplitHostPort(clientIP); err == nil {
					clientIP = host
				}
				h.auditGuard.Log(security.AuditEvent{
					UserName:   dummyUser.Name,
					RemoteAddr: clientIP,
					Command:    cmd.Name,
					Result:     "ok",
				})
			}

			// Execute command
			if err := h.executor.Execute(0, cmd, writer); err != nil {
				if buf.Len() == 0 {
					writer.WriteErrorRaw(err.Error())
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"response": buf.String(),
		})
	})))

	addr := fmt.Sprintf("%s:%d", h.cfg.Host, h.cfg.HTTPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           recoverHTTP(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       h.cfg.ReadTimeout,
		WriteTimeout:      h.cfg.WriteTimeout,
	}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	if h.cfg.HTTPSocket != "" {
		ln, err := isolation.Listen(h.cfg.HTTPSocket, h.cfg.PeerPIDs)
		if err != nil {
			return err
		}
		return srv.Serve(ln)
	}
	return srv.ListenAndServe()
}

func (h *HTTPServer) executorUsage() map[string]int64 {
	out := map[string]int64{}
	if h.executor != nil {
		if kv := h.executor.KV(); kv != nil {
			out["keys"] = kv.KeyCount()
		}
		if vec := h.executor.Vectors(); vec != nil {
			out["vectors"] = vec.LiveVectorCount()
		}
		out["memory_used_bytes"] = h.executor.MemoryUsage()
	}
	if h.metrics != nil {
		snap := h.metrics.Snapshot()
		out["memory_limit_bytes"] = snap["tenant_memory_limit_bytes"]
		out["commands"] = snap["total_commands"]
		out["errors"] = snap["total_errors"]
		out["avg_latency_ns"] = snap["avg_latency_ns"]
		out["ready"] = snap["tenant_ready"]
	}
	return out
}
