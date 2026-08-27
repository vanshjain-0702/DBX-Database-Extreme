import { useState, useEffect, useRef } from 'react';
import { Server, Wifi, HardDrive, Activity, BarChart2, RefreshCw } from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer
} from 'recharts';
import { fetchWithAuth } from '../api';
import PageChrome from '../components/PageChrome';
import { formatClock, formatDuration, formatMemory, formatMetric } from '../format';

interface ChartPoint { time: string; ops: number; }

const MAX_DATA_POINTS = 60;

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: { name: string; value: number; stroke?: string }[]; label?: string }) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md px-3 py-2 text-[12px]">
      <p className="text-[var(--text-muted)] font-mono text-[11px] mb-1">{label}</p>
      {payload.map((p, i) => (
        <p key={i} style={{ color: p.stroke }} className="font-mono font-medium">
          {p.name}: {Math.round(p.value)}
        </p>
      ))}
    </div>
  );
};

export default function HostingPerformancePage({ clusterId }: { clusterId: string }) {
  const [chartData, setChartData] = useState<ChartPoint[]>([]);
  const [liveOps, setLiveOps] = useState(0);
  const [liveMemMB, setLiveMemMB] = useState(0);
  const [liveConns, setLiveConns] = useState(0);
  const [liveLatency, setLiveLatency] = useState(0);
  const [totalCmds, setTotalCmds] = useState(0);
  const [uptime, setUptime] = useState(0);
  const [sampledAt, setSampledAt] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [unreachable, setUnreachable] = useState(false);

  const lastOpsRef = useRef(0);
  const lastTimeRef = useRef(Date.now());

  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (!res.ok) {
          setUnreachable(true);
          return;
        }
        setUnreachable(false);
        const d = await res.json();

        const nowMs = Date.now();
        const totalOps = d.total_commands ?? d.dbx_commands_total ?? 0;
        const dt = (nowMs - lastTimeRef.current) / 1000;

        const opsPerSec = lastOpsRef.current > 0 && dt > 0
          ? Math.max(0, Math.floor((totalOps - lastOpsRef.current) / dt))
          : 0;

        lastOpsRef.current = totalOps;
        lastTimeRef.current = nowMs;

        setLiveOps(opsPerSec);
        setLiveMemMB((d.memory_used_bytes ?? 0) / 1024 / 1024);
        setLiveConns(d.active_conns ?? 0);
        setLiveLatency((d.avg_latency_ns ?? 0) / 1_000_000);
        setTotalCmds(totalOps);
        setUptime(d.uptime_seconds ?? 0);
        setSampledAt(formatClock());

        setChartData(prev => {
          const newData = [...prev, { time: formatClock(), ops: opsPerSec }];
          if (newData.length > MAX_DATA_POINTS) {
            return newData.slice(newData.length - MAX_DATA_POINTS);
          }
          return newData;
        });
      } catch {
        setUnreachable(true);
      }
    };

    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [clusterId]);

  const doRefresh = () => {
    setRefreshing(true);
    setTimeout(() => setRefreshing(false), 400);
  };

  const mem = formatMemory(liveMemMB);
  const idle = chartData.every(d => !d.ops);

  const stats = [
    { label: 'Uptime', value: formatDuration(uptime), note: 'process uptime', icon: <Activity size={14} /> },
    { label: 'Active conns', value: formatMetric(liveConns), note: liveConns ? 'connected' : 'none', icon: <Server size={14} /> },
    { label: 'Ops / sec', value: formatMetric(liveOps), note: 'live sample', icon: <Wifi size={14} /> },
    { label: 'Memory', value: mem.label, note: 'heap allocation', icon: <HardDrive size={14} /> },
    { label: 'Total commands', value: formatMetric(totalCmds), note: 'cumulative', icon: <BarChart2 size={14} /> },
  ];

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Hosting"
        purpose="Application metrics from this tenant engine. No invented month-over-month deltas."
        extra={
          <button type="button" className="btn-secondary" onClick={doRefresh}>
            <RefreshCw size={13} className={refreshing ? 'animate-spin' : ''} />
            Refresh
          </button>
        }
      />

      {unreachable && <div className="banner-down">Engine unreachable. Live sample paused.</div>}
      {sampledAt && !unreachable && (
        <p className="text-[11px] font-mono text-[var(--text-muted)] -mt-2">Last sample {sampledAt}</p>
      )}

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><BarChart2 size={14} /> Resource summary</div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-5 divide-x divide-[var(--border-color)]">
          {stats.map(s => (
            <div key={s.label} className="px-4 py-4">
              <div className="flex justify-between items-center mb-2">
                <span className="page-label">{s.label}</span>
                <span className="text-[var(--text-muted)]">{s.icon}</span>
              </div>
              <div className="stat-value text-[22px]">{s.value}</div>
              <div className="stat-change mt-1">{s.note}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title">Operations / sec</div>
        </div>
        <div className="relative px-3 pt-3 pb-2" style={{ height: 260 }}>
          {idle && <div className="chart-idle">No commands in the sampling window.</div>}
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 4, right: 4, left: -12, bottom: 0 }}>
              <defs>
                <linearGradient id="gOpsHP" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#c2410c" stopOpacity={0.25} />
                  <stop offset="95%" stopColor="#c2410c" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="var(--border-color)" vertical={false} />
              <XAxis dataKey="time" stroke="var(--text-muted)" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickLine={false} axisLine={false} interval={Math.max(0, Math.floor(chartData.length / 6))} />
              <YAxis stroke="var(--text-muted)" tick={{ fill: 'var(--text-muted)', fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} />
              <Tooltip content={<CustomTooltip />} />
              <Area type="monotone" dataKey="ops" name="ops" stroke="#c2410c" strokeWidth={1.5} fill="url(#gOpsHP)" dot={false} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="stat-grid" style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}>
        <div className="stat-card">
          <div className="stat-header">Avg latency</div>
          <div className="stat-value">{liveLatency.toFixed(2)}<span className="text-[16px] text-[var(--text-muted)]">ms</span></div>
          <div className="stat-change">live sample</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">Ops / sec</div>
          <div className="stat-value">{formatMetric(liveOps)}</div>
          <div className="stat-change">live sample</div>
        </div>
        <div className="stat-card">
          <div className="stat-header">Memory</div>
          <div className="stat-value">{mem.label}</div>
          <div className="stat-change">heap allocation</div>
        </div>
      </div>
    </div>
  );
}
