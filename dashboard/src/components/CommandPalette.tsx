import type { KeyboardEvent as ReactKeyboardEvent } from 'react';
import { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, Terminal, Database, Settings, Moon, Sun, LayoutGrid, Network, Key } from 'lucide-react';
import { useTheme } from './ThemeProvider';
import { useTenants } from './TenantProvider';

export default function CommandPalette() {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const { theme, setTheme } = useTheme();
  const { tenants } = useTenants();

  const match = window.location.pathname.match(/\/cluster\/([^/]+)/);
  const clusterId = match?.[1] || tenants[0]?.id;

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setIsOpen(prev => !prev);
      }
      if (e.key === 'Escape' && isOpen) {
        setIsOpen(false);
      }
    };
    const handleOpen = () => setIsOpen(true);
    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('dbx:open-palette', handleOpen);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('dbx:open-palette', handleOpen);
    };
  }, [isOpen]);

  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 10);
      setSearch('');
      setActive(0);
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSelect = (action: () => void) => {
    action();
    setIsOpen(false);
  };

  const commands = [
    { name: 'Go to tenants', icon: <LayoutGrid size={15} />, action: () => navigate('/') },
    ...(clusterId
      ? [
          { name: 'Go to Overview', icon: <LayoutGrid size={15} />, action: () => navigate(`/cluster/${clusterId}/overview`) },
          { name: 'Go to Data Explorer', icon: <Database size={15} />, action: () => navigate(`/cluster/${clusterId}/explorer`) },
          { name: 'Go to Vector Playground', icon: <Network size={15} />, action: () => navigate(`/cluster/${clusterId}/vector`) },
          { name: 'Go to Tenant keys', icon: <Key size={15} />, action: () => navigate(`/cluster/${clusterId}/keys`) },
          { name: 'Go to Console', icon: <Terminal size={15} />, action: () => navigate(`/cluster/${clusterId}/terminal`) },
        ]
      : []),
    { name: 'Settings', icon: <Settings size={15} />, action: () => navigate('/settings') },
    {
      name: 'Toggle dark mode',
      icon: theme === 'dark' ? <Sun size={15} /> : <Moon size={15} />,
      action: () => setTheme(theme === 'dark' ? 'light' : 'dark'),
    },
    ...tenants.map(t => ({
      name: `Open tenant ${t.name}`,
      icon: <Database size={15} />,
      action: () => navigate(`/cluster/${t.id}/overview`),
    })),
  ];

  const filtered = commands.filter(c => c.name.toLowerCase().includes(search.toLowerCase()));

  const onKeyDown = (e: ReactKeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive(i => Math.min(filtered.length - 1, i + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(i => Math.max(0, i - 1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const cmd = filtered[active];
      if (cmd) handleSelect(cmd.action);
    }
  };

  return (
    <div
      className="fixed inset-0 bg-zinc-950/40 z-[100] flex items-start justify-center pt-[12vh] p-4"
      onClick={() => setIsOpen(false)}
    >
      <div
        className="bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-lg w-full max-w-xl overflow-hidden"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-label="Command palette"
      >
        <div className="flex items-center px-3 py-2.5 border-b border-[var(--border-color)]">
          <Search size={16} className="text-[var(--text-muted)] mr-2.5 shrink-0" />
          <input
            ref={inputRef}
            type="text"
            className="flex-1 bg-transparent text-[var(--text-primary)] outline-none placeholder:text-[var(--text-muted)] text-[14px]"
            placeholder="Jump to a page or tenant…"
            value={search}
            onChange={e => { setSearch(e.target.value); setActive(0); }}
            onKeyDown={onKeyDown}
          />
          <kbd className="text-[10px] font-mono text-[var(--text-muted)] border border-[var(--border-color)] px-1.5 py-0.5 rounded">ESC</kbd>
        </div>

        <div className="max-h-80 overflow-y-auto p-1">
          {filtered.length === 0 ? (
            <div className="py-8 text-center text-[13px] text-[var(--text-muted)]">No matches.</div>
          ) : (
            filtered.map((cmd, i) => (
              <button
                key={`${cmd.name}-${i}`}
                className={`w-full flex items-center gap-2.5 px-3 py-2 rounded text-[13px] font-medium text-left ${
                  i === active
                    ? 'bg-[var(--accent-soft)] text-[var(--accent-primary)]'
                    : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)]'
                }`}
                onMouseEnter={() => setActive(i)}
                onClick={() => handleSelect(cmd.action)}
              >
                <span className="text-[var(--text-muted)]">{cmd.icon}</span>
                {cmd.name}
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
