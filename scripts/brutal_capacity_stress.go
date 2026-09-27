// Brutal capacity + feature stress for DBX updates (VMIGRATE WAL, AS_OF, density).
//go:build ignore

package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/events"
	"github.com/dbx/dbx/internal/observability"
	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/query"
	"github.com/dbx/dbx/internal/transaction"
)

func main() {
	fmt.Println("=== DBX BRUTAL CAPACITY + FEATURE STRESS ===")
	fmt.Printf("GOMAXPROCS=%d  CPUs=%d\n\n", runtime.GOMAXPROCS(0), runtime.NumCPU())

	runDensity("certified", 100, 25, 5*time.Second, 250*time.Millisecond)
	runDensity("2x-cert", 200, 50, 5*time.Second, 500*time.Millisecond)
	runDensity("4x-cert", 400, 100, 5*time.Second, time.Second)
	runDensity("push-800", 800, 200, 4*time.Second, 2*time.Second)

	fmt.Println()
	runFeatureBrutal(50, 40) // 50 tenant engines × 40 ops (migrate/AS_OF/KV/vector)
}

func runDensity(label string, idleN, activeN int, dur, budget time.Duration) {
	stores := make([]*engine.KVStore, idleN+activeN)
	for i := range stores {
		stores[i] = engine.New(8)
		stores[i].Set("keep", []byte("ok"), protocol.TypeString, 0)
	}
	idle, active := stores[:idleN], stores[idleN:]
	stop := make(chan struct{})
	var wg sync.WaitGroup
	payload := make([]byte, 128)
	var writes atomic.Int64
	for _, noisy := range active {
		store := noisy
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := 0
			for {
				select {
				case <-stop:
					return
				default:
					store.Set("n", payload, protocol.TypeString, 0)
					writes.Add(1)
					n++
					if n%64 == 0 {
						runtime.Gosched()
					}
				}
			}
		}()
	}

	deadline := time.Now().Add(dur)
	var max time.Duration
	ops := 0
	for time.Now().Before(deadline) {
		for _, quiet := range idle {
			start := time.Now()
			entry := quiet.Get("keep")
			elapsed := time.Since(start)
			ops++
			if entry == nil {
				close(stop)
				wg.Wait()
				fmt.Printf("DENSITY %-10s FAIL idle key disappeared\n", label)
				return
			}
			if elapsed > max {
				max = elapsed
			}
		}
	}
	close(stop)
	wg.Wait()
	status := "PASS"
	if max > budget {
		status = "FAIL"
	}
	fmt.Printf("DENSITY %-10s %s  tenants=%d (idle=%d active=%d)  idle_gets=%d  worst_idle=%s  budget=%s  noisy_writes=%d\n",
		label, status, idleN+activeN, idleN, activeN, ops, max.Round(time.Microsecond), budget, writes.Load())
}

