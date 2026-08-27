package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dbx/dbx/dashboard"
	"github.com/dbx/dbx/internal/orchestrator"
)

func main() {
	httpPort := flag.Int("port", 8000, "HTTP API port")
	respAddr := flag.String("resp-addr", ":6380", "public authenticated RESP ingress address")
	tlsCert := flag.String("tls-cert", "certs/server.crt", "TLS certificate for the control-plane API")
	tlsKey := flag.String("tls-key", "certs/server.key", "TLS private key for the control-plane API")
	insecureHTTP := flag.Bool("insecure-http", false, "Disable TLS for local development only")
	stateFile := flag.String("state-file", "data/tenants.json", "tenant state file")
	adminFile := flag.String("admin-file", "data/admin.json", "admin credential file")
	flag.Parse()

	type Secrets struct {
		JWTSecret     string `json:"jwt_secret"`
		InternalToken string `json:"internal_token"`
	}
	var secrets Secrets
	secretsPath := "data/secrets.json"
	if b, err := os.ReadFile(secretsPath); err == nil {
		json.Unmarshal(b, &secrets)
	}

	jwtSecret := []byte(os.Getenv("DBX_JWT_SECRET"))
	if len(jwtSecret) < 32 {
		if len(secrets.JWTSecret) >= 32 {
			jwtSecret = []byte(secrets.JWTSecret)
		} else {
			log.Println("WARNING: DBX_JWT_SECRET is missing or weak, generating new secret")
			jwtSecret = make([]byte, 32)
			rand.Read(jwtSecret)
		}
	}

	adminPassword := os.Getenv("DBX_ADMIN_PASSWORD")
	if len(adminPassword) < 12 {
		log.Fatal("DBX_ADMIN_PASSWORD must be set to at least 12 characters")
	}

	internalToken := os.Getenv("DBX_INTERNAL_API_TOKEN")
	if internalToken == "" {
		if secrets.InternalToken != "" {
			internalToken = secrets.InternalToken
		} else {
			log.Println("WARNING: DBX_INTERNAL_API_TOKEN is missing, generating new token")
			tokenBytes := make([]byte, 32)
			rand.Read(tokenBytes)
			internalToken = fmt.Sprintf("%x", tokenBytes)
		}
		os.Setenv("DBX_INTERNAL_API_TOKEN", internalToken)
	}

	secrets.JWTSecret = string(jwtSecret)
	secrets.InternalToken = internalToken
	if b, err := json.MarshalIndent(secrets, "", "  "); err == nil {
		os.MkdirAll("data", 0755)
		os.WriteFile(secretsPath, b, 0600)
	}

	manager, err := orchestrator.NewManager(*stateFile)
	if err != nil {
		log.Fatalf("Failed to init manager: %v", err)
	}

	adminStore, err := orchestrator.NewAdminStore(*adminFile, adminPassword)
	if err != nil {
		log.Fatalf("Failed to init admin store: %v", err)
	}

	proxy := orchestrator.NewProxy(manager, os.Getenv("DBX_INTERNAL_API_TOKEN"))

	mux := http.NewServeMux()
	var loginMu sync.Mutex
	loginFailures := make(map[string]struct {
		count int
		until time.Time
	})

	// Login API (Unprotected)
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		client := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			client = strings.Split(xff, ",")[0]
		} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			client = realIP
		} else if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			client = host
		}
		client = strings.TrimSpace(client)
		loginMu.Lock()
		state := loginFailures[client]
		blocked := time.Now().Before(state.until)
		loginMu.Unlock()
		if blocked {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too many login attempts", http.StatusTooManyRequests)
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		if adminStore.VerifyPassword(req.Username, req.Password) {
			loginMu.Lock()
			delete(loginFailures, client)
			loginMu.Unlock()
			token, err := orchestrator.GenerateToken(req.Username, jwtSecret)
			if err != nil {
				http.Error(w, "Failed to generate token", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"token": token})
		} else {
			loginMu.Lock()
			state := loginFailures[client]
			state.count++
			if state.count >= 5 {
				state.count = 0
				state.until = time.Now().Add(time.Minute)
			}
			loginFailures[client] = state
			loginMu.Unlock()
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		}
	})

	// Protected Routes Mux
	protectedMux := http.NewServeMux()

	// Provisioning API
	protectedMux.HandleFunc("/api/provision", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Replicas int    `json:"replicas"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		t, err := manager.Provision(req.ID, req.Name, req.Replicas)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(t)
	})

	// List API
	protectedMux.HandleFunc("/api/tenants", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(manager.ListTenantViews())
	})

	protectedMux.HandleFunc("/api/tenants/promote", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ReplicaID string `json:"replica_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := manager.Promote(req.ReplicaID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "promoted", "replica_id": req.ReplicaID})
	})

	// Backup API
	protectedMux.HandleFunc("/api/tenants/backup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		path, manifest, err := manager.BackupTenant(req.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success", "path": path, "manifest": manifest,
		})
	})

	protectedMux.HandleFunc("/api/tenants/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := manager.RestoreTenant(req.ID, req.Path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "restored"})
	})

	// Off-boarding API: removes one tenant without touching any other tenant's data.
	protectedMux.HandleFunc("/api/tenants/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID    string `json:"id"`
			Purge bool   `json:"purge"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, ok := manager.GetTenant(req.ID); !ok {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}
		if err := manager.DeleteTenant(req.ID, req.Purge); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "deleted",
			"id":     req.ID,
			"purged": req.Purge,
		})
	})

	// Password Change API
	protectedMux.HandleFunc("/api/admin/password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if !adminStore.VerifyPassword("admin", req.OldPassword) {
			http.Error(w, "Incorrect old password", http.StatusUnauthorized)
			return
		}
		if err := adminStore.UpdatePassword("admin", req.NewPassword); err != nil {
			http.Error(w, "Failed to update password", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})

	// API Keys Management
	protectedMux.HandleFunc("/api/admin/keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			keys := adminStore.ListAPIKeys()
			json.NewEncoder(w).Encode(keys)
		case http.MethodPost:
			var req struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			if req.Name == "" {
				http.Error(w, "Name is required", http.StatusBadRequest)
				return
			}
			fullKey, keyObj, err := adminStore.GenerateAPIKey(req.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Return the full secret key only once!
			json.NewEncoder(w).Encode(map[string]interface{}{
				"api_key":  fullKey,
				"key_info": keyObj,
			})
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	protectedMux.HandleFunc("/api/admin/keys/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := adminStore.RevokeAPIKey(req.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})

	protectedMux.HandleFunc("/api/v1/tenants/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "tenants" || parts[4] != "keys" {
			http.NotFound(w, r)
			return
		}
		tenantID := parts[3]
		switch {
		case r.Method == http.MethodGet && len(parts) == 5:
			keys, err := manager.ListTenantKeys(tenantID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(keys)
		case r.Method == http.MethodPost && len(parts) == 5:
			var req struct {
				Name        string   `json:"name"`
				Role        string   `json:"role"`
				KeyPatterns []string `json:"key_patterns"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			secret, key, err := manager.CreateTenantKey(tenantID, req.Name, req.Role, req.KeyPatterns)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"secret": secret, "key": key})
		case r.Method == http.MethodDelete && len(parts) == 6:
			if err := manager.RevokeTenantKey(tenantID, parts[5]); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Data Plane proxy
	protectedMux.Handle("/t/", proxy)

	// Apply middleware to protected routes
	mux.Handle("/api/provision", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants/promote", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants/backup", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants/restore", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants/delete", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/admin/password", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/admin/keys", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/admin/keys/revoke", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/v1/tenants/", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/t/", orchestrator.RequireAuth(jwtSecret, protectedMux))

	// Static Dashboard Serving (SPA Handler)
	subFS, err := fs.Sub(dashboard.DistFS, "dist")
	if err != nil {
		log.Fatalf("Failed to load embedded dashboard: %v", err)
	}
	fileServer := http.FileServer(http.FS(subFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := subFS.Open(path)
		if err != nil {
			// fallback to index.html for SPA routing
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	})

	fmt.Printf("Control Plane Orchestrator running on :%d\n", *httpPort)
	server := &http.Server{Addr: fmt.Sprintf(":%d", *httpPort), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	serverErr := make(chan error, 1)
	go func() {
		if *insecureHTTP {
			log.Println("WARNING: control-plane TLS disabled by -insecure-http")
			serverErr <- server.ListenAndServe()
			return
		}
		if _, err := os.Stat(*tlsCert); err != nil {
			serverErr <- fmt.Errorf("TLS certificate unavailable: %w", err)
			return
		}
		if _, err := os.Stat(*tlsKey); err != nil {
			serverErr <- fmt.Errorf("TLS private key unavailable: %w", err)
			return
		}
		serverErr <- server.ListenAndServeTLS(*tlsCert, *tlsKey)
	}()
	ingress := orchestrator.NewRESPIngress(manager, *respAddr)
	go func() {
		if err := ingress.ListenAndServe(appCtx); err != nil {
			serverErr <- fmt.Errorf("RESP ingress: %w", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	select {
	case sig := <-sigCh:
		log.Printf("Received %v, stopping tenants", sig)
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Control-plane server stopped: %v", err)
		}
	}
	appCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	manager.StopAll()
}
