import { useState, useEffect, useRef } from 'react';
import { Cpu, Activity, Server } from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer
} from 'recharts';
import { fetchWithAuth } from '../api';
import PageChrome from '../components/PageChrome';
import { formatClock, formatDuration, formatMetric } from '../format';

interface ChartPoint { time: string; goroutines: number; ops: number; }

export default function HardwarePage({ clusterId }: { clusterId: string }) {
  const [data, setData] = useState<ChartPoint[]>([]);
  const [cpuCount, setCpuCount] = useState(0);
  const [goroutines, setGoroutines] = useState(0);
  const [uptime, setUptime] = useState(0);
  const [sampledAt, setSampledAt] = useState<string | null>(null);
  const lastOpsRef = useRef(0);
  const lastTimeRef = useRef(Date.now());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (!res.ok) {
          setError(`Failed to fetch metrics: HTTP ${res.status}`);
          return;
        }
        setError(null);
        const d = await res.json();

        setCpuCount(d.cpu_count || 0);
        setGoroutines(d.goroutines || 0);
        setUptime(d.uptime_seconds || 0);

        const nowMs = Date.now();
        const totalOps = d.total_commands ?? 0;
        const dt = (nowMs - lastTimeRef.current) / 1000;

        const opsPerSec = lastOpsRef.current > 0 && dt > 0
          ? Math.max(0, Math.floor((totalOps - lastOpsRef.current) / dt))
          : 0;

        lastOpsRef.current = totalOps;
        lastTimeRef.current = nowMs;
        setSampledAt(formatClock());

        setData(prev => {
          const newData = [...prev, { time: formatClock(), goroutines: d.goroutines || 0, ops: opsPerSec }];
          if (newData.length > 30) return newData.slice(newData.length - 30);
          return newData;
        });
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : 'Network error');
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
        title="Hardware"
        purpose="Go runtime load for this tenant process. Values are a live sample, not a monthly trend."
      />

      {error && <div className="alert-error">{error}</div>}

      <div className="stat-grid" style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}>
        <div className="stat-card">
          <div className="stat-header">Logical cores <div className="stat-icon"><Server size={14} /></div></div>
          <div className="stat-value">{formatMetric(cpuCount)}</div>
          <div className="stat-change">live sample{sampledAt ? ` · ${sampledAt}` : ''}</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">Goroutines <div className="stat-icon"><Activity size={14} /></div></div>
          <div className="stat-value">{formatMetric(goroutines)}</div>
          <div className="stat-change">live sample</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">Engine uptime <div className="stat-icon"><Cpu size={14} /></div></div>
          <div className="stat-value">{formatDuration(uptime)}</div>
          <div className="stat-change">since process start</div>
        </div>
      </div>

      <div className="panel flex-1 min-h-[360px]">
        <div className="panel-header">
          <div className="panel-title">Goroutines</div>
        </div>
        <div className="relative" style={{ height: 320 }}>
          {data.every(d => !d.goroutines) && (
            <div className="chart-idle">No runtime samples yet.</div>
          )}
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 8, right: 8, left: -12, bottom: 4 }}>
              <defs>
                <linearGradient id="gGoroutines" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#c2410c" stopOpacity={0.2} />
                  <stop offset="95%" stopColor="#c2410c" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="var(--border-color)" vertical={false} />
              <XAxis dataKey="time" stroke="var(--text-muted)" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickLine={false} axisLine={false} />
              <YAxis stroke="var(--text-muted)" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} />
              <Tooltip contentStyle={{ background: 'var(--bg-panel)', border: '1px solid var(--border-color)', borderRadius: 6 }} />
              <Area type="monotone" dataKey="goroutines" name="Goroutines" stroke="#c2410c" strokeWidth={1.5} fill="url(#gGoroutines)" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  );
}
