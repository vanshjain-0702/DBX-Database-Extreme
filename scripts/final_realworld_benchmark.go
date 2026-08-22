//go:build ignore
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	NumStrings = 1000000
	NumHashes  = 1000000
	NumVectors = 15000
	VectorDim  = 768
	Workers    = 200
)

type QueryReq struct {
	Command []string `json:"command"`
}

func main() {
	baseURL := "https://localhost:8000"

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 5 * time.Second,
	}

	// 1. Login
	loginBody := `{"username":"admin", "password":"adminadminadmin"}`
	resp, err := client.Post(baseURL+"/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil || resp.StatusCode != 200 {
		fmt.Printf("Login failed: %v\n", err)
		os.Exit(1)
	}
	var loginRes struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&loginRes)
	resp.Body.Close()
	token := loginRes.Token

	// 2. Provision Tenant
	provBody := `{"id":"final-bench", "name":"Final Real World Bench"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/provision", strings.NewReader(provBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, _ := client.Do(req)
	if res != nil {
		res.Body.Close()
	}

	fmt.Println("🚀 Starting Final Real World Benchmark...")
	
	var successOps int64
	var failOps int64
	
	// Reuse existing client for benchmark phase

	runPhase := func(name string, total int, genCmd func(i int) []string) {
		fmt.Printf("▶ Phase: %s (%d items)\n", name, total)
		start := time.Now()
		
		ch := make(chan int, 10000)
		var wg sync.WaitGroup
		
		for w := 0; w < Workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range ch {
					cmd := genCmd(i)
					qReq := QueryReq{Command: cmd}
					bodyBytes, _ := json.Marshal(qReq)
					
					req, _ := http.NewRequest("POST", baseURL+"/t/final-bench/query", bytes.NewReader(bodyBytes))
					req.Header.Set("Authorization", "Bearer "+token)
					req.Header.Set("Content-Type", "application/json")
					
					res, err := client.Do(req)
					if err != nil {
						atomic.AddInt64(&failOps, 1)
						continue
					}
					io.Copy(io.Discard, res.Body)
					res.Body.Close()
					
					if res.StatusCode == 200 {
						atomic.AddInt64(&successOps, 1)
					} else {
						atomic.AddInt64(&failOps, 1)
					}
				}
			}()
		}
		
		for i := 0; i < total; i++ {
			ch <- i
		}
		close(ch)
		wg.Wait()
		
		dur := time.Since(start)
		fmt.Printf("✅ %s completed in %v (%.2f ops/sec)\n", name, dur, float64(total)/dur.Seconds())
	}

	// Phase 1: Strings
	runPhase("1M Strings (SET)", NumStrings, func(i int) []string {
		return []string{"SET", fmt.Sprintf("str:%d", i), fmt.Sprintf("static_data_payload_for_key_%d", i)}
	})

	// Phase 2: Hashes
	runPhase("1M Hashes (HSET)", NumHashes, func(i int) []string {
		return []string{"HSET", fmt.Sprintf("hash:%d", i), "field", fmt.Sprintf("val_%d", i)}
	})

	// Phase 3: Vectors
	runPhase("15K Vectors (VSET)", NumVectors, func(i int) []string {
		vec := make([]string, VectorDim)
		for j := 0; j < VectorDim; j++ {
			vec[j] = fmt.Sprintf("%.4f", rand.Float32())
		}
		vecStr := strings.Join(vec, ",")
		return []string{"VSET", fmt.Sprintf("vec:%d", i), vecStr}
	})

	fmt.Println("-------------------------------------------------")
	fmt.Printf("🎉 Benchmark Complete!\n")
	fmt.Printf("Successful Ops: %d\n", successOps)
	fmt.Printf("Failed Ops: %d\n", failOps)
}




