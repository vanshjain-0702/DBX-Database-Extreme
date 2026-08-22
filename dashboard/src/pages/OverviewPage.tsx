import { useState, useEffect, useRef, useLayoutEffect } from 'react';
import gsap from 'gsap';
import { Zap, HardDrive, Users, Activity, BarChart2, CloudUpload } from 'lucide-react';
import {
  XAxis, YAxis, CartesianGrid, Tooltip,
  ResponsiveContainer, AreaChart, Area, PieChart, Pie, Cell
} from 'recharts';
import { fetchWithAuth } from '../api';

const initMetrics = () => Array.from({ length: 15 }).map(() => ({
  time: ``,
  ops: 0,
  latency: 0,
  memory: 0,
  connections: 0,
  totalCommands: 0,
}));

const TYPE_COLORS: Record<string, string> = {
  String: '#3b82f6', Hash: '#6366f1', List: '#8b5cf6',
  ZSet: '#0ea5e9', Set: '#10b981', JSON: '#f59e0b',
  Geo: '#14b8a6', Stream: '#f97316', Bitmap: '#84cc16',
  Vector: '#db2777', Snapshot: '#a855f7'
};

const CustomTooltip = ({ active, payload, label }: any) => {
  if (active && payload && payload.length) {
    return (
      <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border-color)', borderRadius: '8px', padding: '12px 16px' }}>
        <p style={{ color: 'var(--text-muted)', fontSize: 12, marginBottom: 6 }}>{label}</p>
        {payload.map((p: any, i: number) => (
          <p key={i} style={{ color: p.color || p.stroke, fontSize: 13, fontWeight: 600 }}>
            {p.name}: {p.value.toLocaleString()}{p.name === 'latency' ? 'ms' : p.name === 'memory' ? 'MB' : ''}
          </p>
        ))}
      </div>
    );
  }
  return null;
};

import { useToast } from '../components/Toaster';

function TweenedNumber({ value, prefix = '', suffix = '' }: { value: number, prefix?: string, suffix?: string }) {
  const nodeRef = useRef<HTMLSpanElement>(null);
  const valRef = useRef({ val: 0 });

  useEffect(() => {
    gsap.to(valRef.current, {
      val: value,
      duration: 0.8,
      ease: "power2.out",
      onUpdate: () => {
        if (nodeRef.current) {
          const formatted = (valRef.current.val >= 1000)
            ? (valRef.current.val / 1000).toFixed(1) + 'k'
            : valRef.current.val.toFixed(valRef.current.val % 1 !== 0 ? 2 : 0);
          nodeRef.current.innerText = prefix + formatted + suffix;
        }
      }
    });
  }, [value, prefix, suffix]);

  return <span ref={nodeRef}>{prefix}0{suffix}</span>;
}

