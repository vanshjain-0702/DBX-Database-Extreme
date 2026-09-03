package orchestrator

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/server"
)

var (
	workerBinOnce sync.Once
	workerBinPath string
	workerBinErr  error
)

// strictWorkerBinary builds cmd/dbx-server once per test binary. Strict mode
// spawns a real process, so there is no way to cover it without one.
func strictWorkerBinary(t *testing.T) string {
	t.Helper()
	workerBinOnce.Do(func() {
		if v := strings.TrimSpace(os.Getenv("DBX_SERVER_BIN")); v != "" {
			workerBinPath = v
			return
		}
		dir, err := os.MkdirTemp("", "dbx-worker-bin")
		if err != nil {
			workerBinErr = err
			return
		}
		out := filepath.Join(dir, "dbx-server")
		cmd := exec.Command("go", "build", "-o", out, "github.com/dbx/dbx/cmd/dbx-server")
		if combined, err := cmd.CombinedOutput(); err != nil {
			workerBinErr = fmt.Errorf("building dbx-server: %v: %s", err, combined)
			return
		}
		workerBinPath = out
	})
	if workerBinErr != nil {
		t.Fatalf("%v", workerBinErr)
	}
	return workerBinPath
}

func newStrictManager(t *testing.T) (*Manager, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("strict isolation is Linux-only")
	}
	root := t.TempDir()
	t.Setenv("DBX_DATA_DIR", root)
	t.Setenv("DBX_ISOLATION_MODE", "strict")
	t.Setenv("DBX_KEK", hex.EncodeToString(bytes.Repeat([]byte{0x2b}, 32)))
	t.Setenv("DBX_SERVER_BIN", strictWorkerBinary(t))

	m := &Manager{
		tenants:      make(map[string]*Tenant),
		stateFile:    filepath.Join(root, "state.json"),
		instances:    make(map[string]*server.Instance),
		workers:      make(map[string]*isolatedWorker),
		starting:     make(map[string]bool),
		restarts:     make(map[string]int),
		tenantQuotas: make(map[string]int64),
		nextHTTPPort: 18081,
		nextRESPPort: 16401,
		nextReplPort: 17401,
		profile:      isolation.FromEnv(),
	}
	if !m.profile.Process {
		t.Skipf("strict profile unavailable: %s", m.profile)
	}
	return m, root
}

