import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Database, Plus, Settings, Search, X, Loader2, AlertCircle,
  LogOut, ChevronDown, Moon, Sun, Monitor, ArrowUpRight, Zap
} from 'lucide-react';
import { fetchWithAuth, openCommandPalette, type Tenant } from '../api';
import { useTenants } from '../components/TenantProvider';
import { useTheme } from '../components/ThemeProvider';
import { StatusBadge } from '../components/PageChrome';
import logo from '../assets/logo.jpg';

export default function ProjectsPage() {
  const navigate = useNavigate();
  const { tenants, loading: pageLoading, error: listError, refresh } = useTenants();
  const { theme, setTheme, resolved } = useTheme();
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [newId, setNewId] = useState('');
  const [newName, setNewName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [newReplicas, setNewReplicas] = useState(0);
  const [search, setSearch] = useState('');
  const [showUserMenu, setShowUserMenu] = useState(false);

  const handleNewCluster = async () => {
    if (!newId.trim() || !newName.trim()) {
      setError('Name and tenant ID are required.');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const res = await fetchWithAuth('/api/provision', {
        method: 'POST',
        body: JSON.stringify({ id: newId.trim(), name: newName.trim(), replicas: newReplicas }),
        headers: { 'Content-Type': 'application/json' },
      });
      if (res.ok) {
        setIsModalOpen(false);
        setNewId('');
        setNewReplicas(0);
        await refresh();
      } else {
        const text = await res.text();
        setError(text || 'Failed to provision tenant.');
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'An error occurred while provisioning.');
    } finally {
      setLoading(false);
    }
  };

  const handleLogout = () => {
    localStorage.removeItem('dbx_token');
    navigate('/login');
  };

  const filtered = tenants.filter(c => {
    if (c.replica_of) return false;
    const q = search.toLowerCase().trim();
    if (!q) return true;
    return (c.name || '').toLowerCase().includes(q) || (c.id || '').toLowerCase().includes(q);
  });

  const cycleTheme = () => {
    if (theme === 'light') setTheme('dark');
    else if (theme === 'dark') setTheme('system');
    else setTheme('light');
  };

  const ThemeIcon = theme === 'system' ? Monitor : resolved === 'dark' ? Moon : Sun;

  const totalTenants = tenants.filter(t => !t.replica_of).length;
  const runningTenants = tenants.filter(t => !t.replica_of && t.status === 'running').length;

  return (
    <div className="flex flex-col flex-1 w-full h-full overflow-hidden" style={{ background: 'var(--bg-primary)' }}>
      {/* Header */}
      <header
        className="flex items-center justify-between px-6 h-[52px] sticky top-0 z-10 flex-shrink-0"
        style={{
          background: 'var(--bg-glass)',
          backdropFilter: 'blur(12px)',
          WebkitBackdropFilter: 'blur(12px)',
          borderBottom: '1px solid var(--border-color)',
          boxShadow: '0 1px 0 var(--border-color)',
        }}
      >
        {/* Accent bottom gradient */}
        <div
          className="absolute bottom-0 left-0 right-0 h-[1px] pointer-events-none"
          style={{
            background: 'linear-gradient(90deg, transparent 0%, var(--accent-glow) 30%, var(--accent-glow) 70%, transparent 100%)',
            opacity: 0.5,
          }}
        />

        <div className="flex items-center gap-7">
          {/* Logo */}
          <button
            type="button"
            onClick={() => navigate('/')}
            className="flex items-center gap-2.5 group"
          >
            <div
              className="w-7 h-7 rounded-md overflow-hidden border flex-shrink-0 transition-all duration-200 group-hover:shadow-[0_0_8px_var(--accent-glow)]"
              style={{ borderColor: 'var(--border-color)' }}
            >
              <img src={logo} alt="" className="w-full h-full object-cover" />
            </div>
            <div className="font-bold text-[14px] tracking-tight text-[var(--text-primary)]">DBX</div>
          </button>

          {/* Nav tabs */}
          <nav className="hidden md:flex items-center gap-0.5">
            <button
              type="button"
              className="px-3 py-1.5 rounded-md text-[13px] font-semibold transition-all duration-150"
              style={{ background: 'var(--accent-soft)', color: 'var(--accent-primary)' }}
            >
              Tenants
            </button>
            <button
              type="button"
              onClick={() => navigate('/settings')}
              className="px-3 py-1.5 rounded-md text-[13px] font-medium text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] transition-all duration-150"
            >
              Settings
            </button>
          </nav>
        </div>

        <div className="flex items-center gap-1.5">
          {/* Search */}
          <button
            type="button"
            onClick={() => openCommandPalette()}
            className="flex items-center gap-2 h-7 px-2.5 rounded-md text-[12px] text-[var(--text-muted)] hover:text-[var(--text-secondary)] transition-all duration-150"
            style={{ border: '1px solid var(--border-color)', background: 'var(--bg-tertiary)' }}
          >
            <Search size={13} />
            <span className="hidden sm:inline">Search</span>
            <kbd className="hidden md:inline font-mono text-[10px] border border-[var(--border-color)] px-1.5 py-0.5 rounded bg-[var(--bg-primary)]">⌘K</kbd>
          </button>

          {/* Theme */}
          <button
            type="button"
            onClick={cycleTheme}
            className="h-7 w-7 flex items-center justify-center rounded-md text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-all duration-150"
            title={`Theme: ${theme}`}
          >
            <ThemeIcon size={15} />
          </button>

          {/* Settings */}
          <button
            type="button"
            onClick={() => navigate('/settings')}
            title="Settings"
            className="h-7 w-7 flex items-center justify-center rounded-md text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-all duration-150"
          >
            <Settings size={15} />
          </button>

          {/* User menu */}
          <div className="relative">
            <button
              type="button"
              onClick={() => setShowUserMenu(v => !v)}
              className="flex items-center gap-1.5 p-1 rounded-md hover:bg-[var(--bg-tertiary)] transition-all duration-150"
            >
              <div
                className="w-7 h-7 rounded-full flex items-center justify-center font-bold text-[11px] text-white"
                style={{
                  background: 'linear-gradient(135deg, #c2410c, #ea580c)',
                  boxShadow: '0 0 0 2px var(--bg-panel), 0 0 0 3px rgba(194,65,12,0.3)',
                }}
              >
                A
              </div>
              <ChevronDown size={12} className="text-[var(--text-muted)] opacity-70" />
            </button>

            {showUserMenu && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setShowUserMenu(false)} />
                <div
                  className="absolute right-0 top-full mt-1.5 w-52 border rounded-xl z-20 overflow-hidden"
                  style={{
                    background: 'var(--bg-panel)',
                    borderColor: 'var(--border-color)',
                    boxShadow: 'var(--shadow-lg)',
                    animation: 'scale-in 0.15s ease both',
                  }}
                >
                  <div className="px-3 py-2.5 border-b border-[var(--border-color)] bg-[var(--bg-tertiary)]">
                    <div className="text-[13px] font-semibold text-[var(--text-primary)]">Admin</div>
                    <div className="text-[11px] text-[var(--text-muted)]">control plane</div>
                  </div>
                  <div className="p-1.5 space-y-0.5">
                    <button
                      type="button"
                      onClick={() => { navigate('/settings'); setShowUserMenu(false); }}
                      className="w-full flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] rounded-md transition-colors"
                    >
                      <Settings size={13} /> Settings
                    </button>
                    <div className="divider" />
                    <button
                      type="button"
                      onClick={handleLogout}
                      className="w-full flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] rounded-md transition-colors"
                      style={{ color: 'var(--error)' }}
                      onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--error-soft)'}
                      onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}
                    >
                      <LogOut size={13} /> Sign out
                    </button>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </header>

      {/* Main */}
      <main className="flex-1 min-h-0 w-full overflow-y-auto" style={{ padding: '32px 28px' }}>
        {/* Page header */}
        <div
          className="flex flex-col md:flex-row md:items-end justify-between gap-5 mb-8"
          style={{ animation: 'fade-in-up 0.35s ease both' }}
        >
          <div>
            <h1 className="text-[24px] font-bold tracking-tight mb-1.5" style={{ letterSpacing: '-0.03em' }}>
              Tenants
            </h1>
            <p className="text-[var(--text-muted)] text-[13px]">
              Isolated per-tenant memory engines — KV + vector in one engine, one backup, one delete.
            </p>
            {totalTenants > 0 && (
              <div className="flex items-center gap-4 mt-2.5">
                <span className="flex items-center gap-1.5 text-[12px] font-mono text-[var(--text-muted)]">
                  <span className="w-1.5 h-1.5 rounded-full" style={{ background: 'var(--success)', boxShadow: '0 0 4px var(--success)' }} />
                  {runningTenants}/{totalTenants} running
                </span>
                {totalTenants - runningTenants > 0 && (
                  <span className="flex items-center gap-1.5 text-[12px] font-mono" style={{ color: 'var(--error)' }}>
                    <span className="w-1.5 h-1.5 rounded-full bg-[var(--error)]" />
                    {totalTenants - runningTenants} down
                  </span>
                )}
              </div>
            )}
          </div>

          <div className="flex items-center gap-2.5">
            {/* Search */}
            <div className="relative">
              <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] pointer-events-none" />
              <input
                type="text"
                placeholder="Filter tenants…"
                value={search}
                onChange={e => setSearch(e.target.value)}
                className="w-56 pl-8 pr-3 py-2 text-[13px] rounded-xl outline-none transition-all duration-200"
                style={{
                  background: 'var(--bg-panel)',
                  border: '1px solid var(--border-color)',
                  color: 'var(--text-primary)',
                  boxShadow: 'var(--shadow-sm)',
                }}
                onFocus={e => {
                  e.currentTarget.style.borderColor = 'var(--accent-primary)';
                  e.currentTarget.style.boxShadow = '0 0 0 3px var(--accent-soft)';
                }}
                onBlur={e => {
                  e.currentTarget.style.borderColor = 'var(--border-color)';
                  e.currentTarget.style.boxShadow = 'var(--shadow-sm)';
                }}
              />
              {search && (
                <button
                  type="button"
                  onClick={() => setSearch('')}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
                >
                  <X size={13} />
                </button>
              )}
            </div>
            <button
              type="button"
              className="btn-primary"
              onClick={() => setIsModalOpen(true)}
            >
              <Plus size={15} />
              Provision
            </button>
          </div>
        </div>

        {listError && <div className="alert-error mb-5">{listError}</div>}

        {/* Content */}
        {pageLoading ? (
          <div className="flex flex-col items-center justify-center py-24 gap-3 text-[var(--text-muted)]">
            <Loader2 size={22} className="animate-spin" style={{ color: 'var(--accent-primary)' }} />
            <span className="text-[13px]">Loading tenants…</span>
          </div>
        ) : tenants.filter(t => !t.replica_of).length === 0 ? (
          <div
            className="flex flex-col items-center text-center py-20 rounded-2xl"
            style={{
              border: '2px dashed var(--border-color)',
              background: 'var(--bg-panel)',
              animation: 'fade-in 0.3s ease both',
            }}
          >
            <div
              className="w-14 h-14 rounded-2xl flex items-center justify-center mb-5"
              style={{ background: 'var(--accent-soft)', border: '1px solid var(--accent-glow)' }}
            >
              <Database size={24} style={{ color: 'var(--accent-primary)' }} />
            </div>
            <div className="text-[16px] font-semibold mb-2">No tenants yet</div>
            <p className="text-[13px] text-[var(--text-muted)] mb-6 max-w-xs leading-relaxed">
              Provision an isolated engine for a customer — one API call gives you KV + vector memory.
            </p>
            <button type="button" className="btn-primary" onClick={() => setIsModalOpen(true)}>
              <Plus size={15} />
              Provision first tenant
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4 animate-children">
            {filtered.map(cluster => (
              <TenantCard key={cluster.id} tenant={cluster} onOpen={() => navigate(`/cluster/${cluster.id}/overview`)} />
            ))}

            {!pageLoading && filtered.length === 0 && search && (
              <div className="col-span-full flex flex-col items-center justify-center py-16 text-[var(--text-muted)]">
                <Search size={24} className="mb-3 opacity-40" />
                <div className="text-[14px] font-medium mb-1">No results for "{search}"</div>
                <button type="button" onClick={() => setSearch('')} className="mt-2 text-[13px]" style={{ color: 'var(--accent-primary)' }}>
                  Clear filter
                </button>
              </div>
            )}
          </div>
        )}
      </main>

      {/* Provision Modal */}
      {isModalOpen && (
        <div className="modal-overlay" onClick={() => setIsModalOpen(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            {/* Accent top strip */}
            <div className="h-[2px]" style={{ background: 'linear-gradient(90deg, #c2410c, #ea580c)' }} />

            <div className="px-6 py-4 border-b border-[var(--border-color)] flex items-center justify-between">
              <div>
                <h3 className="font-bold text-[15px] tracking-tight">Provision tenant</h3>
                <p className="text-[12px] text-[var(--text-muted)] mt-0.5">Creates an isolated engine with its own WAL and vector index.</p>
              </div>
              <button
                type="button"
                onClick={() => setIsModalOpen(false)}
                className="text-[var(--text-muted)] p-1.5 rounded-md hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-all duration-150"
              >
                <X size={16} />
              </button>
            </div>

            <div className="p-6 space-y-5">
              <div>
                <label className="block mb-1.5">Name</label>
                <input
                  type="text"
                  className="input-field"
                  placeholder="e.g. Acme Support"
                  value={newName}
                  onChange={e => setNewName(e.target.value)}
                  autoFocus
                />
              </div>
              <div>
                <label className="block mb-1.5">Tenant ID</label>
                <input
                  type="text"
                  className="input-field font-mono"
                  placeholder="e.g. acme-support"
                  value={newId}
                  onChange={e => setNewId(e.target.value)}
                />
                <p className="text-[var(--text-muted)] text-[12px] mt-1.5">Alphanumeric and dashes. Used in API routing.</p>
              </div>
              <div>
                <label className="block mb-1.5">Replicas</label>
                <select
                  className="input-field"
                  value={newReplicas}
                  onChange={e => setNewReplicas(Number(e.target.value))}
                >
                  <option value={0}>None (single-node, certified path)</option>
                  <option value={1}>1 async WAL replica</option>
                  <option value={2}>2 async WAL replicas</option>
                </select>
                <p className="text-[var(--text-muted)] text-[12px] mt-1.5">Replicas do not add write RTT. Failover is promote, not Raft.</p>
              </div>

              {error && (
                <div className="alert-error">
                  <AlertCircle size={14} className="flex-shrink-0" />
                  {error}
                </div>
              )}

              <div className="pt-1 flex items-center justify-end gap-2.5">
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={() => { setIsModalOpen(false); setError(''); }}
                >
                  Cancel
                </button>
                <button
                  type="button"
                  className="btn-primary"
                  onClick={handleNewCluster}
                  disabled={loading}
                >
                  {loading ? (
                    <><Loader2 size={14} className="animate-spin" /> Provisioning…</>
                  ) : (
                    <><Zap size={14} /> Provision</>
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function TenantCard({ tenant, onOpen }: { tenant: Tenant; onOpen: () => void }) {
  const down = tenant.status === 'down';

  return (
    <button
      type="button"
      onClick={onOpen}
      className={`w-full block text-left rounded-xl p-5 transition-all duration-200 group relative overflow-hidden ${down ? 'tenant-card-down' : ''}`}
      style={{
        background: 'var(--bg-panel)',
        border: '1px solid var(--border-color)',
        boxShadow: 'var(--shadow-sm)',
      }}
    >
      {/* Subtle top accent line on hover */}
      <div
        className="absolute top-0 left-0 right-0 h-[2px] opacity-0 group-hover:opacity-100 transition-opacity duration-200"
        style={{ background: 'linear-gradient(90deg, #c2410c, #ea580c)' }}
      />

      {/* Header */}
      <div className="flex items-start justify-between mb-4 gap-2">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div
            className="w-10 h-10 rounded-xl flex items-center justify-center flex-shrink-0 transition-all duration-200 group-hover:shadow-[0_0_12px_rgba(194,65,12,0.3)]"
            style={{
              background: down ? 'var(--bg-tertiary)' : 'rgba(194,65,12,0.1)',
              border: `1px solid ${down ? 'var(--border-color)' : 'rgba(194,65,12,0.25)'}`,
              color: down ? 'var(--text-muted)' : '#c2410c',
            }}
          >
            <Database size={17} />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="font-semibold text-[14px] truncate text-[var(--text-primary)] leading-tight">
              {tenant.name}
            </h3>
            <div className="text-[var(--text-muted)] text-[11px] font-mono mt-0.5 truncate">
              {tenant.id}
            </div>
            {(tenant.replicas?.length || tenant.role === 'primary') && (
              <div className="text-[11px] text-[var(--text-muted)] mt-0.5">
                {tenant.replicas?.length
                  ? `${tenant.replicas.length} replica${tenant.replicas.length === 1 ? '' : 's'}`
                  : 'primary'}
              </div>
            )}
          </div>
        </div>

        <div className="flex flex-col items-end gap-2 flex-shrink-0">
          <StatusBadge tenant={tenant} />
          <ArrowUpRight
            size={13}
            className="opacity-0 group-hover:opacity-60 transition-opacity duration-150 text-[#c2410c]"
          />
        </div>
      </div>

      {/* Port info */}
      <div
        className="pt-3.5 border-t grid grid-cols-2 gap-3"
        style={{ borderColor: 'var(--border-color)' }}
      >
        <div>
          <div className="text-[10px] font-bold uppercase tracking-[0.08em] text-[var(--text-muted)] mb-1">RESP</div>
          <div className="text-[13px] font-mono text-[var(--text-primary)]">
            {tenant.resp_port ? `:${tenant.resp_port}` : '—'}
          </div>
        </div>
        <div>
          <div className="text-[10px] font-bold uppercase tracking-[0.08em] text-[var(--text-muted)] mb-1">HTTP</div>
          <div className="text-[13px] font-mono text-[var(--text-primary)]">
            {tenant.http_port ? `:${tenant.http_port}` : '—'}
          </div>
        </div>
      </div>
    </button>
  );
}
