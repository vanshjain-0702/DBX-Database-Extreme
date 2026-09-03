package orchestrator

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/isolation"
)

// largeWorkload is the default CI-sized load. DBX_LARGE=1 raises it to a
// 50k-key / 20k-vector pass used for the Isolation Kernel certification.
func largeWorkload() (keys, vectors, dim int) {
	if os.Getenv("DBX_LARGE") == "1" {
		return 50000, 20000, 64
	}
	return 10000, 5000, 32
}

type tenantSession struct {
	conn   net.Conn
	reader *bufio.Reader
}

func dialTenant(t *testing.T, dataDir, user, secret string) *tenantSession {
	t.Helper()
	conn, err := net.DialTimeout("unix", isolation.RESPSocket(dataDir), 3*time.Second)
	if err != nil {
		t.Fatalf("dial tenant socket: %v", err)
	}
	s := &tenantSession{conn: conn, reader: bufio.NewReader(conn)}
	if err := s.write("AUTH", user, secret); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if got, err := s.read(); err != nil || got != "OK" {
		conn.Close()
		t.Fatalf("AUTH = %q err=%v", got, err)
	}
	return s
}

func (s *tenantSession) Close() {
	if s != nil && s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *tenantSession) write(args ...string) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&buf, "$%d\r\n%s\r\n", len(arg), arg)
	}
	_, err := s.conn.Write(buf.Bytes())
	return err
}

func (s *tenantSession) read() (string, error) {
	v, err := s.readValue()
	if err != nil {
		return "", err
	}
	return stringifyRESP(v), nil
}

func (s *tenantSession) readValue() (any, error) {
	prefix, err := s.reader.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+', '-', ':':
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return strings.TrimSuffix(line, "\r\n"), nil
	case '$':
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(s.reader, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, n)
		for i := 0; i < n; i++ {
			item, err := s.readValue()
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown RESP prefix %q", prefix)
	}
}

func stringifyRESP(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			parts[i] = stringifyRESP(item)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(t)
	}
}

func floatArgs(vec []float32) []string {
	out := make([]string, len(vec))
	for i, v := range vec {
		out[i] = strconv.FormatFloat(float64(v), 'f', 6, 32)
	}
	return out
}

func unitVector(seed uint64, dim int) []float32 {
	vec := make([]float32, dim)
	var norm float64
	x := seed
	for i := range vec {
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
		vec[i] = float32(int64(x%2001)-1000) / 1000
		norm += float64(vec[i] * vec[i])
	}
	if norm == 0 {
		vec[0] = 1
		return vec
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec
}

func workerRSS(pid int) int64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0
			}
			kb, _ := strconv.ParseInt(fields[1], 10, 64)
			return kb * 1024
		}
	}
	return 0
}

func restartTenant(t *testing.T, m *Manager, tenant *Tenant) {
	t.Helper()
	m.mu.Lock()
	worker := m.workers[tenant.ID]
	delete(m.workers, tenant.ID)
	m.mu.Unlock()
	if worker != nil {
		worker.Stop()
	}
	if err := m.StartTenant(tenant); err != nil {
		t.Fatalf("restart tenant: %v", err)
	}
	waitRunning(t, m, tenant.ID)
}

