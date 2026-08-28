// Package observability provides metrics, tracing, logging, and alerting.
package observability

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

var StartTime = time.Now()

// Metrics holds all server-level counters and gauges.
type Metrics struct {
	// Counters
	TotalCommands     atomic.Int64
	TotalReads        atomic.Int64
	TotalWrites       atomic.Int64
	TotalErrors       atomic.Int64
	TotalExpired      atomic.Int64
	TotalEvicted      atomic.Int64
	TotalBytes        atomic.Int64
	ActiveConns       atomic.Int64
	PubSubMessages    atomic.Int64
	TenantMemoryUsed  atomic.Int64
	TenantMemoryLimit atomic.Int64
	TenantReady       atomic.Int64

	// Latency histograms (simplified as sum + count)
	CmdLatencySum   atomic.Int64
	CmdLatencyCount atomic.Int64
}

// Global metrics instance.
var Global = &Metrics{}

// RecordCommand records a command execution.
func (m *Metrics) RecordCommand(isWrite bool, latencyNs int64, err error) {
	m.TotalCommands.Add(1)
	if isWrite {
		m.TotalWrites.Add(1)
	} else {
		m.TotalReads.Add(1)
	}
	if err != nil {
		m.TotalErrors.Add(1)
	}
	m.CmdLatencySum.Add(latencyNs)
	m.CmdLatencyCount.Add(1)
}

// AvgLatencyNs returns the average command latency in nanoseconds.
func (m *Metrics) AvgLatencyNs() int64 {
	count := m.CmdLatencyCount.Load()
	if count == 0 {
		return 0
	}
	return m.CmdLatencySum.Load() / count
}

func (m *Metrics) Snapshot() map[string]int64 {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return map[string]int64{
		"total_commands":            m.TotalCommands.Load(),
		"total_reads":               m.TotalReads.Load(),
		"total_writes":              m.TotalWrites.Load(),
		"total_errors":              m.TotalErrors.Load(),
		"total_expired":             m.TotalExpired.Load(),
		"total_evicted":             m.TotalEvicted.Load(),
		"active_conns":              m.ActiveConns.Load(),
		"pubsub_messages":           m.PubSubMessages.Load(),
		"tenant_memory_used_bytes":  m.TenantMemoryUsed.Load(),
		"tenant_memory_limit_bytes": m.TenantMemoryLimit.Load(),
		"tenant_ready":              m.TenantReady.Load(),
		"avg_latency_ns":            m.AvgLatencyNs(),
		"memory_used_bytes":         int64(memory.Alloc),
		"memory_sys_bytes":          int64(memory.Sys),
		"heap_objects":              int64(memory.HeapObjects),
		"gc_pause_total_ns":         int64(memory.PauseTotalNs),
		"goroutines":                int64(runtime.NumGoroutine()),
		"cpu_count":                 int64(runtime.NumCPU()),
		"uptime_seconds":            int64(time.Since(StartTime).Seconds()),
	}
}

// WritePrometheus emits unlabeled or tenant-labeled gauges from a snapshot.
func WritePrometheus(buf *strings.Builder, tenantID string, snap map[string]int64) {
	label := ""
	if tenantID != "" {
		label = `{tenant="` + tenantID + `"}`
	}
	for key, value := range snap {
		metric := "dbx_" + key
		if label == "" {
			fmt.Fprintf(buf, "%s %d\n", metric, value)
			continue
		}
		fmt.Fprintf(buf, "%s%s %d\n", metric, label, value)
	}
}
