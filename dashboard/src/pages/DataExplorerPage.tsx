import { useState, useEffect, useLayoutEffect, useRef } from 'react';
import { Database, Search, RefreshCw, Layers, Plus, Save } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { SkeletonRow } from '../components/Skeleton';
import gsap from 'gsap';
export default function DataExplorerPage({ clusterId }: { clusterId: string }) {
  const [keys, setKeys] = useState<{key:string, type:string, ttl:number}[]>([]);
  const [keyFilter, setKeyFilter] = useState('');
  const [selectedKey, setSelectedKey] = useState<{key:string, type:string} | null>(null);
  const [keyValue, setKeyValue] = useState<any>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyValue, setNewKeyValue] = useState('');
  const [saving, setSaving] = useState(false);

  const [loadingKeys, setLoadingKeys] = useState(false);
  const [fetchError, setFetchError] = useState('');

  const fetchKeys = async () => {
    setLoadingKeys(true);
    setFetchError('');
    try {
      const res = await fetchWithAuth(`/t/${clusterId}/query`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: ['KEYS', '*'] })
      });
      if (!res.ok) {
        setFetchError(`Server error: ${res.status}`);
        setKeys([]);
        return;
      }
      const data = await res.json();
      const respStr: string = data.response || '';

      // Parse RESP3 array: *N\r\n$len\r\nkey\r\n...
      const names: string[] = [];
      if (respStr.startsWith('*')) {
        const lines = respStr.split('\r\n');
        const count = parseInt(lines[0].substring(1));
        let i = 1;
        for (let n = 0; n < count && i < lines.length; n++) {
          if (lines[i] && lines[i].startsWith('$')) {
            i++; // skip length line
            if (lines[i] !== undefined) {
              names.push(lines[i]);
              i++;
            }
          } else {
            i++;
          }
        }
      }

      // Limit to first 200 keys for performance
      const limited = names.slice(0, 200);

      // Fetch types in parallel (batched)
      const keysWithType = await Promise.all(
        limited.map(async (n: string) => {
          try {
            const typeRes = await fetchWithAuth(`/t/${clusterId}/query`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ command: ['TYPE', n] })
            });
            const td = await typeRes.json();
            const type = (td.response || '+string').replace(/^\+/, '').trim();
            return { key: n, type, ttl: -1 };
          } catch {
            return { key: n, type: 'unknown', ttl: -1 };
          }
        })
      );
      setKeys(keysWithType);
      if (names.length > 200) {
        setFetchError(`Showing first 200 of ${names.length} keys`);
      }
    } catch (e: any) {
      setFetchError(e.message || 'Failed to fetch keys');
      setKeys([]);
    } finally {
      setLoadingKeys(false);
    }
  };

  useEffect(() => {
    fetchKeys();
  }, [clusterId]);

  const loadKeyValue = async (key: string, type: string) => {
    setSelectedKey({key, type});
    setKeyValue("Loading...");
    
    try {
      if (type === 'string') {
        const res = await fetchWithAuth(`/t/${clusterId}/query`, {
          method: 'POST', body: JSON.stringify({ command: ['GET', key] })
        });
        const data = await res.json();
        const match = data.response.match(/\$[0-9]+\r\n([\s\S]*?)\r\n/);
        if (match) {
           let val = match[1];
           try {
              val = JSON.stringify(JSON.parse(val), null, 2);
           } catch(e){}
           setKeyValue(val);
        } else {
           setKeyValue(data.response);
        }
      } else {
        setKeyValue(`Cannot preview raw ${type} yet. Use terminal.`);
      }
    } catch(e) {
       setKeyValue("Error loading data.");
    }
  };

  const handleNewKey = async () => {
    if (!newKeyName || !newKeyValue) return;
    setSaving(true);
    try {
      const res = await fetchWithAuth(`/t/${clusterId}/query`, {
        method: 'POST', body: JSON.stringify({ command: ['SET', newKeyName, newKeyValue] })
      });
      if (res.ok) {
        setIsModalOpen(false);
        setNewKeyName('');
        setNewKeyValue('');
        fetchKeys();
      }
    } catch(e) {
      console.error(e);
    } finally {
      setSaving(false);
    }
  };

  const filteredKeys = keys.filter(k => k.key.toLowerCase().includes(keyFilter.toLowerCase()));
  const listRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (listRef.current && filteredKeys.length > 0) {
      gsap.fromTo(
        listRef.current.querySelectorAll('.key-row'),
        { y: 20, opacity: 0, scale: 0.95 },
        { 
          y: 0, opacity: 1, scale: 1, 
          duration: 0.6, 
          stagger: 0.03, 
          ease: 'elastic.out(1, 0.7)',
          clearProps: 'all'
        }
      );
    }
  }, [keyFilter, keys.length]);

  return (
    <div className="flex flex-col h-full bg-[#f4f4f5]">
      {/* Header */}
      <div className="bg-white px-8 py-5 border-b border-gray-200 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Database size={20} className="text-red-600" />
          <h1 className="text-xl font-bold text-slate-900 tracking-tight">Key Browser</h1>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => setIsModalOpen(true)} className="bg-red-600 hover:bg-red-700 text-white px-4 py-2 rounded-lg font-medium text-sm flex items-center gap-2 transition-colors shadow-sm">
            <Plus size={16} /> New Key
          </button>
          <button onClick={fetchKeys} className="bg-white border border-gray-200 hover:bg-gray-50 text-slate-700 px-4 py-2 rounded-lg font-medium text-sm flex items-center gap-2 transition-colors">
            <RefreshCw size={16} /> Refresh
          </button>
        </div>
      </div>

      {/* Loading/Error banner */}
      {fetchError && (
        <div className="px-4 py-2 bg-amber-50 border-b border-amber-200 text-amber-700 text-xs font-medium">
          {fetchError}
        </div>
      )}

      {/* Split Pane */}
      <div className="flex-1 flex overflow-hidden">

        {/* Left Pane: Keys List (Mini Data Grid) */}
        <div className="w-[450px] bg-white dark:bg-slate-900 border-r border-gray-200 dark:border-slate-800 flex flex-col flex-shrink-0">
          <div className="p-4 border-b border-gray-200 dark:border-slate-800">
            <div className="relative">
              <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input 
                type="text" 
                placeholder="Search keys..." 
                value={keyFilter}
                onChange={e => setKeyFilter(e.target.value)}
                className="w-full pl-9 pr-4 py-2 bg-gray-50 dark:bg-slate-800 border border-gray-200 dark:border-slate-700 rounded-lg text-sm outline-none focus:border-red-500 transition-colors"
              />
            </div>
          </div>
          
          {/* Grid Header */}
          <div className="flex items-center px-4 py-2 bg-gray-50 dark:bg-slate-800/50 border-b border-gray-200 dark:border-slate-800 text-xs font-semibold text-gray-500 uppercase tracking-wider">
             <div className="flex-1">Key Name</div>
             <div className="w-20 text-right">Type</div>
          </div>

          <div className="flex-1 overflow-y-auto">
            {loadingKeys ? (
              <div className="flex flex-col">
                {Array.from({ length: 8 }).map((_, i) => <SkeletonRow key={i} />)}
              </div>
            ) : filteredKeys.length === 0 ? (
              <div className="p-8 text-center text-sm text-gray-500">
                {keyFilter ? `No keys matching "${keyFilter}"` : 'No keys found. Use the CLI or populate data.'}
              </div>
            ) : (
              <div className="divide-y divide-gray-100 dark:divide-slate-800/50" ref={listRef}>
                {filteredKeys.map(k => (
                  <div 
                    key={k.key}
                    onClick={() => loadKeyValue(k.key, k.type)}
                    className={`key-row flex items-center px-4 py-3 cursor-pointer transition-colors group ${
                      selectedKey?.key === k.key 
                        ? 'bg-red-50 dark:bg-red-900/20 shadow-inner' 
                        : 'hover:bg-gray-50 dark:hover:bg-slate-800/50'
                    }`}
                  >
                    <div className="flex-1 flex items-center gap-3 min-w-0">
                      <Layers size={14} className={selectedKey?.key === k.key ? "text-red-600" : "text-gray-400 group-hover:text-gray-500"} />
                      <span className="text-sm font-medium font-mono text-slate-700 dark:text-slate-300 truncate">{k.key}</span>
                    </div>
                    <div className="w-20 flex justify-end">
                      <span className={`text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded ${
                        selectedKey?.key === k.key 
                          ? 'bg-red-100 text-red-700 dark:bg-red-800 dark:text-red-100' 
                          : 'bg-gray-100 text-gray-500 dark:bg-slate-800 dark:text-slate-400'
                      }`}>
                        {k.type}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Right Pane: Value Editor */}
        <div className="flex-1 bg-gray-50 flex flex-col">
          {selectedKey ? (
            <>
              <div className="px-6 py-4 bg-white border-b border-gray-200 flex items-center justify-between">
                <div>
                  <h2 className="text-lg font-bold text-slate-900 font-mono">{selectedKey.key}</h2>
                  <span className="text-xs font-semibold uppercase tracking-wider text-red-600 bg-red-50 px-2 py-0.5 rounded-md mt-1 inline-block border border-red-100">{selectedKey.type}</span>
                </div>
              </div>
              <div className="flex-1 p-6 overflow-y-auto">
                <div className="bg-white border border-gray-200 rounded-xl overflow-hidden shadow-sm h-full flex flex-col">
                  <div className="px-4 py-2 bg-gray-50 border-b border-gray-200 flex items-center justify-between">
                     <span className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Value</span>
                     <button className="text-xs font-semibold text-red-600 hover:text-red-700 flex items-center gap-1"><Save size={12}/> Save Changes</button>
                  </div>
                  <textarea 
                    className="flex-1 w-full p-4 text-sm font-mono text-slate-800 outline-none resize-none"
                    value={keyValue}
                    readOnly
                  />
                </div>
              </div>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-center flex-col text-gray-400">
              <Database size={48} className="mb-4 text-gray-300" />
              <p className="text-slate-600 font-medium">Select a key to view its data</p>
              <p className="text-sm mt-1">Or create a new key to get started.</p>
            </div>
          )}
        </div>
      </div>

      {/* Create Key Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4" onClick={() => setIsModalOpen(false)}>
          <div className="bg-white border border-gray-200 rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden" onClick={e => e.stopPropagation()}>
            <div className="px-6 py-4 border-b border-gray-200 flex items-center justify-between bg-gray-50">
              <h3 className="text-slate-900 font-bold text-[15px]">Create New Key</h3>
              <button onClick={() => setIsModalOpen(false)} className="text-gray-500 hover:text-slate-900 transition-colors">
                <Plus size={20} className="rotate-45" />
              </button>
            </div>
            <div className="p-6 space-y-5">
              <div>
                <label className="block text-[12px] font-bold text-gray-600 uppercase tracking-wider mb-2">Key Name</label>
                <input 
                  type="text" 
                  className="w-full bg-white border border-gray-200 text-slate-900 text-sm rounded-lg px-3 py-2.5 outline-none focus:border-red-500 focus:ring-1 focus:ring-red-500/50 transition-all font-mono"
                  placeholder="e.g. user:1000"
                  value={newKeyName}
                  onChange={e => setNewKeyName(e.target.value)}
                  autoFocus
                />
              </div>
              <div>
                <label className="block text-[12px] font-bold text-gray-600 uppercase tracking-wider mb-2">Value (String)</label>
                <textarea 
                  className="w-full bg-white border border-gray-200 text-slate-900 text-sm rounded-lg px-3 py-2.5 outline-none focus:border-red-500 focus:ring-1 focus:ring-red-500/50 transition-all font-mono h-32 resize-none"
                  placeholder='e.g. {"name": "Alice"}'
                  value={newKeyValue}
                  onChange={e => setNewKeyValue(e.target.value)}
                />
              </div>
              <div className="pt-2 flex items-center justify-end gap-3">
                <button className="px-4 py-2 text-sm font-medium text-gray-500 hover:text-slate-900 transition-colors" onClick={() => setIsModalOpen(false)}>Cancel</button>
                <button 
                  className="bg-red-600 text-white hover:bg-red-700 px-6 py-2.5 rounded-lg font-semibold text-sm transition-colors shadow-md hover:shadow-lg disabled:opacity-50" 
                  onClick={handleNewKey} 
                  disabled={saving}
                >
                  {saving ? 'Saving...' : 'Save Key'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
