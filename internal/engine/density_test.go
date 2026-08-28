package engine

import (
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/protocol"
)

func TestNoisyNeighborWritesDoNotBlockQuietReads(t *testing.T) {
	noisy := New(16)
	quiet := New(16)
	quiet.Set("keep", []byte("ok"), protocol.TypeString, 0)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := make([]byte, 256)
			for {
				select {
				case <-stop:
					return
				default:
					noisy.Set("n", payload, protocol.TypeString, 0)
					runtime.Gosched()
				}
			}
		}()
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	var max time.Duration
	for time.Now().Before(deadline) {
		start := time.Now()
		entry := quiet.Get("keep")
		elapsed := time.Since(start)
		if entry == nil {
			close(stop)
			wg.Wait()
			t.Fatal("quiet key disappeared")
		}
		if elapsed > max {
			max = elapsed
		}
	}
	close(stop)
	wg.Wait()
	if max > 200*time.Millisecond {
		t.Fatalf("quiet GET p-worst %s exceeded isolation budget", max)
	}
}

func TestDensitySoakIdleReadsStayBounded(t *testing.T) {
	idleN, activeN := 12, 4
	if os.Getenv("DBX_SOAK_FULL") == "1" {
		idleN, activeN = 100, 25
	}
	stores := make([]*KVStore, idleN+activeN)
	for i := range stores {
		stores[i] = New(8)
		stores[i].Set("keep", []byte("ok"), protocol.TypeString, 0)
	}
	idle, active := stores[:idleN], stores[idleN:]
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
	deadline := time.Now().Add(400 * time.Millisecond)
	var max time.Duration
	for time.Now().Before(deadline) {
		for _, quiet := range idle {
			start := time.Now()
			entry := quiet.Get("keep")
			elapsed := time.Since(start)
			if entry == nil {
				close(stop)
				wg.Wait()
				t.Fatal("idle tenant key disappeared")
			}
			if elapsed > max {
				max = elapsed
			}
		}
	}
	close(stop)
	wg.Wait()
	if max > 250*time.Millisecond {
		t.Fatalf("idle GET p-worst %s exceeded density soak budget (%d idle / %d active)", max, idleN, activeN)
	}
}
