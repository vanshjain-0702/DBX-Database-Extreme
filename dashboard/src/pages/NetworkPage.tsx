import { useState, useEffect } from 'react';
import { Globe, ArrowUpRight, ArrowDownRight, Activity } from 'lucide-react';
import { fetchWithAuth } from '../api';
import PageChrome from '../components/PageChrome';
import { formatClock, formatMetric } from '../format';

export default function NetworkPage({ clusterId }: { clusterId: string }) {
  const [metrics, setMetrics] = useState<Record<string, number>>({});
  const [sampledAt, setSampledAt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (!res.ok) {
          setError(`HTTP ${res.status}`);
          return;
        }
        setError(null);
        setMetrics(await res.json());
        setSampledAt(formatClock());
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Network error');
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [clusterId]);

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Network"
        purpose="Command I/O counters for this tenant. Live sample, not a fabricated trend."
      />

      {error && <div className="alert-error">{error}</div>}
      {sampledAt && <p className="text-[11px] font-mono text-[var(--text-muted)] -mt-2">Last sample {sampledAt}</p>}

      <div className="stat-grid">
        <div className="stat-card">
          <div className="stat-header">Read ops <div className="stat-icon"><ArrowUpRight size={14} /></div></div>
          <div className="stat-value">{formatMetric(metrics.total_reads || 0)}</div>
          <div className="stat-change">cumulative</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">Write ops <div className="stat-icon"><ArrowDownRight size={14} /></div></div>
          <div className="stat-value">{formatMetric(metrics.total_writes || 0)}</div>
          <div className="stat-change">cumulative</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">TCP conns <div className="stat-icon"><Globe size={14} /></div></div>
          <div className="stat-value">{formatMetric(metrics.active_conns || 0)}</div>
          <div className="stat-change">live sample</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">Avg latency <div className="stat-icon"><Activity size={14} /></div></div>
          <div className="stat-value">{((metrics.avg_latency_ns || 0) / 1_000_000).toFixed(2)}<span className="text-[16px] text-[var(--text-muted)]"> ms</span></div>
          <div className="stat-change">command average</div>
        </div>
      </div>
    </div>
  );
}
