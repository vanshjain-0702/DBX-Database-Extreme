import type { KeyboardEvent } from 'react';
import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { Search, Bell, ChevronRight, ChevronDown, Moon, Sun, Monitor, AlertTriangle } from 'lucide-react';
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

  const themeTitle = theme === 'system' ? 'System theme' : theme === 'dark' ? 'Dark mode' : 'Light mode';
  const ThemeIcon = theme === 'system' ? Monitor : resolved === 'dark' ? Moon : Sun;

  return (
    <header
      className="h-[52px] flex items-center justify-between px-4 flex-shrink-0 z-10 relative"
      style={{
        background: 'var(--bg-glass)',
        backdropFilter: 'blur(12px)',
        WebkitBackdropFilter: 'blur(12px)',
        borderBottom: '1px solid var(--border-color)',
      }}
    >
      {/* Subtle bottom glow line */}
      <div
        className="absolute bottom-0 left-0 right-0 h-[1px] pointer-events-none"
        style={{
          background: 'linear-gradient(90deg, transparent 0%, var(--accent-glow) 40%, var(--accent-glow) 60%, transparent 100%)',
          opacity: 0.5,
        }}
      />

      {/* Left: Breadcrumb + Tenant switcher */}
      <div className="flex items-center gap-3 min-w-0">
        <nav
          className="flex items-center text-[13px] font-medium text-[var(--text-muted)] min-w-0"
          aria-label="Breadcrumb"
        >
          {pathnames.map((value, index) => {
            const isLast = index === pathnames.length - 1;
            return (
              <div key={`${value}-${index}`} className="flex items-center min-w-0">
                <span className={`truncate transition-colors ${isLast ? 'text-[var(--text-primary)] font-semibold' : 'hover:text-[var(--text-secondary)]'}`}>
                  {getBreadcrumbLabel(value, index)}
                </span>
                {!isLast && (
                  <ChevronRight size={13} className="mx-1 text-[var(--border-highlight)] flex-shrink-0 opacity-60" />
                )}
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
              className="flex items-center gap-1.5 h-7 px-2.5 rounded-md text-[12px] font-medium text-[var(--text-secondary)] transition-all duration-150"
              style={{
                border: '1px solid var(--border-color)',
                background: 'var(--bg-tertiary)',
              }}
              onMouseEnter={e => {
                (e.currentTarget as HTMLElement).style.borderColor = 'var(--border-highlight)';
                (e.currentTarget as HTMLElement).style.background = 'var(--bg-secondary)';
              }}
              onMouseLeave={e => {
                (e.currentTarget as HTMLElement).style.borderColor = 'var(--border-color)';
                (e.currentTarget as HTMLElement).style.background = 'var(--bg-tertiary)';
              }}
            >
              <span className="font-mono truncate max-w-[140px]">{current?.id || id}</span>
              <ChevronDown size={12} className="opacity-70" />
            </button>

            {switcherOpen && (
              <ul
                role="listbox"
                className="absolute left-0 top-full mt-1.5 w-72 border rounded-xl z-30 max-h-72 overflow-y-auto py-1.5"
                style={{
                  background: 'var(--bg-panel)',
                  borderColor: 'var(--border-color)',
                  boxShadow: 'var(--shadow-lg)',
                  animation: 'scale-in 0.15s cubic-bezier(0.4,0,0.2,1) both',
                }}
              >
                {tenants.map((t, i) => (
                  <li key={t.id} role="option" aria-selected={t.id === id}>
                    <button
                      type="button"
                      onClick={() => jumpTenant(t.id)}
                      className={`w-full flex items-center justify-between gap-2 px-3 py-2 text-left text-[13px] rounded-lg mx-1 transition-colors ${
                        i === switcherIndex
                          ? 'bg-[var(--accent-soft)] text-[var(--text-primary)]'
                          : 'hover:bg-[var(--bg-tertiary)]'
                      }`}
                      style={{ width: 'calc(100% - 8px)' }}
                    >
                      <span className="min-w-0">
                        <span className="block truncate text-[var(--text-primary)] font-medium">{t.name}</span>
                        <span className="block truncate font-mono text-[11px] text-[var(--text-muted)]">{t.id}</span>
                      </span>
                      <span className={`status-badge ${t.status} flex-shrink-0`}>{statusLabel(t.status)}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>

      {/* Right: Search, theme, alerts, avatar */}
      <div className="flex items-center gap-1">
        {/* Search */}
        <button
          type="button"
          onClick={() => openCommandPalette()}
          className="flex items-center gap-2 h-7 px-2.5 rounded-md text-[12px] text-[var(--text-muted)] transition-all duration-150 hover:text-[var(--text-secondary)]"
          style={{ border: '1px solid var(--border-color)', background: 'var(--bg-tertiary)' }}
          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--border-highlight)'; }}
          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--border-color)'; }}
        >
          <Search size={13} />
          <span className="hidden sm:inline">Search</span>
          <kbd className="hidden md:inline font-mono text-[10px] border border-[var(--border-color)] px-1.5 py-0.5 rounded bg-[var(--bg-primary)]">⌘K</kbd>
        </button>

        {/* Theme toggle */}
        <button
          type="button"
          onClick={cycleTheme}
          title={themeTitle}
          className="h-7 w-7 flex items-center justify-center rounded-md text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-all duration-150"
        >
          <ThemeIcon size={15} />
        </button>

        {/* Alerts */}
        <div className="relative" ref={alertsRef}>
          <button
            type="button"
            onClick={() => setAlertsOpen(v => !v)}
            className="relative h-7 w-7 flex items-center justify-center rounded-md text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-all duration-150"
            title="Tenant alerts"
          >
            <Bell size={15} />
            {unhealthy.length > 0 && (
              <span
                className="absolute top-1 right-1 w-2 h-2 rounded-full flex items-center justify-center"
                style={{
                  background: 'var(--error)',
                  boxShadow: '0 0 0 2px var(--bg-panel)',
                }}
              />
            )}
          </button>

          {alertsOpen && (
            <div
              className="absolute right-0 top-full mt-1.5 w-80 border rounded-xl z-30 overflow-hidden"
              style={{
                background: 'var(--bg-panel)',
                borderColor: 'var(--border-color)',
                boxShadow: 'var(--shadow-lg)',
                animation: 'scale-in 0.15s cubic-bezier(0.4,0,0.2,1) both',
              }}
            >
              <div className="px-3.5 py-2.5 border-b border-[var(--border-color)] flex items-center justify-between bg-[var(--bg-tertiary)]">
                <span className="text-[12px] font-bold uppercase tracking-wider text-[var(--text-muted)]">Alerts</span>
                {unhealthy.length > 0 && (
                  <span className="text-[11px] font-mono font-bold text-[var(--error)] bg-[var(--error-soft)] px-2 py-0.5 rounded-full">
                    {unhealthy.length}
                  </span>
                )}
              </div>
              {unhealthy.length === 0 ? (
                <div className="px-4 py-6 text-[13px] text-[var(--text-muted)] text-center flex flex-col items-center gap-2">
                  <span className="text-xl">✓</span>
                  All tenants are running.
                </div>
              ) : (
                <ul className="py-1">
                  {unhealthy.map(t => (
                    <li key={t.id}>
                      <button
                        type="button"
                        className="w-full flex items-center justify-between px-3 py-2.5 text-left hover:bg-[var(--bg-tertiary)] transition-colors"
                        onClick={() => {
                          setAlertsOpen(false);
                          navigate(`/cluster/${t.id}/overview`);
                        }}
                      >
                        <span className="flex items-center gap-2.5">
                          <AlertTriangle size={13} className="text-[var(--error)] flex-shrink-0" />
                          <span>
                            <span className="block text-[13px] font-medium text-[var(--text-primary)]">{t.name}</span>
                            <span className="block font-mono text-[11px] text-[var(--text-muted)]">{t.id}</span>
                          </span>
                        </span>
                        <span className={`status-badge ${t.status} flex-shrink-0`}>{statusLabel(t.status)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>

        {/* Avatar */}
        <div
          className="w-7 h-7 rounded-full flex items-center justify-center font-bold text-[11px] text-white ml-0.5"
          style={{
            background: 'linear-gradient(135deg, #c2410c, #ea580c)',
            boxShadow: '0 0 0 2px var(--bg-panel), 0 0 0 3px rgba(194,65,12,0.25)',
          }}
        >
          A
        </div>
      </div>
    </header>
  );
}
