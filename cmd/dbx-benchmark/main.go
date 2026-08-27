package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func auth(conn net.Conn, reader *bufio.Reader, password string) error {
	if password == "" {
		return nil
	}
	cmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return err
	}
	return readReply(reader)
}

func readReply(r *bufio.Reader) error {
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	if len(line) == 0 {
		return nil
	}
	switch line[0] {
	case '+', '-', ':':
		return nil
	case '$':
		n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
		if n < 0 {
			return nil
		}
		_, err := io.CopyN(io.Discard, r, int64(n+2))
		return err
	case '*':
		n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
		for i := 0; i < n; i++ {
			if err := readReply(r); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func runPhase(name string, addr, password string, concurrency, perWorker int, write func(worker, i int) string) {
	total := concurrency * perWorker
	var wg sync.WaitGroup
	var latencyMu sync.Mutex
	var batchLatencies []time.Duration
	start := time.Now()
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				fmt.Println("dial:", err)
				return
			}
			defer conn.Close()
			reader := bufio.NewReaderSize(conn, 32*1024)
			if err := auth(conn, reader, password); err != nil {
				return
			}
			const pipe = 64
			pending := 0
			var batchStart time.Time
			flush := func() {
				if pending == 0 {
					return
				}
				for pending > 0 {
					if err := readReply(reader); err != nil {
						return
					}
					pending--
				}
				latencyMu.Lock()
				batchLatencies = append(batchLatencies, time.Since(batchStart))
				latencyMu.Unlock()
			}
			for j := 0; j < perWorker; j++ {
				if pending == 0 {
					batchStart = time.Now()
				}
				req := write(workerID, j)
				if _, err := conn.Write([]byte(req)); err != nil {
					return
				}
				pending++
				if pending >= pipe {
					flush()
				}
			}
			flush()
		}(w)
	}
	wg.Wait()
	dur := time.Since(start)
	sort.Slice(batchLatencies, func(i, j int) bool { return batchLatencies[i] < batchLatencies[j] })
	fmt.Printf("%s  %d ops in %v -> %.0f ops/sec; pipeline-64 p50=%v p95=%v p99=%v\n",
		name, total, dur.Round(time.Millisecond), float64(total)/dur.Seconds(),
		percentile(batchLatencies, 0.50), percentile(batchLatencies, 0.95), percentile(batchLatencies, 0.99))
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return values[index].Round(time.Microsecond)
}

func main() {
	addr := os.Getenv("DBX_BENCH_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6401"
	}
	password := os.Getenv("DBX_DEFAULT_PASSWORD")
	concurrency := 64
	perWorker := 2000 // 128,000 ops — enough to stabilize vs README 50k sample
	fmt.Printf("DBX RESP benchmark %s  workers=%d  per=%d\n", addr, concurrency, perWorker)
	runPhase("SET", addr, password, concurrency, perWorker, func(w, i int) string {
		key := fmt.Sprintf("b:%d:%d", w, i)
		val := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
		return fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)
	})
	runPhase("GET", addr, password, concurrency, perWorker, func(w, i int) string {
		key := fmt.Sprintf("b:%d:%d", w, i)
		return fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key)
	})
}
