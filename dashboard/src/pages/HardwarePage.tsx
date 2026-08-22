import { useState, useEffect, useRef } from 'react';
import { Cpu, Activity, Server } from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer
} from 'recharts';
import { fetchWithAuth } from '../api';

interface ChartPoint { time: string; goroutines: number; ops: number; }

export default function HardwarePage({ clusterId }: { clusterId: string }) {
  const [data, setData] = useState<ChartPoint[]>([]);
  const [cpuCount, setCpuCount] = useState(0);
  const [goroutines, setGoroutines] = useState(0);
  const [uptime, setUptime] = useState(0);
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
        
        setCpuCount(d.cpu_count || 1);
        setGoroutines(d.goroutines || 0);
        setUptime(d.uptime_seconds || 0);
        
        const nowMs = Date.now();
        const totalOps = d.total_commands ?? 0;
        const dt = (nowMs - lastTimeRef.current) / 1000;
        
        const opsPerSec = lastOpsRef.current > 0 && dt > 0
          ? Math.max(0, Math.floor((totalOps - lastOpsRef.current) / dt))
          : 0;
          
        lastOpsRef.current  = totalOps;
        lastTimeRef.current = nowMs;

        const now = new Date();
        const timeLabel = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`;

        setData(prev => {
          const newData = [...prev, { time: timeLabel, goroutines: d.goroutines, ops: opsPerSec }];
          if (newData.length > 30) return newData.slice(newData.length - 30);
          return newData;
        });

      } catch (err: any) {
        setError(err.message || 'Network error');
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [clusterId]);

  return (
    <div className="flex flex-col h-full bg-[#f4f4f5] p-8 overflow-y-auto">
      <div className="flex items-center gap-3 mb-8">
        <Cpu size={24} className="text-red-600" />
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Hardware & Load</h1>
          <p className="text-sm text-gray-500 mt-1">Real-time Go runtime CPU telemetry.</p>
        </div>
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-xl text-red-600 text-sm font-medium">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-6 mb-8">
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Logical Cores</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{cpuCount}</h2>
           </div>
           <Server size={32} className="text-gray-300" />
        </div>
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Active Goroutines</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{goroutines}</h2>
           </div>
           <Activity size={32} className="text-red-600 opacity-80" />
        </div>
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Engine Uptime</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{(uptime / 3600).toFixed(1)}h</h2>
           </div>
           <Cpu size={32} className="text-red-500 opacity-80" />
        </div>
      </div>

      <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex-1 min-h-[400px]">
        <h3 className="text-sm font-bold text-slate-900 mb-6 flex items-center gap-2">
          Goroutines vs Throughput
        </h3>
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 5, right: 5, left: -15, bottom: 0 }}>
            <defs>
              <linearGradient id="gGoroutines" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%"  stopColor="#dc2626" stopOpacity={0.2} />
                <stop offset="95%" stopColor="#dc2626" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid stroke="#f1f5f9" vertical={false} />
            <XAxis dataKey="time" stroke="#94a3b8" tick={{ fill: '#64748b', fontSize: 11 }} tickLine={false} axisLine={false} />
            <YAxis stroke="#94a3b8" tick={{ fill: '#64748b', fontSize: 11 }} tickLine={false} axisLine={false} />
            <Tooltip contentStyle={{ borderRadius: '12px', border: '1px solid #e2e8f0', boxShadow: '0 10px 15px -3px rgb(0 0 0 / 0.1)' }} />
            <Area type="monotone" dataKey="goroutines" name="Goroutines" stroke="#dc2626" strokeWidth={2} fill="url(#gGoroutines)" />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
