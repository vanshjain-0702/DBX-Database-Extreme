package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dbx/dbx/dashboard"
	"github.com/dbx/dbx/internal/orchestrator"
)

func main() {
	nodeID := flag.String("id", "node1", "Node ID")
	bindAddr := flag.String("bind", "127.0.0.1:8001", "Raft bind address")
	raftDir := flag.String("raftdir", "./data/raft", "Raft data dir")
	joinAddr := flag.String("join", "", "Join address of existing leader")
	httpPort := flag.Int("port", 8000, "HTTP API port")
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

	// Initialize Raft Control Plane
	raftNode, err := orchestrator.NewRaftNode(*nodeID, *bindAddr, *raftDir, manager)
	if err != nil {
		log.Fatalf("Failed to init Raft: %v", err)
	}
	manager.RaftNode = raftNode

	if *joinAddr == "" {
		// Bootstrap standalone
		if err := raftNode.Bootstrap(*nodeID, *bindAddr); err != nil {
			log.Printf("Bootstrap error (may already exist): %v", err)
		}
	} else {
		// Attempt to join leader via HTTP API (omitted actual request logic for brevity in this MVP,
		// but typically would make a POST to the leader's /api/raft/join)
		log.Printf("Should join leader at %s", *joinAddr)
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
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
		} else if host, _, err := strings.Cut(r.RemoteAddr, ":"); err {
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
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		t, err := manager.Provision(req.ID, req.Name)
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
		json.NewEncoder(w).Encode(manager.ListTenants())
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

		tenant, ok := manager.GetTenant(req.ID)
		if !ok {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}

		if err := orchestrator.RunBackup(tenant.DataDir, tenant.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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

	// Data Plane proxy
	protectedMux.Handle("/t/", proxy)

	// Apply middleware to protected routes
	mux.Handle("/api/provision", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/tenants/backup", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/api/admin/password", orchestrator.RequireAuth(jwtSecret, protectedMux))
	mux.Handle("/t/", orchestrator.RequireAuth(jwtSecret, protectedMux))

	// Raft Join API (Internal/Admin)
	raftJoinHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			NodeID string `json:"node_id"`
			Addr   string `json:"addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if err := raftNode.Join(req.NodeID, req.Addr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/api/raft/join", orchestrator.RequireAuth(jwtSecret, raftJoinHandler))

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
	if *insecureHTTP {
		log.Println("WARNING: control-plane TLS disabled by -insecure-http")
		log.Fatal(server.ListenAndServe())
	}
	if _, err := os.Stat(*tlsCert); err != nil {
		log.Fatalf("TLS certificate unavailable: %v", err)
	}
	if _, err := os.Stat(*tlsKey); err != nil {
		log.Fatalf("TLS private key unavailable: %v", err)
	}
	log.Fatal(server.ListenAndServeTLS(*tlsCert, *tlsKey))
}