export default function OverviewPage({ clusterId }: { clusterId: string }) {
  const [metrics, setMetrics] = useState(initMetrics());
  const [keyspaceData, setKeyspaceData] = useState<{ name: string, value: number, color: string }[]>([{ name: 'Empty', value: 1, color: '#444' }]);
  const [isBackingUp, setIsBackingUp] = useState(false);
  const toast = useToast();

  const lastOpsRef = useRef<number>(0);
  const lastTimeRef = useRef<number>(Date.now());

  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (!res.ok) return;
        const data = await res.json();
        const now = new Date();
        const nowMs = now.getTime();

        const currentTotalOps = data.total_commands ?? data.dbx_commands_total ?? 0;
        let opsPerSec = 0;
        if (lastOpsRef.current > 0) {
          const timeDiffSec = (nowMs - lastTimeRef.current) / 1000;
          const opsDiff = currentTotalOps - lastOpsRef.current;
          opsPerSec = timeDiffSec > 0 ? Math.max(0, Math.floor(opsDiff / timeDiffSec)) : 0;
        } else if (currentTotalOps > 0) {
          opsPerSec = 0;
        }

        lastOpsRef.current = currentTotalOps;
        lastTimeRef.current = nowMs;

        setMetrics(prev => {
          return [...prev.slice(1), {
            time: `${now.getHours()}:${now.getMinutes()}:${now.getSeconds()}`,
            ops: opsPerSec,
            latency: (data.avg_latency_ns ?? 0) / 1_000_000,
            memory: (data.memory_used_bytes ?? data.dbx_memory_used_bytes ?? 0) / 1024 / 1024,
            connections: data.active_conns ?? data.dbx_active_connections ?? 0,
            totalCommands: currentTotalOps,
          }];
        });
        const ksRes = await fetchWithAuth(`/t/${clusterId}/api/keyspace`);
        const ksData = await ksRes.json();
        const formattedKs = Object.keys(ksData).map(k => {
          const capitalized = k.charAt(0).toUpperCase() + k.slice(1);
          return {
            name: capitalized,
            value: ksData[k],
            color: TYPE_COLORS[capitalized] || '#888'
          };
        });
        setKeyspaceData(formattedKs.length > 0 ? formattedKs : [{ name: 'Empty', value: 1, color: '#444' }]);
      } catch (e) { console.error(e); }
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 2000);
    return () => clearInterval(interval);
  }, [clusterId]);

  const latestMetric = metrics[metrics.length - 1];

  const handleBackup = async () => {
    // using window.confirm is still ok for blocking prompts, but we can replace the alerts
    if (!window.confirm("Trigger point-in-time S3 backup for this cluster?")) return;
    setIsBackingUp(true);
    try {
      const res = await fetchWithAuth('/api/tenants/backup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: clusterId })
      });
      if (res.ok) {
        toast.success("S3 Backup triggered successfully.");
      } else {
        toast.error("Backup failed: " + await res.text());
      }
    } catch (e: any) {
      toast.error("Error: " + e.message);
    } finally {
      setIsBackingUp(false);
    }
  };

  const containerRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (containerRef.current) {
      gsap.fromTo(
        gsap.utils.toArray('.stat-card', containerRef.current),
        { y: 20, opacity: 0 },
        { y: 0, opacity: 1, duration: 0.6, stagger: 0.1, ease: 'expo.out' }
      );
      gsap.fromTo(
        gsap.utils.toArray('.panel', containerRef.current),
        { y: 20, opacity: 0 },
        { y: 0, opacity: 1, duration: 0.8, stagger: 0.1, ease: 'expo.out', delay: 0.2 }
      );
    }
  }, []);

  return (
    <div className="content-area" ref={containerRef}>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
        <button className="btn-secondary" onClick={handleBackup} disabled={isBackingUp} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <CloudUpload size={16} />
          {isBackingUp ? 'Backing up...' : 'Trigger S3 Backup'}
        </button>
      </div>
      <div className="stat-grid">
        {[
          { label: 'Operations/sec', raw: latestMetric.ops, suffix: '', isTween: true, value: `${(latestMetric.ops / 1000).toFixed(1)}k`, icon: <Zap size={20} />, color: '#3b82f6', change: `total ${latestMetric.totalCommands.toLocaleString()}`, pos: null },
          { label: 'Memory Used', raw: (latestMetric.memory / 1024), suffix: ' GB', isTween: true, value: `${(latestMetric.memory / 1024).toFixed(2)} GB`, icon: <HardDrive size={20} />, color: '#8b5cf6', change: 'runtime allocation', pos: null },
          { label: 'Active Clients', raw: latestMetric.connections, suffix: '', isTween: true, value: latestMetric.connections.toLocaleString(), icon: <Users size={20} />, color: '#0ea5e9', change: 'current connections', pos: null },
          { label: 'Avg Latency', raw: latestMetric.latency, suffix: 'ms', isTween: true, value: `${latestMetric.latency.toFixed(2)}ms`, icon: <Activity size={20} />, color: '#10b981', change: 'command average', pos: null },
        ].map(stat => (
          <div className="stat-card" key={stat.label}>
            <div className="stat-header">
              {stat.label}
              <div className="stat-icon" style={{ color: stat.color, borderColor: `${stat.color}30`, background: `${stat.color}15` }}>
                {stat.icon}
              </div>
            </div>
            <div className="stat-value">{stat.isTween ? <TweenedNumber value={stat.raw} suffix={stat.suffix} /> : stat.value}</div>
            <div className={`stat-change ${stat.pos === true ? 'positive' : stat.pos === false ? 'negative' : 'neutral'}`}>
              {stat.change}
            </div>
          </div>
        ))}
      </div>

      <div className="chart-section">
        <div className="panel">
          <div className="panel-header">
            <div className="panel-title"><BarChart2 size={18} /> Throughput (ops/s)</div>
          </div>
          <div style={{ height: 220 }}>
            <ResponsiveContainer>
              <AreaChart data={metrics}>
                <defs>
                  <linearGradient id="gOps" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#6366f1" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="#6366f1" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="rgba(255,255,255,0.04)" vertical={false} />
                <XAxis dataKey="time" stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} interval={4} />
                <YAxis stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} tickFormatter={v => `${(v / 1000).toFixed(0)}k`} />
                <Tooltip content={<CustomTooltip />} />
                <Area type="monotone" dataKey="ops" name="ops" stroke="#6366f1" strokeWidth={2} fill="url(#gOps)" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="panel">
          <div className="panel-header">
            <div className="panel-title">Key Distribution</div>
          </div>
          <div style={{ height: 220, display: 'flex', alignItems: 'center' }}>
            <ResponsiveContainer>
              <PieChart>
                <Pie data={keyspaceData} cx="50%" cy="50%" innerRadius={55} outerRadius={80} paddingAngle={3} dataKey="value">
                  {keyspaceData.map((entry, i) => (
                    <Cell key={i} fill={entry.color} opacity={0.9} />
                  ))}
                </Pie>
                <Tooltip contentStyle={{ background: 'var(--bg-secondary)', border: '1px solid var(--border-color)', borderRadius: 8 }} />
              </PieChart>
            </ResponsiveContainer>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, minWidth: 100 }}>
              {keyspaceData.map(d => (
                <div key={d.name} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
                  <div style={{ width: 10, height: 10, borderRadius: 2, background: d.color }} />
                  <span style={{ color: 'var(--text-secondary)' }}>{d.name}</span>
                  <span style={{ color: 'white', fontWeight: 600, marginLeft: 'auto' }}>{d.value}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