func waitRunning(t *testing.T, m *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if tenant, ok := m.GetTenant(id); ok && m.TenantRunning(id) {
			_, respErr := os.Stat(isolation.RESPSocket(tenant.DataDir))
			_, httpErr := os.Stat(isolation.HTTPSocket(tenant.DataDir))
			if respErr == nil && httpErr == nil {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tenant %s never started", id)
}

// tenantSocketCommand talks to a worker's RESP socket. The test process is the
// orchestrator, so SO_PEERCRED lets it through.
func tenantSocketCommand(t *testing.T, dataDir, auth, secret string, args ...string) string {
	t.Helper()
	conn, err := net.DialTimeout("unix", isolation.RESPSocket(dataDir), 2*time.Second)
	if err != nil {
		t.Fatalf("dial tenant socket: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	send := func(parts ...string) {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "*%d\r\n", len(parts))
		for _, p := range parts {
			fmt.Fprintf(&buf, "$%d\r\n%s\r\n", len(p), p)
		}
		if _, err := conn.Write(buf.Bytes()); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	reader := bufio.NewReader(conn)
	send("AUTH", auth, secret)
	authLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("auth read: %v", err)
	}
	if !strings.HasPrefix(authLine, "+OK") {
		return authLine
	}
	send(args...)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.HasPrefix(line, "$") && !strings.HasPrefix(line, "$-1") {
		body, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		return line + body
	}
	return line
}

func TestStrictModeSpawnsSealedWorkerAndServes(t *testing.T) {
	m, _ := newStrictManager(t)
	tenant, err := m.Provision("acme", "Acme", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, tenant.ID)

	secret, key, err := m.CreateTenantKey(tenant.ID, "w", "writer", nil)
	if err != nil {
		t.Fatal(err)
	}

	// A minted key must work on the very next command. The ACL reaches the
	// worker through acl.json, so this fails if the reload is only polled.
	auth := key.ID
	if got := tenantSocketCommand(t, tenant.DataDir, auth, secret, "SET", "session", "topsecretvalue"); got != "+OK\r\n" {
		t.Fatalf("SET right after mint = %q (credential propagation is not synchronous)", got)
	}
	if got := tenantSocketCommand(t, tenant.DataDir, auth, secret, "GET", "session"); got != "$14\r\ntopsecretvalue\r\n" {
		t.Fatalf("GET = %q", got)
	}

	// Revocation must also take effect immediately.
	if err := m.RevokeTenantKey(tenant.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	if got := tenantSocketCommand(t, tenant.DataDir, auth, secret, "GET", "session"); strings.HasPrefix(got, "$") {
		t.Fatalf("revoked key still served data: %q", got)
	}

	// The socket must be private to the orchestrator and the WAL ciphertext.
	info, err := os.Stat(isolation.RESPSocket(tenant.DataDir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("tenant socket mode = %o, want 600", perm)
	}
	walBytes, err := os.ReadFile(filepath.Join(tenant.DataDir, "wal", "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(walBytes, []byte("topsecretvalue")) {
		t.Fatal("WAL contains plaintext under strict isolation")
	}
}

// The worker must not inherit control-plane secrets. With DBX_JWT_SECRET in its
// environment a compromised worker could mint operator tokens.
func TestStrictWorkerDoesNotInheritControlPlaneSecrets(t *testing.T) {
	t.Setenv("DBX_JWT_SECRET", "operator-jwt-secret-value-32-chars")
	t.Setenv("DBX_ADMIN_PASSWORD", "operator-admin-password")
	m, _ := newStrictManager(t)
	tenant, err := m.Provision("sealed", "Sealed", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, tenant.ID)

	m.mu.RLock()
	worker := m.workers[tenant.ID]
	m.mu.RUnlock()
	if worker == nil || worker.cmd == nil || worker.cmd.Process == nil {
		t.Fatal("no worker process")
	}
	environ, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", worker.cmd.Process.Pid))
	if err != nil {
		t.Skipf("cannot read worker environ: %v", err)
	}
	for _, forbidden := range []string{"DBX_KEK=", "DBX_JWT_SECRET=", "DBX_ADMIN_PASSWORD=", "DBX_DEFAULT_PASSWORD="} {
		if bytes.Contains(environ, []byte(forbidden)) {
			t.Errorf("tenant worker inherited %s", strings.TrimSuffix(forbidden, "="))
		}
	}
}

// Landlock must stop a worker reading a neighbour's key material.
func TestStrictWorkerCannotReadSiblingTenantKey(t *testing.T) {
	m, root := newStrictManager(t)
	if _, err := m.Provision("one", "One", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Provision("two", "Two", 0); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, "one")
	waitRunning(t, m, "two")

	victim := filepath.Join(root, "tenants", "two", isolation.WrappedDEKName)
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("expected a wrapped DEK for tenant two: %v", err)
	}
	// Confine to tenant one, then attempt the cross-tenant read the USP forbids.
	out, err := exec.Command(strictWorkerBinary(t), "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("worker binary unusable: %v %s", err, out)
	}
	probe := exec.Command("/proc/self/exe", "-test.run=TestStrictSiblingProbeHelper")
	probe.Env = append(os.Environ(),
		"DBX_STRICT_PROBE=1",
		"DBX_STRICT_PROBE_DIR="+filepath.Join(root, "tenants", "one"),
		"DBX_STRICT_PROBE_TARGET="+victim,
	)
	probeOut, probeErr := probe.CombinedOutput()
	if probeErr != nil {
		if exit, ok := probeErr.(*exec.ExitError); ok && exit.ExitCode() == 3 {
			t.Skipf("landlock unavailable on this kernel: %s", probeOut)
		}
		t.Fatalf("probe failed: %v %s", probeErr, probeOut)
	}
	if !bytes.Contains(probeOut, []byte("BLOCKED")) {
		t.Fatalf("sandboxed worker could read a sibling tenant's DEK: %s", probeOut)
	}
}

func TestStrictSiblingProbeHelper(t *testing.T) {
	if os.Getenv("DBX_STRICT_PROBE") != "1" {
		t.Skip("helper process only")
	}
	if err := isolation.RestrictFilesystem(os.Getenv("DBX_STRICT_PROBE_DIR")); err != nil {
		fmt.Println("landlock unavailable:", err)
		os.Exit(3)
	}
	if _, err := os.ReadFile(os.Getenv("DBX_STRICT_PROBE_TARGET")); err != nil {
		fmt.Println("BLOCKED")
		os.Exit(0)
	}
	fmt.Println("LEAKED")
	os.Exit(0)
}

// Replication has to survive the move to sandboxed processes and Unix sockets.
func TestStrictModeReplicationReachesReplica(t *testing.T) {
	m, _ := newStrictManager(t)
	primary, err := m.Provision("repl", "Repl", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, primary.ID)
	waitRunning(t, m, "repl-r1")

	replica, ok := m.GetTenant("repl-r1")
	if !ok {
		t.Fatal("replica missing")
	}
	secret, key, err := m.CreateTenantKey(primary.ID, "w", "writer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantSocketCommand(t, primary.DataDir, key.ID, secret, "SET", "k", "replicated"); got != "+OK\r\n" {
		t.Fatalf("primary SET = %q", got)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		got := tenantSocketCommand(t, replica.DataDir, key.ID, secret, "GET", "k")
		if got == "$10\r\nreplicated\r\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replica never received the write under strict isolation: GET = %q", got)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Backup must land in the shared backup directory even though the worker is
// confined to its own tenant directory, and restore must survive a purge.
func TestStrictModeBackupAndRestoreRoundTrip(t *testing.T) {
	m, root := newStrictManager(t)
	tenant, err := m.Provision("archive", "Archive", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, tenant.ID)

	secret, key, err := m.CreateTenantKey(tenant.ID, "w", "writer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantSocketCommand(t, tenant.DataDir, key.ID, secret, "SET", "keep", "value"); got != "+OK\r\n" {
		t.Fatalf("SET = %q", got)
	}

	path, manifest, err := m.BackupTenant(tenant.ID)
	if err != nil {
		t.Fatalf("backup under strict isolation: %v", err)
	}
	if len(manifest.Files) == 0 {
		t.Fatal("empty manifest")
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("backup path %q is not absolute; callers cannot resolve it", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("archive not at reported path %s: %v", path, err)
	}
	if want := filepath.Join(root, "backups"); filepath.Dir(path) != want {
		t.Fatalf("archive landed in %s, want %s", filepath.Dir(path), want)
	}
	var hasWrap bool
	for _, f := range manifest.Files {
		if f.Path == isolation.WrappedDEKName {
			hasWrap = true
		}
	}
	if !hasWrap {
		t.Fatal("archive omits the wrapped DEK, so a restore cannot decrypt it")
	}

	if err := m.RestoreTenant(tenant.ID, path); err != nil {
		t.Fatalf("restore under strict isolation: %v", err)
	}
	waitRunning(t, m, tenant.ID)
	secret2, key2, err := m.CreateTenantKey(tenant.ID, "w2", "writer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantSocketCommand(t, tenant.DataDir, key2.ID, secret2, "GET", "keep"); got != "$5\r\nvalue\r\n" {
		t.Fatalf("restored tenant lost data: GET = %q", got)
	}
}

// Purging a tenant must shred the wrapped key, not just unlink files.
func TestStrictModeDeleteShredsKey(t *testing.T) {
	m, root := newStrictManager(t)
	tenant, err := m.Provision("gone", "Gone", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, tenant.ID)

	wrap := filepath.Join(root, "tenants", "gone", isolation.WrappedDEKName)
	if _, err := os.Stat(wrap); err != nil {
		t.Fatalf("no wrapped DEK: %v", err)
	}
	if err := m.DeleteTenant(tenant.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wrap); !os.IsNotExist(err) {
		t.Fatalf("wrapped DEK survived purge: %v", err)
	}
}
