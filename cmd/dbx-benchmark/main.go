package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

func main() {
	concurrency := 100
	requestsPerWorker := 3000
	totalRequests := concurrency * requestsPerWorker

	fmt.Printf("🚀 Starting DBX High-Concurrency Benchmark...\n")
	fmt.Printf("Concurrency: %d, Total Requests: %d\n", concurrency, totalRequests)

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", "localhost:6399")
			if err != nil {
				fmt.Println("Error connecting to DBX:", err)
				return
			}
			defer conn.Close()

			reader := bufio.NewReader(conn)
			for j := 0; j < requestsPerWorker; j++ {
				key := fmt.Sprintf("benchmark:perf:%d:%d", workerID, j)
				val := "this_is_a_dummy_value_for_benchmarking_speed"

				// Send SET using raw RESP protocol
				req := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)
				conn.Write([]byte(req))

				// Read response (+OK\r\n)
				_, err = reader.ReadString('\n')
				if err != nil {
					break
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)
	opsPerSec := float64(totalRequests) / duration.Seconds()
	avgLatency := float64(duration.Milliseconds()) / float64(totalRequests/concurrency)

	fmt.Printf("\n✅ Benchmark Complete!\n")
	fmt.Printf("----------------------------------\n")
	fmt.Printf("Total Time:    %v\n", duration)
	fmt.Printf("Throughput:    %.2f Ops/sec\n", opsPerSec)
	fmt.Printf("Avg Latency:   %.2f ms\n", avgLatency)
	fmt.Printf("----------------------------------\n")
}
