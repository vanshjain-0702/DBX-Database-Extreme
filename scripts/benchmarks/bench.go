package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {
	fmt.Println("=== DBX NATIVE TCP RESP BENCHMARK ===")
	address := "127.0.0.1:6401"
	
	cert, err := tls.LoadX509KeyPair("./certs/client.crt", "./certs/client.key")
	if err != nil {
		fmt.Println("No client cert found:", err)
	}

	caCert, _ := os.ReadFile("./certs/ca.crt")
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	conf := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}

	concurrency := 100
	numRequests := 100000 // 100k strings
	
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(concurrency)
	
	reqsPerRoutine := numRequests / concurrency
	
	for i := 0; i < concurrency; i++ {
		go func(routineID int) {
			defer wg.Done()
			conn, err := tls.Dial("tcp", address, conf)
			if err != nil {
				fmt.Println("Dial error:", err)
				return
			}
			defer conn.Close()
			
			buf := make([]byte, 256)
			
			for j := 0; j < reqsPerRoutine; j++ {
				key := "k:" + strconv.Itoa(routineID) + ":" + strconv.Itoa(j)
				val := "v"
				cmd := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)
				conn.Write([]byte(cmd))
				conn.Read(buf)
			}
		}(i)
	}
	wg.Wait()
	duration := time.Since(start).Seconds()
	fmt.Printf("[STRING] %d SETs: %.2f seconds -> %.0f ops/sec\n", numRequests, duration, float64(numRequests)/duration)

    // GET Benchmark
    start = time.Now()
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(routineID int) {
			defer wg.Done()
			conn, err := tls.Dial("tcp", address, conf)
			if err != nil { return }
			defer conn.Close()
			buf := make([]byte, 256)
			
			for j := 0; j < reqsPerRoutine; j++ {
				key := "k:" + strconv.Itoa(routineID) + ":" + strconv.Itoa(j)
				cmd := fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key)
				conn.Write([]byte(cmd))
				conn.Read(buf)
			}
		}(i)
	}
	wg.Wait()
	duration = time.Since(start).Seconds()
	fmt.Printf("[STRING] %d GETs: %.2f seconds -> %.0f ops/sec\n", numRequests, duration, float64(numRequests)/duration)

    // Vector Benchmark
    vecRequests := 1000000
    vecPerRoutine := vecRequests / concurrency
    
    start = time.Now()
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(routineID int) {
			defer wg.Done()
			conn, err := tls.Dial("tcp", address, conf)
			if err != nil { return }
			defer conn.Close()
			buf := make([]byte, 256)
			
            var vStr string
            for k:=0; k<128; k++ {
                vStr += fmt.Sprintf("$4\r\n0.50\r\n")
            }
			
			for j := 0; j < vecPerRoutine; j++ {
				docID := "vdoc:" + strconv.Itoa(routineID) + ":" + strconv.Itoa(j)
				header := fmt.Sprintf("*131\r\n$4\r\nVADD\r\n$9\r\nbench_idx\r\n$%d\r\n%s\r\n", len(docID), docID)
                cmd := header + vStr
				conn.Write([]byte(cmd))
				conn.Read(buf)
			}
		}(i)
	}
	wg.Wait()
	duration = time.Since(start).Seconds()
	fmt.Printf("[VECTOR] %d VADDs (128-dim): %.2f seconds -> %.0f ops/sec\n", vecRequests, duration, float64(vecRequests)/duration)
}
