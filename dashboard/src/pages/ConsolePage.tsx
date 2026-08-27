import type { FormEvent } from 'react';
import { useState, useEffect, useRef } from 'react';
import { fetchWithAuth } from '../api';
import { useTenant } from '../components/TenantProvider';
import PageChrome from '../components/PageChrome';

export default function ConsolePage({ clusterId }: { clusterId: string }) {
  const { tenant } = useTenant(clusterId);
  const respPort = tenant?.resp_port || 0;
  const promptHost = `127.0.0.1:${respPort || '—'}`;

  const [history, setHistory] = useState<{ type: 'input' | 'output' | 'error'; text: string }[]>([
    { type: 'output', text: `DBX console — tenant ${clusterId}` },
    { type: 'output', text: 'Type a RESP command and press Enter (e.g. PING or SET mykey hello)' }
  ]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const outputRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    outputRef.current?.scrollTo({ top: outputRef.current.scrollHeight });
  }, [history, loading]);

  const handleCommand = async (e: FormEvent) => {
    e.preventDefault();
    if (!input.trim()) return;

    const cmdStr = input.trim();
    setHistory(prev => [...prev, { type: 'input', text: `${promptHost}> ${cmdStr}` }]);
    setInput('');
    setLoading(true);

    const commandArgs = cmdStr.match(/(?:[^\s"]+|"[^"]*")+/g)?.map(arg => arg.replace(/^"|"$/g, '')) || [];

    try {
      const res = await fetchWithAuth(`/t/${clusterId}/query`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: commandArgs })
      });

      if (!res.ok) {
        throw new Error(await res.text());
      }

      const data = await res.json();
      setHistory(prev => [...prev, { type: 'output', text: data.response || '(nil)' }]);
    } catch (err: unknown) {
      setHistory(prev => [...prev, { type: 'error', text: err instanceof Error ? err.message : 'Command failed' }]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col h-full bg-[var(--bg-primary)]">
      <div className="px-7 pt-6 pb-3">
        <PageChrome
          clusterId={clusterId}
          title="Console"
          purpose="Issue RESP commands against this tenant’s engine. Prompt shows the live RESP port."
        />
      </div>

      <div className="flex-1 px-7 pb-6 min-h-0">
        <div className="console-term h-full">
          <div className="bg-[#1a1a1c] px-3 py-1.5 flex items-center gap-2 border-b border-[#27272a]">
            <div className="flex gap-1.5">
              <div className="w-2.5 h-2.5 rounded-full bg-[#e11d48]" />
              <div className="w-2.5 h-2.5 rounded-full bg-[#d97706]" />
              <div className="w-2.5 h-2.5 rounded-full bg-[#059669]" />
            </div>
            <div className="text-zinc-500 text-[11px] font-mono ml-2">
              {clusterId} · RESP {respPort ? `:${respPort}` : '—'}
            </div>
          </div>

          <div ref={outputRef} className="flex-1 overflow-y-auto p-3 font-mono text-[12.5px] leading-relaxed text-zinc-300">
            {history.map((line, idx) => (
              <div
                key={idx}
                className={`mb-0.5 whitespace-pre-wrap ${
                  line.type === 'error' ? 'text-rose-400' :
                  line.type === 'input' ? 'text-emerald-400' : 'text-zinc-300'
                }`}
              >
                {line.text}
              </div>
            ))}
            {loading && <div className="text-zinc-500">…</div>}
          </div>

          <form onSubmit={handleCommand} className="px-3 py-2.5 bg-[#0c0c0e] border-t border-[#27272a] flex gap-2 items-center">
            <span className="text-emerald-400 font-mono text-[12.5px] whitespace-nowrap">{promptHost}&gt;</span>
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="PING"
              className="flex-1 bg-transparent border-none text-zinc-100 outline-none font-mono text-[12.5px] placeholder-zinc-600"
              autoFocus
              disabled={loading || tenant?.status === 'down'}
              spellCheck={false}
            />
          </form>
        </div>
      </div>
    </div>
  );
}
