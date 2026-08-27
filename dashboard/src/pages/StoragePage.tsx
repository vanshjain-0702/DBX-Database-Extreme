import { useState, useEffect } from 'react';
import { HardDrive, Database, Layers } from 'lucide-react';
import { fetchWithAuth } from '../api';
import PageChrome from '../components/PageChrome';
import { formatClock, formatMemory, formatMetric } from '../format';

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

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Storage"
        purpose="Heap and GC telemetry from this tenant process. Live sample only."
      />

      {error && <div className="alert-error">{error}</div>}

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Database size={14} /> Memory allocation</div>
          {sampledAt && <span className="text-[11px] font-mono text-[var(--text-muted)]">sampled {sampledAt}</span>}
        </div>
        <div className="panel-body">
          <div className="relative w-full h-2 bg-[var(--bg-primary)] rounded-sm overflow-hidden border border-[var(--border-color)]">
            <div
              className="absolute top-0 left-0 h-full bg-[var(--accent-primary)] transition-all duration-150"
              style={{ width: `${memPct}%` }}
            />
          </div>
          <div className="flex justify-between mt-2.5 text-[12px] font-mono">
            <span className="text-[var(--text-muted)]">Allocated {usedLabel}</span>
            <span>{memPct.toFixed(1)}%</span>
            <span className="text-[var(--text-muted)]">Sys {sysLabel}</span>
          </div>
        </div>
      </div>

      <div className="stat-grid" style={{ gridTemplateColumns: 'repeat(2, 1fr)' }}>
        <div className="stat-card">
          <div className="stat-header">Heap objects <div className="stat-icon"><Layers size={14} /></div></div>
          <div className="stat-value">{formatMetric(metrics.heap_objects || 0)}</div>
          <div className="stat-change">live sample</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">GC pause total <div className="stat-icon"><HardDrive size={14} /></div></div>
          <div className="stat-value">{((metrics.gc_pause_total_ns || 0) / 1_000_000).toFixed(2)}<span className="text-[16px] text-[var(--text-muted)]"> ms</span></div>
          <div className="stat-change">cumulative</div>
        </div>
      </div>
    </div>
  );
}
