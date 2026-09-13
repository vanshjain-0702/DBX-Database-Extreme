import { useState, useEffect, useRef } from 'react';
import { Zap, HardDrive, Users, Activity, BarChart2, CloudUpload, TrendingUp, Database } from 'lucide-react';
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
  String: '#3b82f6', Hash: '#8b5cf6', List: '#a855f7',
  ZSet: '#06b6d4', Set: '#10b981', JSON: '#f59e0b',
  Geo: '#14b8a6', Stream: '#ea580c', Bitmap: '#84cc16',
  Vector: '#ec4899', Snapshot: '#9333ea'
};

// Stat card color configs — icon bg & glow
const STAT_STYLES = [
  { iconBg: 'rgba(234,88,12,0.12)', iconColor: '#ea580c', glow: 'rgba(234,88,12,0.15)' },
  { iconBg: 'rgba(59,130,246,0.12)', iconColor: '#3b82f6', glow: 'rgba(59,130,246,0.12)' },
  { iconBg: 'rgba(16,185,129,0.12)', iconColor: '#10b981', glow: 'rgba(16,185,129,0.12)' },
  { iconBg: 'rgba(168,85,247,0.12)', iconColor: '#a855f7', glow: 'rgba(168,85,247,0.12)' },
];

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: { name: string; value: number; color?: string; stroke?: string }[]; label?: string }) => {
  if (active && payload && payload.length) {
    return (
      <div style={{
        background: 'var(--bg-panel)',
        border: '1px solid var(--border-color)',
        borderRadius: 10,
        padding: '10px 14px',
        boxShadow: 'var(--shadow-lg)',
        backdropFilter: 'blur(12px)',
      }}>
        <p style={{ color: 'var(--text-muted)', fontSize: 11, marginBottom: 6, fontFamily: 'var(--font-mono)' }}>{label}</p>
        {payload.map((p, i) => (
          <p key={i} style={{ color: p.color || p.stroke, fontSize: 13, fontWeight: 600, fontFamily: 'var(--font-mono)' }}>
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
      duration: 0.45,
      ease: 'power2.out',
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

  const stats = [
    {
      label: 'Operations/sec',
      raw: latestMetric.ops,
      suffix: '',
      icon: <Zap size={16} />,
      change: `${latestMetric.totalCommands.toLocaleString()} total commands`,
      ...STAT_STYLES[0],
    },
    {
      label: 'Memory used',
      raw: mem.value,
      suffix: ` ${mem.unit}`,
      icon: <HardDrive size={16} />,
      change: 'runtime allocation',
      ...STAT_STYLES[1],
    },
    {
      label: 'Active clients',
      raw: latestMetric.connections,
      suffix: '',
      icon: <Users size={16} />,
      change: 'current connections',
      ...STAT_STYLES[2],
    },
    {
      label: 'Avg latency',
      raw: latestMetric.latency,
      suffix: 'ms',
      icon: <Activity size={16} />,
      change: 'command average',
      ...STAT_STYLES[3],
    },
  ];

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
        <div className="banner-down">Engine unreachable — metrics could not be sampled.</div>
      )}

      {/* Stat cards */}
      <div className="stat-grid">
        {stats.map((stat, i) => (
          <div
            className="stat-card"
            key={stat.label}
            style={{ animationDelay: `${(i + 1) * 0.07}s` }}
          >
            <div className="stat-header">
              {stat.label}
              <div
                className="stat-icon"
                style={{
                  background: stat.iconBg,
                  border: `1px solid ${stat.glow}`,
                  color: stat.iconColor,
                }}
              >
                {stat.icon}
              </div>
            </div>
            <div className="stat-value">
              <TweenedNumber value={stat.raw} suffix={stat.suffix} />
            </div>
            <div className="stat-change neutral flex items-center gap-1.5">
              <TrendingUp size={11} className="opacity-60" />
              {stat.change}
            </div>
          </div>
        ))}
      </div>

      {/* Charts */}
      <div className="chart-section">
        {/* Throughput */}
        <div className="panel relative" style={{ overflow: 'visible' }}>
          <div className="panel-header">
            <div className="panel-title">
              <BarChart2 size={15} style={{ color: 'var(--accent-primary)' }} />
              Throughput
              <span className="text-[11px] font-mono text-[var(--text-muted)] font-normal ml-1">ops/s · live</span>
            </div>
            <div className="text-[11px] font-mono text-[var(--text-muted)]">
              {latestMetric.ops > 0 ? (
                <span style={{ color: 'var(--success)' }}>● {latestMetric.ops.toLocaleString()} ops/s</span>
              ) : (
                <span className="opacity-60">● idle</span>
              )}
            </div>
          </div>
          <div style={{ height: 220, position: 'relative' }}>
            <ResponsiveContainer>
              <AreaChart data={metrics} margin={{ top: 8, right: 12, left: -8, bottom: 0 }}>
                <defs>
                  <linearGradient id="gOps" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#c2410c" stopOpacity={0.35} />
                    <stop offset="95%" stopColor="#c2410c" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="var(--border-color)" vertical={false} strokeDasharray="3 0" />
                <XAxis dataKey="time" stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} interval={4} />
                <YAxis stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} tickFormatter={formatAxis} allowDecimals={false} />
                <Tooltip content={<CustomTooltip />} cursor={{ stroke: 'var(--accent-primary)', strokeWidth: 1, strokeDasharray: '4 2' }} />
                <Area type="monotone" dataKey="ops" name="ops" stroke="#c2410c" strokeWidth={2} fill="url(#gOps)" dot={false} />
              </AreaChart>
            </ResponsiveContainer>
            {idleOps && (
              <div className="chart-idle">
                <span className="text-[13px] font-medium" style={{ color: 'var(--text-muted)' }}>
                  No commands in sampling window
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Key distribution */}
        <div className="panel">
          <div className="panel-header">
            <div className="panel-title">Key distribution</div>
            {keyspaceData.length > 0 && (
              <span className="text-[11px] font-mono text-[var(--text-muted)]">
                {keyspaceData.reduce((s, d) => s + d.value, 0).toLocaleString()} keys
              </span>
            )}
          </div>
          <div style={{ height: 220, display: 'flex', alignItems: 'center' }}>
            {keyspaceData.length === 0 ? (
              <div className="empty-state" style={{ width: '100%' }}>
                <Database size={28} style={{ opacity: 0.25 }} />
                No keys in this tenant.
              </div>
            ) : (
              <>
                <ResponsiveContainer>
                  <PieChart>
                    <Pie
                      data={keyspaceData}
                      cx="50%"
                      cy="50%"
                      innerRadius={50}
                      outerRadius={76}
                      paddingAngle={3}
                      dataKey="value"
                      strokeWidth={0}
                    >
                      {keyspaceData.map((entry, i) => (
                        <Cell key={i} fill={entry.color} />
                      ))}
                    </Pie>
                    <Tooltip
                      contentStyle={{
                        background: 'var(--bg-panel)',
                        border: '1px solid var(--border-color)',
                        borderRadius: 10,
                        color: 'var(--text-primary)',
                        boxShadow: 'var(--shadow-lg)',
                        fontSize: 13,
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 7, minWidth: 110, paddingRight: 14 }}>
                  {keyspaceData.map(d => (
                    <div key={d.name} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
                      <div style={{ width: 8, height: 8, borderRadius: 2, background: d.color, flexShrink: 0 }} />
                      <span style={{ color: 'var(--text-secondary)', flex: 1 }}>{d.name}</span>
                      <span style={{ color: 'var(--text-primary)', fontWeight: 600, fontFamily: 'var(--font-mono)', fontSize: 12 }}>{d.value}</span>
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
