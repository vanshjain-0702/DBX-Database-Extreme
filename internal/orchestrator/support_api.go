package orchestrator

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SupportAPI exposes only the tenant summaries and lifecycle action granted by
// the operator's explicit tenant allowlists. It does not reuse admin JWTs.
type SupportAPI struct {
	manager     *Manager
	readToken   []byte
	wakeToken   []byte
	readTenants map[string]struct{}
	wakeTenants map[string]struct{}
	auditPath   string
	mu          sync.Mutex
	lastWake    map[string]time.Time
}

func NewSupportAPI(manager *Manager, readToken, wakeToken, readTenants, wakeTenants, auditPath string) (*SupportAPI, error) {
	readIDs, err := parseSupportTenantAllowlist(readTenants)
	if err != nil {
		return nil, fmt.Errorf("DBX_SUPPORT_READ_TENANTS: %w", err)
	}
	wakeIDs, err := parseSupportTenantAllowlist(wakeTenants)
	if err != nil {
		return nil, fmt.Errorf("DBX_SUPPORT_WAKE_TENANTS: %w", err)
	}
	if readToken != "" && len(readToken) < 32 {
		return nil, errors.New("DBX_SUPPORT_READ_TOKEN must contain at least 32 characters")
	}
	if wakeToken != "" && len(wakeToken) < 32 {
		return nil, errors.New("DBX_SUPPORT_WAKE_TOKEN must contain at least 32 characters")
	}
	for _, token := range []string{readToken, wakeToken} {
		for _, ch := range token {
			if ch <= 32 || ch >= 127 {
				return nil, errors.New("DBX support tokens must be printable ASCII without whitespace")
			}
		}
	}
	if wakeToken != "" && readToken == "" {
		return nil, errors.New("DBX_SUPPORT_READ_TOKEN is required when support wake is enabled")
	}
	if readToken != "" && wakeToken != "" && hmac.Equal([]byte(readToken), []byte(wakeToken)) {
		return nil, errors.New("DBX support read and wake tokens must be different")
	}
	if readToken != "" && len(readIDs) == 0 {
		return nil, errors.New("DBX_SUPPORT_READ_TENANTS must list at least one tenant when read support is enabled")
	}
	if wakeToken != "" && len(wakeIDs) == 0 {
		return nil, errors.New("DBX_SUPPORT_WAKE_TENANTS must list at least one tenant when wake support is enabled")
	}
	for id := range wakeIDs {
		if _, ok := readIDs[id]; !ok {
			return nil, fmt.Errorf("wake tenant %q must also be in DBX_SUPPORT_READ_TENANTS", id)
		}
	}
	if auditPath == "" {
		auditPath = filepath.Join("data", "support-audit.jsonl")
	}
	return &SupportAPI{
		manager: manager, readToken: []byte(readToken), wakeToken: []byte(wakeToken),
		readTenants: readIDs, wakeTenants: wakeIDs, auditPath: auditPath,
		lastWake: make(map[string]time.Time),
	}, nil
}

func parseSupportTenantAllowlist(value string) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	for _, raw := range strings.Split(value, ",") {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if id == "." || id == ".." {
			return nil, fmt.Errorf("tenant ID %q is a path segment", id)
		}
		for _, ch := range id {
			if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '.' && ch != '_' && ch != '-' {
				return nil, fmt.Errorf("tenant ID %q contains unsupported characters", id)
			}
		}
		if _, duplicate := ids[id]; duplicate {
			return nil, fmt.Errorf("tenant ID %q is listed more than once", id)
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}

func (s *SupportAPI) Snapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		supportError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if len(s.readToken) == 0 {
		supportError(w, http.StatusServiceUnavailable, "support read capability is not configured")
		return
	}
	if !supportBearerMatches(r, s.readToken) {
		supportError(w, http.StatusUnauthorized, "invalid support capability")
		return
	}
	if s.manager == nil {
		supportError(w, http.StatusServiceUnavailable, "support manager is unavailable")
		return
	}
	tenantViews := s.manager.ListTenantViews()
	views := make([]TenantView, 0, len(s.readTenants))
	usage := make([]TenantUsage, 0, len(s.readTenants))
	for _, tenant := range tenantViews {
		if _, allowed := s.readTenants[tenant.ID]; allowed {
			// Replication relationships may name tenants outside this capability.
			// Redact those references as well as filtering the top-level rows.
			if _, readable := s.readTenants[tenant.ReplicaOf]; !readable {
				tenant.ReplicaOf = ""
			}
			replicas := make([]string, 0, len(tenant.Replicas))
			for _, replica := range tenant.Replicas {
				if _, readable := s.readTenants[replica]; readable {
					replicas = append(replicas, replica)
				}
			}
			tenant.Replicas = replicas
			views = append(views, tenant)
		}
	}
	for _, tenantID := range sortedSupportTenantIDs(s.readTenants) {
		row, err := s.manager.TenantUsage(tenantID)
		if err == nil {
			usage = append(usage, row)
		}
	}
	sort.Slice(views, func(i, j int) bool { return views[i].ID < views[j].ID })
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tenants": views, "usage": usage})
}

