package engine

import (
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
	for i := 0; i < 8; i++ {
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
	if max > 50*time.Millisecond {
		t.Fatalf("quiet GET p-worst %s exceeded isolation budget", max)
	}
}