func runFeatureBrutal(tenants, opsPer int) {
	root := filepath.Join(os.TempDir(), fmt.Sprintf("dbx-brutal-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(root, 0o700)
	defer os.RemoveAll(root)

	cats := map[string]*brutalStats{
		"KV": {}, "Vector": {}, "Migrate": {}, "TimeTravel": {},
	}

	var wg sync.WaitGroup
	t0 := time.Now()
	for t := 0; t < tenants; t++ {
		wg.Add(1)
		go func(tid int) {
			defer wg.Done()
			dir := filepath.Join(root, fmt.Sprintf("t%d", tid))
			_ = os.MkdirAll(filepath.Join(dir, "wal"), 0o700)
			kv := engine.New(16)
			wal, err := persistence.OpenWAL(filepath.Join(dir, "wal"), "everysec", 64)
			if err != nil {
				atomicAdd(cats["Migrate"], false, 0)
				return
			}
			defer wal.Close()
			vec := engine.NewVectorStore(kv, dir, 0)
			defer vec.CloseAll()
			ex := query.NewExecutor(
				kv, vec,
				transaction.NewMultiManager(), transaction.NewWatchSet(), transaction.NewMVCCStore(8),
				events.NewPubSub(32, 32), &observability.Metrics{}, wal,
			)
			ex.SetMemoryLimit(64 << 20)
			rng := rand.New(rand.NewSource(int64(tid + 1)))

			for i := 0; i < opsPer; i++ {
				switch rng.Intn(4) {
				case 0:
					start := time.Now()
					key := fmt.Sprintf("k:%d", rng.Intn(200))
					kv.Set(key, []byte(fmt.Sprintf("v-%d-%d", tid, i)), protocol.TypeString, 0)
					if e := kv.Get(key); e == nil {
						atomicAdd(cats["KV"], false, time.Since(start))
					} else {
						atomicAdd(cats["KV"], true, time.Since(start))
					}
				case 1:
					start := time.Now()
					id := fmt.Sprintf("d:%d", i)
					v := unit(rng, 64)
					if err := vec.VAdd("idx", id, v); err != nil {
						atomicAdd(cats["Vector"], false, time.Since(start))
						continue
					}
					if _, err := vec.VSearch("idx", v, 5, nil); err != nil {
						atomicAdd(cats["Vector"], false, time.Since(start))
					} else {
						atomicAdd(cats["Vector"], true, time.Since(start))
					}
				case 2:
					start := time.Now()
					name := fmt.Sprintf("mig-%d", i)
					_ = vec.VAdd(name, "seed", unit(rng, 32))
					ok := true
					if err := vec.StartMigration(name, 32, engine.EncodingSQ8); err != nil {
						ok = false
					} else if err := vec.InsertShadowVector(name, "n1", unit(rng, 32)); err != nil {
						ok = false
					} else if err := vec.SwapMigration(name); err != nil {
						ok = false
					}
					atomicAdd(cats["Migrate"], ok, time.Since(start))
				case 3:
					start := time.Now()
					index := "tt"
					v1 := unit(rng, 32)
					if err := writeVAdd(ex, index, "doc", v1); err != nil {
						atomicAdd(cats["TimeTravel"], false, time.Since(start))
						continue
					}
					time.Sleep(time.Millisecond)
					ts := time.Now().UnixNano()
					time.Sleep(time.Millisecond)
					v2 := unit(rng, 32)
					if err := writeVAdd(ex, index, "doc", v2); err != nil {
						atomicAdd(cats["TimeTravel"], false, time.Since(start))
						continue
					}
					hits, err := searchASOF(ex, index, v1, ts)
					ok := err == nil && len(hits) > 0
					atomicAdd(cats["TimeTravel"], ok, time.Since(start))
				}
			}
		}(t)
	}
	wg.Wait()
	elapsed := time.Since(t0)

	fmt.Println("=== FEATURE BRUTAL (per-tenant engines + WAL) ===")
	fmt.Printf("tenants=%d  ops_per_tenant=%d  wall=%s\n", tenants, opsPer, elapsed.Round(time.Millisecond))
	totalOK, totalErr := int64(0), int64(0)
	for _, name := range []string{"KV", "Vector", "Migrate", "TimeTravel"} {
		s := cats[name]
		s.mu.Lock()
		ok, errn, lats := s.ok, s.err, append([]time.Duration(nil), s.lats...)
		s.mu.Unlock()
		totalOK += ok
		totalErr += errn
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		p50, p99 := durAt(lats, 0.50), durAt(lats, 0.99)
		fmt.Printf("  %-11s ok=%-5d err=%-4d  p50=%-10s p99=%s\n", name, ok, errn, p50, p99)
	}
	fmt.Printf("TOTAL ok=%d err=%d  success_rate=%.2f%%\n",
		totalOK, totalErr, 100*float64(totalOK)/math.Max(1, float64(totalOK+totalErr)))
}

func atomicAdd(s *brutalStats, ok bool, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.ok++
	} else {
		s.err++
	}
	s.lats = append(s.lats, d)
}

type brutalStats struct {
	ok, err int64
	lats    []time.Duration
	mu      sync.Mutex
}

func unit(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	var sum float64
	for i := range v {
		v[i] = float32(rng.NormFloat64())
		sum += float64(v[i] * v[i])
	}
	n := float32(math.Sqrt(sum))
	if n == 0 {
		v[0] = 1
		return v
	}
	for i := range v {
		v[i] /= n
	}
	return v
}

func durAt(lats []time.Duration, p float64) time.Duration {
	if len(lats) == 0 {
		return 0
	}
	i := int(float64(len(lats)-1) * p)
	return lats[i].Round(time.Microsecond)
}

func writeVAdd(ex *query.Executor, index, id string, vec []float32) error {
	args := [][]byte{[]byte(index), []byte(id)}
	for _, f := range vec {
		args = append(args, []byte(fmt.Sprintf("%g", f)))
	}
	cmd := &protocol.Command{Name: "VADD", Args: args}
	var buf discard
	w := protocol.NewWriter(&buf)
	return ex.Execute(1, cmd, w)
}

func searchASOF(ex *query.Executor, index string, vec []float32, ts int64) (string, error) {
	args := [][]byte{[]byte(index)}
	for _, f := range vec {
		args = append(args, []byte(fmt.Sprintf("%g", f)))
	}
	args = append(args, []byte("1"), []byte("AS_OF"), []byte(fmt.Sprintf("%d", ts)))
	cmd := &protocol.Command{Name: "VSEARCH", Args: args}
	var buf discard
	w := protocol.NewWriter(&buf)
	if err := ex.Execute(1, cmd, w); err != nil {
		return "", err
	}
	return string(buf.b), nil
}

type discard struct{ b []byte }

func (d *discard) Write(p []byte) (int, error) {
	d.b = append(d.b, p...)
	return len(p), nil
}
