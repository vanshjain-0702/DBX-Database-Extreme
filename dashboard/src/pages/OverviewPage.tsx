import { useState, useEffect, useRef } from 'react';
import { Zap, HardDrive, Users, Activity, BarChart2, CloudUpload } from 'lucide-react';
import {
  XAxis, YAxis, CartesianGrid, Tooltip,
  ResponsiveContainer, AreaChart, Area, PieChart, Pie, Cell
} from 'recharts';
import { fetchWithAuth } from '../api';
import { useToast } from '../components/Toaster';
import { useTenant } from '../components/TenantProvider';
import PageChrome from '../components/PageChrome';
import { formatAxis, formatClock, formatMemory } from '../format';
import gsap from 'gsap';

const initMetrics = () => Array.from({ length: 15 }).map(() => ({
  time: '',
  ops: 0,
  latency: 0,
  memory: 0,
  connections: 0,
  totalCommands: 0,
}));

const TYPE_COLORS: Record<string, string> = {
  String: '#2563eb', Hash: '#4f46e5', List: '#7c3aed',
  ZSet: '#0284c7', Set: '#059669', JSON: '#d97706',
  Geo: '#0d9488', Stream: '#ea580c', Bitmap: '#65a30d',
  Vector: '#be185d', Snapshot: '#9333ea'
};

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: { name: string; value: number; color?: string; stroke?: string }[]; label?: string }) => {
  if (active && payload && payload.length) {
    return (
      <div style={{ background: 'var(--bg-panel)', border: '1px solid var(--border-color)', borderRadius: 6, padding: '10px 12px' }}>
        <p style={{ color: 'var(--text-muted)', fontSize: 11, marginBottom: 6, fontFamily: 'var(--font-mono)' }}>{label}</p>
        {payload.map((p, i) => (
          <p key={i} style={{ color: p.color || p.stroke, fontSize: 12, fontWeight: 600, fontFamily: 'var(--font-mono)' }}>
            {p.name}: {p.value.toLocaleString()}{p.name === 'latency' ? 'ms' : p.name === 'memory' ? 'MB' : ''}
          </p>
        ))}
      </div>
    );
  }
  return null;
};

function TweenedNumber({ value, prefix = '', suffix = '' }: { value: number; prefix?: string; suffix?: string }) {
  const nodeRef = useRef<HTMLSpanElement>(null);
  const valRef = useRef({ val: 0 });

  useEffect(() => {
    gsap.to(valRef.current, {
      val: value,
      duration: 0.35,
      ease: 'power1.out',
      onUpdate: () => {
        if (nodeRef.current) {
          const n = valRef.current.val;
          let formatted: string;
          if (Math.abs(n) < 1000) {
            formatted = n % 1 !== 0 ? n.toFixed(2) : String(Math.round(n));
          } else if (Math.abs(n) < 1_000_000) {
            formatted = (n / 1000).toFixed(1) + 'k';
          } else {
            formatted = (n / 1_000_000).toFixed(2) + 'M';
          }
          nodeRef.current.innerText = prefix + formatted + suffix;
        }
      }
    });
  }, [value, prefix, suffix]);

  return <span ref={nodeRef}>{prefix}0{suffix}</span>;
}

