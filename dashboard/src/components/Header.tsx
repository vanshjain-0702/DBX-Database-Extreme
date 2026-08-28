import type { KeyboardEvent } from 'react';
import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { Search, Bell, ChevronRight, ChevronDown, Moon, Sun, Monitor } from 'lucide-react';
import { openCommandPalette, statusLabel } from '../api';
import { useTenants } from './TenantProvider';
import { useTheme } from './ThemeProvider';

export default function Header() {
  const location = useLocation();
  const navigate = useNavigate();
  const { id } = useParams();
  const { tenants } = useTenants();
  const { theme, setTheme, resolved } = useTheme();
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const [alertsOpen, setAlertsOpen] = useState(false);
  const [switcherIndex, setSwitcherIndex] = useState(0);
  const switcherRef = useRef<HTMLDivElement>(null);
  const alertsRef = useRef<HTMLDivElement>(null);

  const pathnames = location.pathname.split('/').filter(x => x);
  const current = tenants.find(t => t.id === id);
  const unhealthy = tenants.filter(t => t.status !== 'running' || !t.healthy);
  const subpage = pathnames[0] === 'cluster' ? (pathnames[2] || 'overview') : '';

  const getBreadcrumbLabel = (path: string, index: number) => {
    if (path === 'cluster') return 'Tenant';
    if (path === id) return current?.name || id;
    if (path === 'settings') return 'Settings';
    if (path === 'terminal') return 'Console';
    if (path === 'keys') return 'Tenant keys';
    if (index === 2 && id) return path.charAt(0).toUpperCase() + path.slice(1);
    return path.charAt(0).toUpperCase() + path.slice(1);
  };

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (switcherRef.current && !switcherRef.current.contains(e.target as Node)) setSwitcherOpen(false);
      if (alertsRef.current && !alertsRef.current.contains(e.target as Node)) setAlertsOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  const jumpTenant = (tenantId: string) => {
    setSwitcherOpen(false);
    if (id && subpage) {
      navigate(`/cluster/${tenantId}/${subpage}`);
    } else {
      navigate(`/cluster/${tenantId}/overview`);
    }
  };

  const onSwitcherKey = (e: KeyboardEvent) => {
    if (!switcherOpen) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        setSwitcherOpen(true);
        setSwitcherIndex(Math.max(0, tenants.findIndex(t => t.id === id)));
      }
      return;
    }
    if (e.key === 'Escape') {
      setSwitcherOpen(false);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSwitcherIndex(i => Math.min(tenants.length - 1, i + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSwitcherIndex(i => Math.max(0, i - 1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const t = tenants[switcherIndex];
      if (t) jumpTenant(t.id);
    }
  };

  const cycleTheme = () => {
    if (theme === 'light') setTheme('dark');
    else if (theme === 'dark') setTheme('system');
    else setTheme('light');
  };

  return (
    <header className="h-12 bg-[var(--bg-panel)] border-b border-[var(--border-color)] flex items-center justify-between px-4 flex-shrink-0 z-10">
      <div className="flex items-center gap-3 min-w-0">
        <nav className="flex items-center text-[13px] font-medium text-[var(--text-muted)] min-w-0" aria-label="Breadcrumb">
          {pathnames.map((value, index) => {
            const isLast = index === pathnames.length - 1;
            return (
              <div key={`${value}-${index}`} className="flex items-center min-w-0">
                <span className={`truncate ${isLast ? 'text-[var(--text-primary)] font-semibold' : ''}`}>
                  {getBreadcrumbLabel(value, index)}
                </span>
                {!isLast && <ChevronRight size={13} className="mx-1.5 text-[var(--border-highlight)] flex-shrink-0" />}
              </div>
            );
          })}
        </nav>

        {id && tenants.length > 0 && (
          <div className="relative" ref={switcherRef}>
            <button
              type="button"
              aria-haspopup="listbox"
              aria-expanded={switcherOpen}
              onClick={() => {
                setSwitcherOpen(v => !v);
                setSwitcherIndex(Math.max(0, tenants.findIndex(t => t.id === id)));
              }}
              onKeyDown={onSwitcherKey}
              className="flex items-center gap-1.5 h-7 px-2 rounded border border-[var(--border-color)] text-[12px] font-medium text-[var(--text-secondary)] hover:border-[var(--border-highlight)] hover:text-[var(--text-primary)]"
            >
              <span className="font-mono truncate max-w-[140px]">{current?.id || id}</span>
              <ChevronDown size={12} />
            </button>
            {switcherOpen && (
              <ul
                role="listbox"
                className="absolute left-0 top-full mt-1 w-72 bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md z-30 max-h-72 overflow-y-auto py-1"
              >
                {tenants.map((t, i) => (
                  <li key={t.id} role="option" aria-selected={t.id === id}>
                    <button
                      type="button"
                      onClick={() => jumpTenant(t.id)}
                      className={`w-full flex items-center justify-between gap-2 px-3 py-1.5 text-left text-[13px] ${
                        i === switcherIndex ? 'bg-[var(--accent-soft)]' : 'hover:bg-[var(--bg-tertiary)]'
                      }`}
                    >
                      <span className="min-w-0">
                        <span className="block truncate text-[var(--text-primary)] font-medium">{t.name}</span>
                        <span className="block truncate font-mono text-[11px] text-[var(--text-muted)]">{t.id}</span>
                      </span>
                      <span className={`status-badge ${t.status}`}>{statusLabel(t.status)}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={() => openCommandPalette()}
          className="flex items-center gap-2 h-7 px-2.5 border border-[var(--border-color)] rounded text-[12px] text-[var(--text-muted)] hover:border-[var(--border-highlight)] hover:text-[var(--text-secondary)]"
        >
          <Search size={13} />
          <span className="hidden sm:inline">Search</span>
          <kbd className="hidden md:inline font-mono text-[10px] border border-[var(--border-color)] px-1 rounded">⌘K</kbd>
        </button>

        <button
          type="button"
          onClick={cycleTheme}
          title={`Theme: ${theme}`}
          className="h-7 w-7 flex items-center justify-center rounded text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
        >
          {theme === 'system' ? <Monitor size={15} /> : resolved === 'dark' ? <Moon size={15} /> : <Sun size={15} />}
        </button>

        <div className="relative" ref={alertsRef}>
          <button
            type="button"
            onClick={() => setAlertsOpen(v => !v)}
            className="relative h-7 w-7 flex items-center justify-center rounded text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
            title="Tenant alerts"
          >
            <Bell size={15} />
            {unhealthy.length > 0 && (
              <span className="absolute top-1 right-1 w-1.5 h-1.5 rounded-full bg-[var(--error)]" />
            )}
          </button>
          {alertsOpen && (
            <div className="absolute right-0 top-full mt-1 w-80 bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md z-30 overflow-hidden">
              <div className="px-3 py-2 border-b border-[var(--border-color)] text-[12px] font-semibold">Alerts</div>
              {unhealthy.length === 0 ? (
                <div className="px-3 py-6 text-[13px] text-[var(--text-muted)] text-center">All listed tenants are running.</div>
              ) : (
                <ul>
                  {unhealthy.map(t => (
                    <li key={t.id}>
                      <button
                        type="button"
                        className="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-[var(--bg-tertiary)]"
                        onClick={() => {
                          setAlertsOpen(false);
                          navigate(`/cluster/${t.id}/overview`);
                        }}
                      >
                        <span>
                          <span className="block text-[13px] font-medium">{t.name}</span>
                          <span className="block font-mono text-[11px] text-[var(--text-muted)]">{t.id}</span>
                        </span>
                        <span className={`status-badge ${t.status}`}>{statusLabel(t.status)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>

        <div className="w-7 h-7 rounded-full bg-zinc-800 dark:bg-zinc-200 text-white dark:text-zinc-900 flex items-center justify-center font-semibold text-[11px]">
          A
        </div>
      </div>
    </header>
  );
}
