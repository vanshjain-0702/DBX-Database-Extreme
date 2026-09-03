package orchestrator

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dbx/dbx/internal/auth"
	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/server"
)

type isolatedWorker struct {
	tenant *Tenant
	cmd    *exec.Cmd
	errCh  chan error
	httpCl *http.Client
	token  string
}

func (w *isolatedWorker) Stop() {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return
	}
	_ = w.cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_, _ = w.cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = w.cmd.Process.Kill()
	}
}

func (w *isolatedWorker) ErrorChannel() <-chan error { return w.errCh }

// WorkerToken returns the per-worker control token for a sandboxed tenant.
// In-process tenants have no worker and fall back to the node-wide token.
func (m *Manager) WorkerToken(tenantID string) (string, bool) {
	if m == nil {
		return "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	worker := m.workers[tenantID]
	if worker == nil || worker.token == "" {
		return "", false
	}
	return worker.token, true
}

func (w *isolatedWorker) UpsertUser(*auth.User) {}
func (w *isolatedWorker) DeleteUser(string)     {}

func (w *isolatedWorker) UsageSnapshot() server.UsageSnapshot {
	var out server.UsageSnapshot
	_ = w.getJSON("/usage", &out)
	return out
}

func (w *isolatedWorker) MetricsSnapshot() map[string]int64 {
	out := map[string]int64{}
	_ = w.getJSON("/metrics", &out)
	return out
}

func (w *isolatedWorker) CreateBackup(tenantID, outputPath string) (persistence.BackupManifest, error) {
	var manifest persistence.BackupManifest
	body, _ := json.Marshal(map[string]string{"output": outputPath})
	req, err := http.NewRequest(http.MethodPost, "http://localhost/internal/backup", bytes.NewReader(body))
	if err != nil {
		return manifest, err
	}
	req.Header.Set("X-DBX-Internal-Token", w.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.httpCl.Do(req)
	if err != nil {
		return manifest, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return manifest, fmt.Errorf("worker backup: %s", bytes.TrimSpace(msg))
	}
	err = json.NewDecoder(resp.Body).Decode(&manifest)
	return manifest, err
}

func (w *isolatedWorker) getJSON(path string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-DBX-Internal-Token", w.token)
	resp, err := w.httpCl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func unixHTTPClient(socket string) *http.Client {
	return &http.Client{
		// Backup of a large tenant WAL/snapshot can exceed a few seconds.
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
}

func lookupServerBinary() (string, error) {
	if v := strings.TrimSpace(os.Getenv("DBX_SERVER_BIN")); v != "" {
		return v, nil
	}
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), "dbx-server")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("dbx-server"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("dbx-server binary not found; set DBX_SERVER_BIN")
}

// childEnv builds the worker environment from an allowlist. A tenant worker
// must never inherit control-plane secrets: with DBX_JWT_SECRET in its environ
// a compromised worker could mint operator tokens and reach every other
// tenant, which defeats the point of sandboxing it.
func childEnv(extra ...string) []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "TMPDIR": true, "TZ": true,
		"LANG": true, "LC_ALL": true, "GOMAXPROCS": true,
	}
	var out []string
	for _, e := range os.Environ() {
		name, _, found := strings.Cut(e, "=")
		if found && allowed[name] {
			out = append(out, e)
		}
	}
	return append(out, extra...)
}

func newWorkerToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ReloadACL tells the worker to re-read acl.json now instead of waiting for its
// poll interval, so minting and revoking a key take effect immediately.
func (w *isolatedWorker) ReloadACL() error {
	req, err := http.NewRequest(http.MethodPost, "http://localhost/internal/acl/reload", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-DBX-Internal-Token", w.token)
	resp, err := w.httpCl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker acl reload: status %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) startIsolatedWorker(t *Tenant, cfgPath string, dek []byte, quota int64) error {
	bin, err := lookupServerBinary()
	if err != nil {
		return err
	}
	token, err := newWorkerToken()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "-config", cfgPath, "-isolate", "-dek-stdin", "-tenant-id", t.ID)
	cmd.Dir = t.DataDir
	cmd.Stdin = bytes.NewReader(dek)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Each worker gets its own control token, so one worker cannot authenticate
	// to another worker's control endpoints even if it escapes its sandbox.
	cmd.Env = childEnv(
		"DBX_TENANT_ID="+t.ID,
		"DBX_INTERNAL_API_TOKEN="+token,
		fmt.Sprintf("DBX_ORCHESTRATOR_PID=%d", os.Getpid()),
	)
	cmd.SysProcAttr = workerProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := isolation.ConfineCgroup(t.ID, cmd.Process.Pid, quota); err != nil {
		fmt.Printf("[Orchestrator] tenant %s cgroup unavailable: %v\n", t.ID, err)
	}
	worker := &isolatedWorker{
		tenant: t,
		cmd:    cmd,
		errCh:  make(chan error, 1),
		httpCl: unixHTTPClient(isolation.HTTPSocket(t.DataDir)),
		token:  token,
	}
	m.mu.Lock()
	if m.workers == nil {
		m.workers = make(map[string]*isolatedWorker)
	}
	m.workers[t.ID] = worker
	m.mu.Unlock()
	go func() {
		err := cmd.Wait()
		worker.errCh <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(isolation.RESPSocket(t.DataDir)); err == nil {
			return nil
		}
		select {
		case err := <-worker.errCh:
			return fmt.Errorf("tenant worker exited: %w", err)
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	worker.Stop()
	return fmt.Errorf("tenant worker did not bind %s", isolation.RESPSocket(t.DataDir))
}

func collectTenantUsers(tenants map[string]*Tenant, t *Tenant) []*auth.User {
	users := make([]*auth.User, 0)
	addKeys := func(keys map[string]*TenantKey) {
		for _, key := range keys {
			if key != nil && !key.Revoked {
				users = append(users, tenantUser(key))
			}
		}
	}
	addKeys(t.Keys)
	if t.Role == "replica" {
		if primary, ok := tenants[t.ReplicaOf]; ok {
			addKeys(primary.Keys)
		}
	}
	return users
}

func writeUsersACL(dataDir string, users []*auth.User) error {
	store := auth.NewACLStore()
	store.DisableDefault()
	for _, user := range users {
		store.AddUser(user)
	}
	return store.WriteFile(isolation.ACLFile(dataDir))
}

// syncTenantACL rewrites acl.json for a tenant and its replicas, then tells any
// live worker to reload immediately. Without the reload call a freshly minted
// key is rejected until the worker's next poll, and a revoked key keeps working
// for that window.
func (m *Manager) syncTenantACL(t *Tenant) {
	if t == nil {
		return
	}
	m.mu.RLock()
	targets := []*Tenant{t}
	if t.Role != "replica" {
		for _, rid := range t.Replicas {
			if rt, ok := m.tenants[rid]; ok {
				targets = append(targets, rt)
			}
		}
	}
	type pending struct {
		users  []*auth.User
		dir    string
		worker *isolatedWorker
	}
	work := make([]pending, 0, len(targets))
	for _, target := range targets {
		work = append(work, pending{
			users:  collectTenantUsers(m.tenants, target),
			dir:    target.DataDir,
			worker: m.workers[target.ID],
		})
	}
	m.mu.RUnlock()

	for _, item := range work {
		if err := writeUsersACL(item.dir, item.users); err != nil {
			fmt.Printf("[Orchestrator] acl write %s failed: %v\n", item.dir, err)
			continue
		}
		if item.worker != nil {
			if err := item.worker.ReloadACL(); err != nil {
				fmt.Printf("[Orchestrator] acl reload failed: %v\n", err)
			}
		}
	}
}
