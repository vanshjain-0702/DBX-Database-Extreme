import { useState, useEffect } from 'react';
import { HardDrive, Database, Layers, Activity } from 'lucide-react';
import { fetchWithAuth } from '../api';
import PageChrome from '../components/PageChrome';
import { formatClock, formatMemory, formatMetric } from '../format';

function getMemBarColor(pct: number) {
  if (pct < 60) return 'linear-gradient(90deg, #059669, #10b981)';
  if (pct < 80) return 'linear-gradient(90deg, #d97706, #f59e0b)';
  return 'linear-gradient(90deg, #e11d48, #f43f5e)';
}

function getMemBarGlow(pct: number) {
  if (pct < 60) return 'rgba(5,150,105,0.3)';
  if (pct < 80) return 'rgba(217,119,6,0.3)';
  return 'rgba(225,29,72,0.3)';
}

export default function StoragePage({ clusterId }: { clusterId: string }) {
  const [metrics, setMetrics] = useState<Record<string, number>>({});
  const [error, setError] = useState<string | null>(null);
  const [sampledAt, setSampledAt] = useState<string | null>(null);

  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (!res.ok) {
          setError(`Failed to fetch storage metrics: HTTP ${res.status}`);
          return;
        }
        setError(null);
        setMetrics(await res.json());
        setSampledAt(formatClock());
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : 'Network error');
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [clusterId]);

  const memUsed = (metrics.memory_used_bytes || 0) / 1024 / 1024;
  const memSys = (metrics.memory_sys_bytes || 0) / 1024 / 1024;
  const memPct = memSys > 0 ? Math.min(100, (memUsed / memSys) * 100) : 0;
  const usedLabel = formatMemory(memUsed).label;
  const sysLabel = formatMemory(memSys).label;

  const heapObjects = metrics.heap_objects || 0;
  const gcPauseMs = ((metrics.gc_pause_total_ns || 0) / 1_000_000).toFixed(2);

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Storage"
        purpose="Heap and GC telemetry from this tenant process. Live sample only."
      />

      {error && <div className="alert-error">{error}</div>}

      {/* Memory allocation panel */}
      <div className="panel" style={{ animation: 'fade-in-up 0.35s ease both', animationDelay: '0.05s' }}>
        <div className="panel-header">
          <div className="panel-title">
            <Database size={14} style={{ color: 'var(--accent-primary)' }} />
            Memory allocation
          </div>
          {sampledAt && (
            <span className="flex items-center gap-1.5 text-[11px] font-mono text-[var(--text-muted)]">
              <Activity size={11} className="opacity-60" />
              sampled {sampledAt}
            </span>
          )}
        </div>
        <div className="panel-body">
          {/* Gradient memory bar */}
          <div className="memory-bar-track">
            <div
              className="memory-bar-fill"
              style={{
                width: `${memPct}%`,
                background: getMemBarColor(memPct),
                boxShadow: `0 0 8px ${getMemBarGlow(memPct)}`,
              }}
            />
          </div>

          {/* Labels */}
          <div className="flex justify-between mt-3 text-[12px] font-mono">
            <span className="text-[var(--text-muted)]">
              Allocated <span className="text-[var(--text-primary)] font-semibold">{usedLabel}</span>
            </span>
            <span
              className="font-bold tabular-nums"
              style={{
                color: memPct < 60 ? 'var(--success)' : memPct < 80 ? 'var(--warning)' : 'var(--error)',
              }}
            >
              {memPct.toFixed(1)}%
            </span>
            <span className="text-[var(--text-muted)]">
              Sys <span className="text-[var(--text-primary)] font-semibold">{sysLabel}</span>
            </span>
          </div>

          {/* Usage bar detail */}
          <div
            className="mt-4 flex items-center gap-3 text-[12px] px-3 py-2.5 rounded-lg"
            style={{
              background: 'var(--bg-tertiary)',
              border: '1px solid var(--border-color)',
            }}
          >
            <div
              className="w-2.5 h-2.5 rounded-full flex-shrink-0"
              style={{ background: getMemBarColor(memPct).split(',')[1]?.trim().split(')')[0] + ')' || '#10b981' }}
            />
            <span style={{ color: 'var(--text-muted)' }}>
              {memPct < 60 ? 'Memory pressure is normal.' : memPct < 80 ? 'Memory usage is elevated.' : 'Memory usage is high — consider hibernating idle tenants.'}
            </span>
          </div>
        </div>
      </div>

      {/* Stat cards */}
      <div className="stat-grid" style={{ gridTemplateColumns: 'repeat(2, 1fr)' }}>
        <div className="stat-card" style={{ animationDelay: '0.10s' }}>
          <div className="stat-header">
            Heap objects
            <div
              className="stat-icon"
              style={{
                background: 'rgba(59,130,246,0.12)',
                border: '1px solid rgba(59,130,246,0.2)',
                color: '#3b82f6',
              }}
            >
              <Layers size={14} />
            </div>
          </div>
          <div className="stat-value">{formatMetric(heapObjects)}</div>
          <div className="stat-change neutral">live sample</div>
        </div>

        <div className="stat-card" style={{ animationDelay: '0.15s' }}>
          <div className="stat-header">
            GC pause total
            <div
              className="stat-icon"
              style={{
                background: 'rgba(168,85,247,0.12)',
                border: '1px solid rgba(168,85,247,0.2)',
                color: '#a855f7',
              }}
            >
              <HardDrive size={14} />
            </div>
          </div>
          <div className="stat-value">
            {gcPauseMs}
            <span className="text-[16px] text-[var(--text-muted)] ml-1">ms</span>
          </div>
          <div className="stat-change neutral">cumulative GC time</div>
        </div>
      </div>
    </div>
  );
}
