import { useState, useEffect } from 'react';
import { Globe, ArrowUpRight, ArrowDownRight, Activity } from 'lucide-react';
import { fetchWithAuth } from '../api';

export default function NetworkPage({ clusterId }: { clusterId: string }) {
  const [metrics, setMetrics] = useState<any>({});

  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/metrics`);
        if (res.ok) setMetrics(await res.json());
      } catch (e) { console.error(e); }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [clusterId]);

  return (
    <div className="flex flex-col h-full bg-[#f4f4f5] p-8 overflow-y-auto">
      <div className="flex items-center gap-3 mb-8">
        <Globe size={24} className="text-red-600" />
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Network I/O</h1>
          <p className="text-sm text-gray-500 mt-1">Live throughput and latency metrics.</p>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-6 mb-8">
        <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm flex items-center justify-between">
           <div>
              <div className="flex items-center gap-2 mb-2">
                <ArrowUpRight size={16} className="text-red-500" />
                <p className="text-xs font-bold text-gray-500 uppercase tracking-wider">Total Read Ops</p>
              </div>
              <h2 className="text-4xl font-mono font-bold text-slate-900">{(metrics.total_reads || 0).toLocaleString()}</h2>
           </div>
        </div>
        <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm flex items-center justify-between">
           <div>
              <div className="flex items-center gap-2 mb-2">
                <ArrowDownRight size={16} className="text-emerald-500" />
                <p className="text-xs font-bold text-gray-500 uppercase tracking-wider">Total Write Ops</p>
              </div>
              <h2 className="text-4xl font-mono font-bold text-slate-900">{(metrics.total_writes || 0).toLocaleString()}</h2>
           </div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-6">
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Active TCP Conns</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{metrics.active_conns || 0}</h2>
           </div>
           <Globe size={32} className="text-gray-300" />
        </div>
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Avg Cmd Latency</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{((metrics.avg_latency_ns || 0) / 1000000).toFixed(2)} ms</h2>
           </div>
           <Activity size={32} className="text-red-600 opacity-80" />
        </div>
      </div>
    </div>
  );
}
