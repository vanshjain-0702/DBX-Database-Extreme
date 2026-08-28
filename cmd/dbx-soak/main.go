// Command dbx-soak runs the operator density drill: idle GET latency
// while other tenants write. Default is the certified 100 idle / 25 active
// profile. CI uses the smaller Go test instead (12/4).
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/protocol"
)

func main() {
	idleN := flag.Int("idle", 100, "idle tenants")
	activeN := flag.Int("active", 25, "tenants writing concurrently")
	seconds := flag.Duration("for", 3*time.Second, "how long to soak")
	budget := flag.Duration("budget", 250*time.Millisecond, "max idle GET")
	flag.Parse()

	stores := make([]*engine.KVStore, *idleN+*activeN)
	for i := range stores {
		stores[i] = engine.New(8)
		stores[i].Set("keep", []byte("ok"), protocol.TypeString, 0)
	}
	idle, active := stores[:*idleN], stores[*idleN:]
	stop := make(chan struct{})
	var wg sync.WaitGroup
	payload := make([]byte, 128)
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
					n++
					if n%64 == 0 {
						runtime.Gosched()
					}
				}
			}
		}()
	}

	deadline := time.Now().Add(*seconds)
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
				fmt.Fprintln(os.Stderr, "FAIL idle key disappeared")
				os.Exit(1)
			}
			if elapsed > max {
				max = elapsed
			}
		}
	}
	close(stop)
	wg.Wait()
	fmt.Printf("soak idle=%d active=%d gets=%d worst_idle_get=%s budget=%s\n",
		*idleN, *activeN, ops, max, *budget)
	if max > *budget {
		fmt.Fprintf(os.Stderr, "FAIL worst idle GET exceeded budget\n")
		os.Exit(1)
	}
	fmt.Println("PASS")
}