export default function OverviewPage({ clusterId }: { clusterId: string }) {
  const [metrics, setMetrics] = useState(initMetrics());
  const [keyspaceData, setKeyspaceData] = useState<{ name: string; value: number; color: string }[]>([]);
  const [isBackingUp, setIsBackingUp] = useState(false);
  const [metricsOk, setMetricsOk] = useState(true);
  const toast = useToast();
  const { tenant } = useTenant(clusterId);
  const down = tenant?.status === 'down';

  const lastOpsRef = useRef<number>(0);
  const lastTimeRef = useRef<number>(Date.now());

  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (!res.ok) {
          setMetricsOk(false);
          return;
        }
        setMetricsOk(true);
        const data = await res.json();
        const now = new Date();
        const nowMs = now.getTime();

        const currentTotalOps = data.total_commands ?? data.dbx_commands_total ?? 0;
        let opsPerSec = 0;
        if (lastOpsRef.current > 0) {
          const timeDiffSec = (nowMs - lastTimeRef.current) / 1000;
          const opsDiff = currentTotalOps - lastOpsRef.current;
          opsPerSec = timeDiffSec > 0 ? Math.max(0, Math.floor(opsDiff / timeDiffSec)) : 0;
        }

        lastOpsRef.current = currentTotalOps;
        lastTimeRef.current = nowMs;

        setMetrics(prev => {
          return [...prev.slice(1), {
            time: formatClock(now),
            ops: opsPerSec,
            latency: (data.avg_latency_ns ?? 0) / 1_000_000,
            memory: (data.memory_used_bytes ?? data.dbx_memory_used_bytes ?? 0) / 1024 / 1024,
            connections: data.active_conns ?? data.dbx_active_connections ?? 0,
            totalCommands: currentTotalOps,
          }];
        });
        const ksRes = await fetchWithAuth(`/t/${clusterId}/api/keyspace`);
        if (ksRes.ok) {
          const ksData = await ksRes.json();
          const formattedKs = Object.keys(ksData || {}).map(k => {
            const capitalized = k.charAt(0).toUpperCase() + k.slice(1);
            return {
              name: capitalized,
              value: ksData[k],
              color: TYPE_COLORS[capitalized] || '#71717a'
            };
          }).filter(d => d.value > 0);
          setKeyspaceData(formattedKs);
        }
      } catch {
        setMetricsOk(false);
      }
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 2000);
    return () => clearInterval(interval);
  }, [clusterId]);

  const latestMetric = metrics[metrics.length - 1];
  const mem = formatMemory(latestMetric.memory);
  const idleOps = metrics.every(m => !m.ops);
  const unreachable = down || !metricsOk;

  const handleBackup = async () => {
    if (!window.confirm('Trigger point-in-time backup for this tenant?')) return;
    setIsBackingUp(true);
    try {
      const res = await fetchWithAuth('/api/tenants/backup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: clusterId })
      });
      if (res.ok) {
        toast.success('Backup triggered.');
      } else {
        toast.error('Backup failed: ' + await res.text());
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : 'Backup failed');
    } finally {
      setIsBackingUp(false);
    }
  };

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Overview"
        purpose="Live command rate, memory, and keyspace for this tenant engine."
        extra={
          <button type="button" className="btn-secondary" onClick={handleBackup} disabled={isBackingUp || unreachable}>
            <CloudUpload size={14} />
            {isBackingUp ? 'Backing up…' : 'Backup'}
          </button>
        }
      />

      {unreachable && !down && (
        <div className="banner-down">Engine unreachable. Metrics could not be sampled.</div>
      )}

      <div className="stat-grid">
        {[
          { label: 'Operations/sec', raw: latestMetric.ops, suffix: '', icon: <Zap size={15} />, change: `total ${latestMetric.totalCommands.toLocaleString()}` },
          { label: 'Memory used', raw: mem.value, suffix: ` ${mem.unit}`, icon: <HardDrive size={15} />, change: 'runtime allocation' },
          { label: 'Active clients', raw: latestMetric.connections, suffix: '', icon: <Users size={15} />, change: 'current connections' },
          { label: 'Avg latency', raw: latestMetric.latency, suffix: 'ms', icon: <Activity size={15} />, change: 'command average' },
        ].map(stat => (
          <div className="stat-card" key={stat.label}>
            <div className="stat-header">
              {stat.label}
              <div className="stat-icon" style={{ color: 'var(--accent-primary)' }}>
                {stat.icon}
              </div>
            </div>
            <div className="stat-value"><TweenedNumber value={stat.raw} suffix={stat.suffix} /></div>
            <div className="stat-change neutral">{stat.change}</div>
          </div>
        ))}
      </div>

      <div className="chart-section">
        <div className="panel relative">
          <div className="panel-header">
            <div className="panel-title"><BarChart2 size={15} /> Throughput (ops/s)</div>
          </div>
          <div style={{ height: 220, position: 'relative' }}>
            <ResponsiveContainer>
              <AreaChart data={metrics}>
                <defs>
                  <linearGradient id="gOps" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#c2410c" stopOpacity={0.28} />
                    <stop offset="95%" stopColor="#c2410c" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="var(--border-color)" vertical={false} />
                <XAxis dataKey="time" stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} interval={4} />
                <YAxis stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} tickFormatter={formatAxis} allowDecimals={false} />
                <Tooltip content={<CustomTooltip />} />
                <Area type="monotone" dataKey="ops" name="ops" stroke="#c2410c" strokeWidth={1.5} fill="url(#gOps)" />
              </AreaChart>
            </ResponsiveContainer>
            {idleOps && (
              <div className="chart-idle">No commands in the sampling window.</div>
            )}
          </div>
        </div>

        <div className="panel">
          <div className="panel-header">
            <div className="panel-title">Key distribution</div>
          </div>
          <div style={{ height: 220, display: 'flex', alignItems: 'center' }}>
            {keyspaceData.length === 0 ? (
              <div className="empty-state" style={{ width: '100%' }}>No keys in this tenant.</div>
            ) : (
              <>
                <ResponsiveContainer>
                  <PieChart>
                    <Pie data={keyspaceData} cx="50%" cy="50%" innerRadius={48} outerRadius={72} paddingAngle={2} dataKey="value">
                      {keyspaceData.map((entry, i) => (
                        <Cell key={i} fill={entry.color} />
                      ))}
                    </Pie>
                    <Tooltip contentStyle={{ background: 'var(--bg-panel)', border: '1px solid var(--border-color)', borderRadius: 6, color: 'var(--text-primary)' }} />
                  </PieChart>
                </ResponsiveContainer>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 108, paddingRight: 12 }}>
                  {keyspaceData.map(d => (
                    <div key={d.name} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
                      <div style={{ width: 8, height: 8, borderRadius: 1, background: d.color, flexShrink: 0 }} />
                      <span style={{ color: 'var(--text-secondary)' }}>{d.name}</span>
                      <span style={{ color: 'var(--text-primary)', fontWeight: 600, marginLeft: 'auto', fontFamily: 'var(--font-mono)' }}>{d.value}</span>
                    </div>
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
