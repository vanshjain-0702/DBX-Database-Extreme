import { useState, useEffect } from 'react';
import { Database, Search, RefreshCw, Layers, Plus } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { SkeletonRow } from '../components/Skeleton';
import PageChrome from '../components/PageChrome';

export default function DataExplorerPage({ clusterId }: { clusterId: string }) {
  const [keys, setKeys] = useState<{ key: string; type: string; ttl: number }[]>([]);
  const [keyFilter, setKeyFilter] = useState('');
  const [selectedKey, setSelectedKey] = useState<{ key: string; type: string } | null>(null);
  const [keyValue, setKeyValue] = useState<string | null>(null);
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

      const names: string[] = [];
      if (respStr.startsWith('*')) {
        const lines = respStr.split('\r\n');
        const count = parseInt(lines[0].substring(1));
        let i = 1;
        for (let n = 0; n < count && i < lines.length; n++) {
          if (lines[i] && lines[i].startsWith('$')) {
            i++;
            if (lines[i] !== undefined) {
              names.push(lines[i]);
              i++;
            }
          } else {
            i++;
          }
        }
      }

      const limited = names.slice(0, 200);

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
    } catch (e: unknown) {
      setFetchError(e instanceof Error ? e.message : 'Failed to fetch keys');
      setKeys([]);
    } finally {
      setLoadingKeys(false);
    }
  };

  useEffect(() => {
    fetchKeys();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusterId]);

  const loadKeyValue = async (key: string, type: string) => {
    setSelectedKey({ key, type });
    setKeyValue('Loading…');

    try {
      if (type === 'string') {
        const res = await fetchWithAuth(`/t/${clusterId}/query`, {
          method: 'POST', body: JSON.stringify({ command: ['GET', key] })
        });
        const data = await res.json();
        const match = String(data.response || '').match(/\$[0-9]+\r\n([\s\S]*?)\r\n/);
        if (match) {
          let val = match[1];
          try {
            val = JSON.stringify(JSON.parse(val), null, 2);
          } catch {
            /* keep raw */
          }
          setKeyValue(val);
        } else {
          setKeyValue(data.response);
        }
      } else {
        setKeyValue(`Cannot preview raw ${type} yet. Use the console.`);
      }
    } catch {
      setKeyValue('Error loading data.');
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
    } catch {
      /* keep modal open */
    } finally {
      setSaving(false);
    }
  };

  const filteredKeys = keys.filter(k => k.key.toLowerCase().includes(keyFilter.toLowerCase()));

  return (
    <div className="flex flex-col h-full bg-[var(--bg-primary)]">
      <div className="px-7 pt-6 pb-3">
        <PageChrome
          clusterId={clusterId}
          title="Data Explorer"
          purpose="Browse keys in this tenant and inspect string values as JSON when possible."
          extra={
            <div className="flex items-center gap-2">
              <button type="button" onClick={() => setIsModalOpen(true)} className="btn-primary">
                <Plus size={14} /> New key
              </button>
              <button type="button" onClick={fetchKeys} className="btn-secondary">
                <RefreshCw size={14} /> Refresh
              </button>
            </div>
          }
        />
      </div>

      {fetchError && (
        <div className="mx-7 mb-2 alert-warn">{fetchError}</div>
      )}

      <div className="flex-1 flex overflow-hidden mx-7 mb-6 border border-[var(--border-color)] rounded-lg bg-[var(--bg-panel)]">
        <div className="w-[380px] border-r border-[var(--border-color)] flex flex-col flex-shrink-0">
          <div className="p-2.5 border-b border-[var(--border-color)]">
            <div className="relative">
              <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" />
              <input
                type="text"
                placeholder="Filter keys…"
                value={keyFilter}
                onChange={e => setKeyFilter(e.target.value)}
                className="input-field pl-8 py-1.5"
              />
            </div>
          </div>

          <div className="flex items-center px-3 py-1.5 bg-[var(--bg-tertiary)] border-b border-[var(--border-color)] text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">
            <div className="flex-1">Key</div>
            <div className="w-16 text-right">Type</div>
          </div>

          <div className="flex-1 overflow-y-auto">
            {loadingKeys ? (
              <div className="flex flex-col">
                {Array.from({ length: 8 }).map((_, i) => <SkeletonRow key={i} />)}
              </div>
            ) : filteredKeys.length === 0 ? (
              <div className="empty-state">
                {keyFilter ? `No keys matching “${keyFilter}”` : 'No keys in this tenant.'}
              </div>
            ) : (
              <div>
                {filteredKeys.map(k => (
                  <button
                    type="button"
                    key={k.key}
                    onClick={() => loadKeyValue(k.key, k.type)}
                    className={`key-row w-full text-left ${selectedKey?.key === k.key ? 'selected' : ''}`}
                  >
                    <Layers size={13} className={selectedKey?.key === k.key ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)]'} />
                    <span className="key-name">{k.key}</span>
                    <span className="key-type">{k.type}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="flex-1 bg-[var(--bg-primary)] flex flex-col min-w-0">
          {selectedKey ? (
            <>
              <div className="px-4 py-3 bg-[var(--bg-panel)] border-b border-[var(--border-color)]">
                <h2 className="text-[14px] font-semibold font-mono truncate">{selectedKey.key}</h2>
                <span className="key-type mt-1 inline-block">{selectedKey.type}</span>
              </div>
              <pre className="data-viewer whitespace-pre-wrap">{keyValue}</pre>
            </>
          ) : (
            <div className="empty-state">
              <Database size={28} />
              <p>Select a key to inspect its value.</p>
            </div>
          )}
        </div>
      </div>

      {isModalOpen && (
        <div className="modal-overlay" onClick={() => setIsModalOpen(false)}>
          <div className="modal-content max-w-lg" onClick={e => e.stopPropagation()}>
            <div className="px-5 py-3 border-b border-[var(--border-color)] flex items-center justify-between">
              <h3 className="font-semibold text-[14px]">Create key</h3>
              <button type="button" onClick={() => setIsModalOpen(false)} className="text-[var(--text-muted)]">
                <Plus size={18} className="rotate-45" />
              </button>
            </div>
            <div className="p-5 space-y-4">
              <div>
                <label className="block mb-1.5">Key name</label>
                <input
                  type="text"
                  className="input-field font-mono"
                  placeholder="e.g. user:1000"
                  value={newKeyName}
                  onChange={e => setNewKeyName(e.target.value)}
                  autoFocus
                />
              </div>
              <div>
                <label className="block mb-1.5">Value (string)</label>
                <textarea
                  className="input-field font-mono h-28 resize-none"
                  placeholder='e.g. {"name": "Alice"}'
                  value={newKeyValue}
                  onChange={e => setNewKeyValue(e.target.value)}
                />
              </div>
              <div className="flex justify-end gap-2">
                <button type="button" className="btn-secondary" onClick={() => setIsModalOpen(false)}>Cancel</button>
                <button type="button" className="btn-primary" onClick={handleNewKey} disabled={saving}>
                  {saving ? 'Saving…' : 'Save key'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
