import { useState, useEffect, useRef } from 'react';
import { Terminal, Database } from 'lucide-react';
import { fetchWithAuth } from '../api';
import gsap from 'gsap';

function ScrambleText({ text }: { text: string }) {
  const [display, setDisplay] = useState('');
  
  useEffect(() => {
    let iteration = 0;
    const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+";
    let interval: any = null;
    
    // Quick render for long texts to avoid performance issues
    if (text.length > 500) {
      setDisplay(text);
      return;
    }

    interval = setInterval(() => {
      setDisplay(text.split('').map((letter, index) => {
        if (index < iteration) return text[index];
        // Retain whitespaces
        if (letter === ' ' || letter === '\n' || letter === '\r') return letter;
        return chars[Math.floor(Math.random() * chars.length)];
      }).join(''));

      if (iteration >= text.length) clearInterval(interval);
      iteration += text.length > 50 ? 2 : 1; 
    }, 20);
    
    return () => clearInterval(interval);
  }, [text]);

  return <span>{display}</span>;
}

export default function ConsolePage({ clusterId }: { clusterId: string }) {
  const [history, setHistory] = useState<{ type: 'input' | 'output' | 'error', text: string }[]>([
    { type: 'output', text: 'DBX Interactive Console - Connected to ' + clusterId },
    { type: 'output', text: 'Type a command and press Enter (e.g. SET mykey "hello")' }
  ]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const terminalRef = useRef<HTMLDivElement>(null);

  const triggerGlow = (success: boolean) => {
    if (terminalRef.current) {
      const color = success ? 'rgba(16, 185, 129, 0.4)' : 'rgba(239, 68, 68, 0.4)';
      gsap.fromTo(terminalRef.current, 
        { boxShadow: `0 0 30px ${color}` },
        { boxShadow: '0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)', duration: 0.8, ease: 'power2.out' }
      );
    }
  };

  const handleCommand = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim()) return;

    const cmdStr = input.trim();
    setHistory(prev => [...prev, { type: 'input', text: `> ${cmdStr}` }]);
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
      triggerGlow(true);
    } catch (err: any) {
      setHistory(prev => [...prev, { type: 'error', text: `Error: ${err.message}` }]);
      triggerGlow(false);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col h-full bg-[#f4f4f5]">
      {/* Header */}
      <div className="bg-white px-8 py-5 border-b border-gray-200 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Terminal size={20} className="text-red-600" />
          <h1 className="text-xl font-bold text-slate-900 tracking-tight">CLI Console</h1>
        </div>
      </div>

      <div className="flex-1 p-6">
        <div ref={terminalRef} className="bg-[#1e1e1e] rounded-xl shadow-lg border border-gray-300 flex flex-col h-full overflow-hidden transition-all">
          {/* Terminal Header */}
          <div className="bg-[#2d2d2d] px-4 py-2 flex items-center gap-2 border-b border-black/40">
            <div className="flex gap-1.5">
              <div className="w-3 h-3 rounded-full bg-red-500"></div>
              <div className="w-3 h-3 rounded-full bg-yellow-500"></div>
              <div className="w-3 h-3 rounded-full bg-green-500"></div>
            </div>
            <div className="text-gray-400 text-xs font-mono ml-4 flex items-center gap-2">
              <Database size={12}/> {clusterId}
            </div>
          </div>
          
          {/* Terminal Output */}
          <div className="flex-1 overflow-y-auto p-4 font-mono text-sm leading-relaxed" style={{ scrollBehavior: 'smooth' }}>
            {history.map((line, idx) => (
              <div key={idx} className={`mb-1 whitespace-pre-wrap ${
                line.type === 'error' ? 'text-red-400' : 
                line.type === 'input' ? 'text-green-400 font-bold' : 'text-gray-300'
              }`}>
                {line.type === 'output' && idx > 1 ? <ScrambleText text={line.text} /> : line.text}
              </div>
            ))}
            {loading && <div className="text-gray-500 animate-pulse">Executing...</div>}
          </div>
          
          {/* Terminal Input */}
          <div className="p-4 bg-black/20 border-t border-black/40">
            <form onSubmit={handleCommand} className="flex gap-2 items-center">
              <span className="text-green-400 font-bold font-mono">127.0.0.1:6401&gt;</span>
              <input
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="Enter command..."
                className="flex-1 bg-transparent border-none text-white outline-none font-mono text-sm placeholder-gray-600"
                autoFocus
                disabled={loading}
              />
            </form>
          </div>
        </div>
      </div>
    </div>
  );
}