// Full-stack check: a sandboxed worker ingests a large KV + vector set through
// its Unix RESP socket, serves search, survives a process restart, and still
// reports usage through the worker-specific HTTP token.
func TestStrictModeLargeDataSurvivesRestart(t *testing.T) {
	keys, vectors, dim := largeWorkload()
	m, _ := newStrictManager(t)
	tenant, err := m.Provision("bulk", "Bulk", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()
	waitRunning(t, m, tenant.ID)

	secret, key, err := m.CreateTenantKey(tenant.ID, "w", "writer", nil)
	if err != nil {
		t.Fatal(err)
	}
	sess := dialTenant(t, tenant.DataDir, key.ID, secret)
	defer sess.Close()
	_ = sess.conn.SetDeadline(time.Now().Add(4 * time.Minute))

	ingestStart := time.Now()
	const kvPipe = 200
	for start := 0; start < keys; start += kvPipe {
		end := start + kvPipe
		if end > keys {
			end = keys
		}
		for i := start; i < end; i++ {
			if err := sess.write("SET", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)); err != nil {
				t.Fatalf("SET %d: %v", i, err)
			}
		}
		for i := start; i < end; i++ {
			got, err := sess.read()
			if err != nil || got != "OK" {
				t.Fatalf("SET %d reply = %q err=%v", i, got, err)
			}
		}
	}

	const batch = 100
	for start := 0; start < vectors; start += batch {
		end := start + batch
		if end > vectors {
			end = vectors
		}
		args := []string{"VADD_BATCH", "memories", strconv.Itoa(dim)}
		for i := start; i < end; i++ {
			args = append(args, "vec"+strconv.Itoa(i))
			args = append(args, floatArgs(unitVector(uint64(i+1)*0x9e3779b97f4a7c15, dim))...)
		}
		if err := sess.write(args...); err != nil {
			t.Fatalf("VADD_BATCH %d: %v", start, err)
		}
		got, err := sess.read()
		if err != nil {
			t.Fatalf("VADD_BATCH %d read: %v", start, err)
		}
		if got != strconv.Itoa(end-start) {
			t.Fatalf("VADD_BATCH %d = %q, want %d", start, got, end-start)
		}
	}
	ingest := time.Since(ingestStart)

	if err := sess.write("GET", "k0"); err != nil {
		t.Fatal(err)
	}
	if got, err := sess.read(); err != nil || got != "v0" {
		t.Fatalf("GET k0 = %q err=%v", got, err)
	}
	if err := sess.write("GET", fmt.Sprintf("k%d", keys-1)); err != nil {
		t.Fatal(err)
	}
	if got, err := sess.read(); err != nil || got != fmt.Sprintf("v%d", keys-1) {
		t.Fatalf("GET last key = %q err=%v", got, err)
	}

	query := floatArgs(unitVector(uint64(1)*0x9e3779b97f4a7c15, dim))
	searchArgs := append([]string{"VSEARCH", "memories"}, query...)
	searchArgs = append(searchArgs, "5")
	if err := sess.write(searchArgs...); err != nil {
		t.Fatal(err)
	}
	raw, err := sess.readValue()
	if err != nil {
		t.Fatalf("VSEARCH: %v", err)
	}
	hits, ok := raw.([]any)
	if !ok || len(hits) == 0 {
		t.Fatalf("VSEARCH returned %#v", raw)
	}
	firstHit := stringifyRESP(hits[0])
	if !strings.HasPrefix(firstHit, "vec") {
		t.Fatalf("VSEARCH first hit = %q", firstHit)
	}

	usage, err := m.TenantUsage(tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Keys < int64(keys) {
		t.Fatalf("usage keys = %d, want >= %d", usage.Keys, keys)
	}
	if usage.Vectors < int64(vectors) {
		t.Fatalf("usage vectors = %d, want >= %d", usage.Vectors, vectors)
	}

	// HTTP proxy must use the worker token, not the orchestrator token.
	httpSessOK := false
	m.mu.RLock()
	worker := m.workers[tenant.ID]
	m.mu.RUnlock()
	if worker != nil {
		req, err := http.NewRequest(http.MethodGet, "http://localhost/usage", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-DBX-Internal-Token", worker.token)
		resp, err := worker.httpCl.Do(req)
		if err != nil {
			t.Fatalf("worker HTTP usage: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("worker HTTP usage status %d body=%s", resp.StatusCode, body)
		}
		httpSessOK = true
	}

	sess.Close()
	restartTenant(t, m, tenant)
	sess = dialTenant(t, tenant.DataDir, key.ID, secret)
	defer sess.Close()
	_ = sess.conn.SetDeadline(time.Now().Add(30 * time.Second))

	if err := sess.write("GET", "k0"); err != nil {
		t.Fatal(err)
	}
	if got, err := sess.read(); err != nil || got != "v0" {
		t.Fatalf("after restart GET k0 = %q err=%v", got, err)
	}
	if err := sess.write("GET", fmt.Sprintf("k%d", keys-1)); err != nil {
		t.Fatal(err)
	}
	if got, err := sess.read(); err != nil || got != fmt.Sprintf("v%d", keys-1) {
		t.Fatalf("after restart GET last = %q err=%v", got, err)
	}
	if err := sess.write(searchArgs...); err != nil {
		t.Fatal(err)
	}
	raw, err = sess.readValue()
	if err != nil {
		t.Fatalf("VSEARCH after restart: %v", err)
	}
	hits, ok = raw.([]any)
	if !ok || len(hits) == 0 {
		t.Fatalf("VSEARCH after restart returned %#v", raw)
	}

	walBytes, err := os.ReadFile(filepath.Join(tenant.DataDir, "wal", "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(walBytes, []byte("v0")) || bytes.Contains(walBytes, []byte("vec0")) {
		t.Fatal("large-data WAL contains plaintext after sealed ingest")
	}

	t.Logf("strict large-data keys=%d vectors=%d dim=%d ingest=%s usage_keys=%d usage_vectors=%d http=%t",
		keys, vectors, dim, ingest.Round(time.Millisecond), usage.Keys, usage.Vectors, httpSessOK)
}

// Process-per-tenant density: several sandboxed workers stay up together,
// idle GETs stay bounded while others write, and RSS per idle worker is
// recorded so the 100-tenant in-process certification is not reused blindly.
func TestStrictModeDensityAndRSS(t *testing.T) {
	idleN, activeN := 4, 2
	if os.Getenv("DBX_LARGE") == "1" {
		idleN, activeN = 8, 4
	}
	m, _ := newStrictManager(t)
	defer m.StopAll()

	type live struct {
		tenant *Tenant
		auth   string
		secret string
	}
	tenants := make([]live, idleN+activeN)
	for i := range tenants {
		id := fmt.Sprintf("d%d", i)
		tenant, err := m.Provision(id, id, 0)
		if err != nil {
			t.Fatal(err)
		}
		waitRunning(t, m, tenant.ID)
		secret, key, err := m.CreateTenantKey(tenant.ID, "w", "writer", nil)
		if err != nil {
			t.Fatal(err)
		}
		tenants[i] = live{tenant: tenant, auth: key.ID, secret: secret}
		sess := dialTenant(t, tenant.DataDir, key.ID, secret)
		if err := sess.write("SET", "keep", "ok"); err != nil {
			t.Fatal(err)
		}
		if got, err := sess.read(); err != nil || got != "OK" {
			t.Fatalf("seed SET = %q err=%v", got, err)
		}
		sess.Close()
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, noisy := range tenants[idleN:] {
		item := noisy
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess := dialTenant(t, item.tenant.DataDir, item.auth, item.secret)
			defer sess.Close()
			n := 0
			for {
				select {
				case <-stop:
					return
				default:
					_ = sess.conn.SetDeadline(time.Now().Add(2 * time.Second))
					if err := sess.write("SET", "n", strconv.Itoa(n)); err != nil {
						return
					}
					if _, err := sess.read(); err != nil {
						return
					}
					n++
				}
			}
		}()
	}

	idleSess := make([]*tenantSession, idleN)
	for i, quiet := range tenants[:idleN] {
		idleSess[i] = dialTenant(t, quiet.tenant.DataDir, quiet.auth, quiet.secret)
		defer idleSess[i].Close()
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	var max time.Duration
	gets := 0
	for time.Now().Before(deadline) {
		for _, sess := range idleSess {
			_ = sess.conn.SetDeadline(time.Now().Add(2 * time.Second))
			start := time.Now()
			if err := sess.write("GET", "keep"); err != nil {
				close(stop)
				wg.Wait()
				t.Fatal(err)
			}
			got, err := sess.read()
			elapsed := time.Since(start)
			gets++
			if err != nil || got != "ok" {
				close(stop)
				wg.Wait()
				t.Fatalf("idle GET = %q err=%v", got, err)
			}
			if elapsed > max {
				max = elapsed
			}
		}
	}
	close(stop)
	wg.Wait()

	var rssTotal int64
	var rssMax int64
	measured := 0
	m.mu.RLock()
	for _, item := range tenants {
		worker := m.workers[item.tenant.ID]
		if worker == nil || worker.cmd == nil || worker.cmd.Process == nil {
			continue
		}
		rss := workerRSS(worker.cmd.Process.Pid)
		if rss <= 0 {
			continue
		}
		measured++
		rssTotal += rss
		if rss > rssMax {
			rssMax = rss
		}
	}
	m.mu.RUnlock()
	if measured == 0 {
		t.Fatal("could not read worker RSS")
	}
	avg := rssTotal / int64(measured)
	t.Logf("strict density idle=%d active=%d gets=%d worst_idle_get=%s workers=%d avg_rss=%dB max_rss=%dB",
		idleN, activeN, gets, max, measured, avg, rssMax)
	if max > 750*time.Millisecond {
		t.Fatalf("idle GET p-worst %s exceeded strict density budget", max)
	}
	// A freshly started Go worker is tens of MB. Half a gigabyte idle means
	// the mmap/encryption path leaked the corpus onto the heap.
	if rssMax > 512<<20 {
		t.Fatalf("idle worker RSS %d exceeds 512MiB; process isolation voided density", rssMax)
	}
}
