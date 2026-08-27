package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/dbx/dbx/internal/engine"
)

func main() {
	count := flag.Int("count", 100000, "number of vectors")
	dim := flag.Int("dim", 128, "vector dimensions")
	queries := flag.Int("queries", 25, "deterministic recall queries")
	k := flag.Int("k", 10, "neighbors per query")
	flag.Parse()
	if *count <= 0 || *dim <= 0 || *queries <= 0 || *k <= 0 {
		panic("all benchmark parameters must be positive")
	}
	dir, err := os.MkdirTemp("", "dbx-vector-benchmark-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	store := engine.NewVectorStore(engine.New(64), dir, *count)
	defer store.CloseAll()

	rng := rand.New(rand.NewSource(42))
	vectors := make([][]float32, *count)
	for i := range vectors {
		vectors[i] = randomUnitVector(rng, *dim)
	}
	start := time.Now()
	const batchSize = 1000
	for startRow := 0; startRow < *count; startRow += batchSize {
		end := min(startRow+batchSize, *count)
		ids := make([]string, end-startRow)
		for row := startRow; row < end; row++ {
			ids[row-startRow] = "v" + strconv.Itoa(row)
		}
		if err := store.VAddBatch("recall", *dim, ids, vectors[startRow:end]); err != nil {
			panic(err)
		}
	}
	ingestDuration := time.Since(start)

	// Warm the mmap and search scratch pools before measuring latency.
	for i := 0; i < 25; i++ {
		query := vectors[(i*7919)%*count]
		if _, err := store.VSearch("recall", query, *k, nil); err != nil {
			panic(err)
		}
	}

	latencies := make([]time.Duration, 0, *queries)
	recalls := make([]float64, 0, *queries)
	for i := 0; i < *queries; i++ {
		queryRow := (i * 7919) % *count
		query := vectors[queryRow]
		expected := bruteForce(query, vectors, *k)
		searchStart := time.Now()
		results, err := store.VSearch("recall", query, *k, nil)
		latencies = append(latencies, time.Since(searchStart))
		if err != nil {
			panic(err)
		}
		hits := 0
		for _, result := range results {
			row, _ := strconv.Atoi(result.ID[1:])
			if expected[row] {
				hits++
			}
		}
		recalls = append(recalls, float64(hits)/float64(*k))
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	sort.Float64s(recalls)
	var recallSum float64
	for _, recall := range recalls {
		recallSum += recall
	}
	fmt.Printf("vectors=%d dim=%d queries=%d k=%d\n", *count, *dim, *queries, *k)
	fmt.Printf("ingest_vectors_per_sec=%.0f ingest_duration=%s\n", float64(*count)/ingestDuration.Seconds(), ingestDuration.Round(time.Millisecond))
	fmt.Printf("search_p50=%.3fms search_p95=%.3fms search_p99=%.3fms\n",
		ms(percentile(latencies, 0.50)), ms(percentile(latencies, 0.95)), ms(percentile(latencies, 0.99)))
	fmt.Printf("recall_mean=%.3f recall_p05=%.3f\n",
		recallSum/float64(len(recalls)), recalls[int(math.Floor(float64(len(recalls)-1)*0.05))])
}

func randomUnitVector(rng *rand.Rand, dim int) []float32 {
	vector := make([]float32, dim)
	var norm float64
	for i := range vector {
		vector[i] = rng.Float32()*2 - 1
		norm += float64(vector[i] * vector[i])
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vector {
		vector[i] *= scale
	}
	return vector
}

func bruteForce(query []float32, vectors [][]float32, k int) map[int]bool {
	type scored struct {
		row   int
		score float32
	}
	top := make([]scored, 0, k)
	for row, vector := range vectors {
		var score float32
		for i := range query {
			score += query[i] * vector[i]
		}
		if len(top) < k {
			top = append(top, scored{row: row, score: score})
			sort.Slice(top, func(i, j int) bool { return top[i].score > top[j].score })
		} else if score > top[len(top)-1].score {
			top[len(top)-1] = scored{row: row, score: score}
			sort.Slice(top, func(i, j int) bool { return top[i].score > top[j].score })
		}
	}
	result := make(map[int]bool, len(top))
	for _, item := range top {
		result[item.row] = true
	}
	return result
}

func percentile(values []time.Duration, p float64) time.Duration {
	index := int(math.Ceil(float64(len(values))*p)) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
