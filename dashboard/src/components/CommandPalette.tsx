import { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, Terminal, Database, Settings, Moon, Sun } from 'lucide-react';
import { useTheme } from './ThemeProvider';

export default function CommandPalette() {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const { theme, setTheme } = useTheme();

  // Extract cluster ID from URL if present
  const match = window.location.pathname.match(/\/cluster\/([^\/]+)/);
  const clusterId = match ? match[1] : 'bench-tenant';

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setIsOpen(prev => !prev);
      }
      if (e.key === 'Escape' && isOpen) {
        setIsOpen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen]);

  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 10);
      setSearch('');
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSelect = (action: () => void) => {
    action();
    setIsOpen(false);
  };

  const commands = [
    { name: 'Go to Data Explorer', icon: <Database size={16}/>, action: () => navigate(`/cluster/${clusterId}/explorer`) },
    { name: 'Go to Vector Playground', icon: <Search size={16}/>, action: () => navigate(`/cluster/${clusterId}/vector`) },
    { name: 'Go to Terminal Console', icon: <Terminal size={16}/>, action: () => navigate(`/cluster/${clusterId}/terminal`) },
    { name: 'Settings & API Keys', icon: <Settings size={16}/>, action: () => navigate(`/settings`) },
    { name: 'Toggle Dark Mode', icon: theme === 'dark' ? <Sun size={16}/> : <Moon size={16}/>, action: () => setTheme(theme === 'dark' ? 'light' : 'dark') },
  ];

  const filtered = commands.filter(c => c.name.toLowerCase().includes(search.toLowerCase()));

  return (
    <div className="fixed inset-0 bg-slate-900/40 backdrop-blur-sm z-[100] flex items-start justify-center pt-[15vh] p-4" onClick={() => setIsOpen(false)}>
      <div 
        className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden animate-slide-down"
        onClick={e => e.stopPropagation()}
        style={{ animation: 'slideDown 0.2s cubic-bezier(0.16, 1, 0.3, 1)' }}
      >
        <div className="flex items-center px-4 py-4 border-b border-gray-100 dark:border-slate-800">
          <Search size={20} className="text-gray-400 dark:text-slate-500 mr-3 shrink-0" />
          <input
            ref={inputRef}
            type="text"
            className="flex-1 bg-transparent text-slate-900 dark:text-white outline-none placeholder-gray-400 dark:placeholder-slate-500 text-lg"
            placeholder="Type a command or search..."
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <button onClick={() => setIsOpen(false)} className="bg-gray-100 dark:bg-slate-800 text-gray-500 dark:text-slate-400 text-xs px-2 py-1 rounded font-mono font-bold hover:bg-gray-200 dark:hover:bg-slate-700 transition-colors">ESC</button>
        </div>
        
        <div className="max-h-80 overflow-y-auto p-2">
          {filtered.length === 0 ? (
            <div className="py-8 text-center text-sm text-gray-500 dark:text-slate-400">No commands found.</div>
          ) : (
            <div className="space-y-1">
              {filtered.map((cmd, i) => (
                <button
                  key={i}
                  className="w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium text-slate-700 dark:text-slate-300 hover:bg-red-50 dark:hover:bg-red-900/20 hover:text-red-600 dark:hover:text-red-400 transition-colors text-left"
                  onClick={() => handleSelect(cmd.action)}
                >
                  <span className="text-gray-400 dark:text-slate-500">{cmd.icon}</span>
                  {cmd.name}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
      <style dangerouslySetInnerHTML={{__html: `
        @keyframes slideDown {
          from { opacity: 0; transform: translateY(-16px) scale(0.98); }
          to { opacity: 1; transform: translateY(0) scale(1); }
        }
      `}} />
    </div>
  );
}
