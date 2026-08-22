import { useState, useEffect } from 'react';
import { HardDrive, Database, Layers } from 'lucide-react';
import { fetchWithAuth } from '../api';

export default function StoragePage({ clusterId }: { clusterId: string }) {
  const [metrics, setMetrics] = useState<any>({});

  const [error, setError] = useState<string | null>(null);

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
      } catch (err: any) {
        setError(err.message || 'Network error');
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [clusterId]);

  const memUsed = (metrics.memory_used_bytes || 0) / 1024 / 1024;
  const memSys = (metrics.memory_sys_bytes || 1) / 1024 / 1024;
  const memPct = Math.min(100, (memUsed / memSys) * 100).toFixed(1);

  return (
    <div className="flex flex-col h-full bg-[#f4f4f5] p-8 overflow-y-auto">
      <div className="flex items-center gap-3 mb-8">
        <HardDrive size={24} className="text-red-600" />
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Disk & Storage</h1>
          <p className="text-sm text-gray-500 mt-1">Real-time Go runtime Memory and GC telemetry.</p>
        </div>
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-xl text-red-600 text-sm font-medium">
          {error}
        </div>
      )}

      <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm mb-8">
        <h3 className="text-sm font-bold text-slate-900 mb-6 flex items-center gap-2">
          <Database size={16} className="text-red-600" /> Memory Allocation (OS vs Heap)
        </h3>
        
        <div className="relative w-full h-8 bg-gray-100 rounded-full overflow-hidden border border-gray-200">
           <div 
             className="absolute top-0 left-0 h-full bg-red-500 transition-all duration-500"
             style={{ width: `${memPct}%` }}
           />
        </div>
        <div className="flex justify-between mt-3">
           <div className="text-xs font-bold text-gray-500">Allocated: {memUsed.toFixed(1)} MB</div>
           <div className="text-xs font-bold text-slate-900">{memPct}% Utilized</div>
           <div className="text-xs font-bold text-gray-500">Reserved (Sys): {memSys.toFixed(1)} MB</div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-6">
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Total Heap Objects</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{(metrics.heap_objects || 0).toLocaleString()}</h2>
           </div>
           <Layers size={32} className="text-red-500 opacity-80" />
        </div>
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm flex items-center justify-between">
           <div>
              <p className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">Total GC Pause Time</p>
              <h2 className="text-3xl font-mono font-bold text-slate-900">{((metrics.gc_pause_total_ns || 0) / 1000000).toFixed(2)} ms</h2>
           </div>
           <HardDrive size={32} className="text-red-600 opacity-80" />
        </div>
      </div>
    </div>
  );
}