func sortedSupportTenantIDs(ids map[string]struct{}) []string {
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	return ordered
}

func (s *SupportAPI) Wake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		supportError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if len(s.wakeToken) == 0 {
		supportError(w, http.StatusServiceUnavailable, "support wake capability is not configured")
		return
	}
	if !supportBearerMatches(r, s.wakeToken) {
		supportError(w, http.StatusUnauthorized, "invalid support capability")
		return
	}
	if s.manager == nil {
		supportError(w, http.StatusServiceUnavailable, "support manager is unavailable")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "support" || parts[2] != "v1" || parts[3] != "tenants" || parts[5] != "wake" {
		supportError(w, http.StatusNotFound, "not found")
		return
	}
	tenantID := parts[4]
	if _, allowed := s.wakeTenants[tenantID]; !allowed {
		supportError(w, http.StatusForbidden, "tenant is outside the support wake allowlist")
		return
	}
	hibernated := false
	for _, tenant := range s.manager.ListTenantViews() {
		if tenant.ID == tenantID && tenant.Status == "hibernated" {
			hibernated = true
			break
		}
	}
	if !hibernated {
		supportError(w, http.StatusConflict, "tenant is not hibernated; no recovery action was taken")
		return
	}

	s.mu.Lock()
	// Take the timestamp under the lock so simultaneous requests cannot reserve
	// cooldowns in an order different from the time they were measured.
	now := time.Now().UTC()
	if last := s.lastWake[tenantID]; !last.IsZero() && now.Sub(last) < 5*time.Minute {
		s.mu.Unlock()
		w.Header().Set("Retry-After", "300")
		supportError(w, http.StatusTooManyRequests, "tenant wake is rate limited")
		return
	}
	s.lastWake[tenantID] = now
	s.mu.Unlock()

	if err := s.appendAudit(tenantID, "attempt"); err != nil {
		s.mu.Lock()
		if s.lastWake[tenantID].Equal(now) {
			delete(s.lastWake, tenantID)
		}
		s.mu.Unlock()
		supportError(w, http.StatusServiceUnavailable, "support audit is unavailable; no recovery action was taken")
		return
	}
	if err := s.manager.WakeTenantIfHibernated(tenantID); err != nil {
		_ = s.appendAudit(tenantID, "failed")
		supportError(w, http.StatusConflict, "tenant wake failed")
		return
	}
	if err := s.appendAudit(tenantID, "accepted"); err != nil {
		supportError(w, http.StatusInternalServerError, "wake was accepted but its final audit record failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "tenant_id": tenantID})
}

func (s *SupportAPI) appendAudit(tenantID, outcome string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.auditPath), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.auditPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(map[string]string{
		"time":      time.Now().UTC().Format(time.RFC3339Nano),
		"tenant_id": tenantID,
		"action":    "wake",
		"outcome":   outcome,
	}); err != nil {
		return err
	}
	return file.Sync()
}

func supportBearerMatches(r *http.Request, expected []byte) bool {
	parts := strings.Fields(r.Header.Get("Authorization"))
	return len(parts) == 2 && parts[0] == "Bearer" && hmac.Equal([]byte(parts[1]), expected)
}

func supportError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, message, status)
}
