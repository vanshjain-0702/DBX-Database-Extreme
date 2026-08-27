import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Database, Plus, Settings, Search, X, Loader2, AlertCircle, LogOut, ChevronDown, Moon, Sun, Monitor } from 'lucide-react';
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

  return (
    <div className="flex flex-col h-full bg-[var(--bg-primary)]">
      <header className="flex items-center justify-between px-6 h-12 border-b border-[var(--border-color)] bg-[var(--bg-panel)] sticky top-0 z-10">
        <div className="flex items-center gap-8">
          <div className="flex items-center gap-2.5">
            <div className="w-7 h-7 rounded overflow-hidden border border-[var(--border-color)]">
              <img src={logo} alt="" className="w-full h-full object-cover" />
            </div>
            <div className="text-[var(--text-primary)] font-semibold text-[14px] tracking-tight">DBX</div>
          </div>
          <nav className="hidden md:flex items-center gap-0.5">
            <button type="button" className="px-2.5 py-1 rounded text-[13px] font-medium bg-[var(--accent-soft)] text-[var(--accent-primary)]">
              Tenants
            </button>
            <button
              type="button"
              onClick={() => navigate('/settings')}
              className="px-2.5 py-1 rounded text-[13px] font-medium text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]"
            >
              Settings
            </button>
          </nav>
        </div>

        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => openCommandPalette()}
            className="h-7 px-2.5 border border-[var(--border-color)] rounded text-[12px] text-[var(--text-muted)] hover:text-[var(--text-secondary)]"
          >
            Search <kbd className="ml-1.5 font-mono text-[10px] border border-[var(--border-color)] px-1 rounded">⌘K</kbd>
          </button>
          <button
            type="button"
            onClick={cycleTheme}
            className="h-7 w-7 flex items-center justify-center rounded text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)]"
            title={`Theme: ${theme}`}
          >
            {theme === 'system' ? <Monitor size={15} /> : resolved === 'dark' ? <Moon size={15} /> : <Sun size={15} />}
          </button>
          <button
            type="button"
            onClick={() => navigate('/settings')}
            title="Settings"
            className="text-[var(--text-muted)] hover:text-[var(--text-primary)] p-1.5 rounded hover:bg-[var(--bg-tertiary)]"
          >
            <Settings size={16} />
          </button>

          <div className="relative">
            <button
              type="button"
              onClick={() => setShowUserMenu(v => !v)}
              className="flex items-center gap-1.5 p-1 rounded hover:bg-[var(--bg-tertiary)]"
            >
              <div className="w-7 h-7 rounded-full bg-zinc-800 dark:bg-zinc-200 text-white dark:text-zinc-900 flex items-center justify-center font-semibold text-[11px]">
                A
              </div>
              <ChevronDown size={13} className="text-[var(--text-muted)]" />
            </button>

            {showUserMenu && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setShowUserMenu(false)} />
                <div className="absolute right-0 top-full mt-1 w-52 bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md z-20 overflow-hidden">
                  <div className="px-3 py-2.5 border-b border-[var(--border-color)]">
                    <div className="text-[13px] font-semibold">Admin</div>
                    <div className="text-[11px] text-[var(--text-muted)]">control plane</div>
                  </div>
                  <div className="p-1">
                    <button
                      type="button"
                      onClick={() => { navigate('/settings'); setShowUserMenu(false); }}
                      className="w-full flex items-center gap-2 px-2.5 py-1.5 text-[13px] hover:bg-[var(--bg-tertiary)] rounded"
                    >
                      Settings
                    </button>
                    <button
                      type="button"
                      onClick={handleLogout}
                      className="w-full flex items-center gap-2 px-2.5 py-1.5 text-[13px] text-[var(--error)] hover:bg-rose-50 dark:hover:bg-rose-950/40 rounded"
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

      <main className="flex-1 w-full px-7 py-6 overflow-y-auto">
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 mb-6">
          <div>
            <h1 className="text-[20px] font-semibold tracking-tight mb-1">Tenants</h1>
            <p className="text-[var(--text-muted)] text-[13px]">Isolated in-memory + vector engines. Status comes from the orchestrator, not a default Ready badge.</p>
          </div>
          <div className="flex items-center gap-2">
            <div className="relative">
              <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-muted)] pointer-events-none" />
              <input
                type="text"
                placeholder="Filter by name or id…"
                value={search}
                onChange={e => setSearch(e.target.value)}
                className="w-64 bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md pl-8 pr-3 py-2 text-[13px] outline-none focus:border-[var(--accent-primary)]"
              />
            </div>
            <button
              type="button"
              className="btn-primary"
              onClick={() => setIsModalOpen(true)}
            >
              <Plus size={15} /> Provision
            </button>
          </div>
        </div>

        {listError && (
          <div className="alert-error mb-4">{listError}</div>
        )}

        {pageLoading ? (
          <div className="flex items-center justify-center py-20 text-[var(--text-muted)]">
            <Loader2 size={18} className="animate-spin mr-2" />
            <span className="text-[13px]">Loading tenants…</span>
          </div>
        ) : tenants.filter(t => !t.replica_of).length === 0 ? (
          <div className="border border-dashed border-[var(--border-color)] rounded-lg py-16 flex flex-col items-center text-center">
            <Database size={28} className="text-[var(--text-muted)] mb-3" />
            <div className="text-[14px] font-medium mb-1">No tenants in this fleet</div>
            <p className="text-[13px] text-[var(--text-muted)] mb-4">Provision an isolated engine for a customer.</p>
            <button type="button" className="btn-primary" onClick={() => setIsModalOpen(true)}>
              <Plus size={15} /> Provision tenant
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
            {filtered.map(cluster => (
              <TenantCard key={cluster.id} tenant={cluster} onOpen={() => navigate(`/cluster/${cluster.id}/overview`)} />
            ))}

            {!pageLoading && filtered.length === 0 && search && (
              <div className="col-span-full flex flex-col items-center justify-center py-14 text-[var(--text-muted)]">
                <Search size={24} className="mb-2 opacity-50" />
                <div className="text-[13px] font-medium">No tenants match “{search}”</div>
                <button type="button" onClick={() => setSearch('')} className="mt-2 text-[12px] text-[var(--accent-primary)]">
                  Clear search
                </button>
              </div>
            )}
          </div>
        )}
      </main>

      {isModalOpen && (
        <div className="modal-overlay" onClick={() => setIsModalOpen(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            <div className="px-5 py-3 border-b border-[var(--border-color)] flex items-center justify-between">
              <h3 className="font-semibold text-[14px]">Provision tenant</h3>
              <button type="button" onClick={() => setIsModalOpen(false)} className="text-[var(--text-muted)] p-1 rounded hover:bg-[var(--bg-tertiary)]">
                <X size={16} />
              </button>
            </div>

            <div className="p-5 space-y-4">
              <div>
                <label className="block mb-1.5">Name</label>
                <input
                  type="text"
                  className="input-field"
                  placeholder="e.g. acme-support"
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
                <p className="text-[var(--text-muted)] text-[12px] mt-1.5">Alphanumeric and dashes. Used for routing.</p>
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
                <div className="flex items-start gap-2 p-3 bg-rose-50 dark:bg-rose-950/30 border border-rose-200 dark:border-rose-900 rounded-md text-[var(--error)] text-[13px]">
                  <AlertCircle size={14} className="mt-0.5 flex-shrink-0" />
                  {error}
                </div>
              )}

              <div className="pt-1 flex items-center justify-end gap-2">
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
                  {loading ? <><Loader2 size={14} className="animate-spin" /> Provisioning…</> : 'Provision'}
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
      className={`text-left bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-lg p-4 hover:border-[var(--accent-primary)] transition-colors ${down ? 'tenant-card-down' : ''}`}
    >
      <div className="flex items-start justify-between mb-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-9 h-9 rounded-md bg-[var(--accent-soft)] text-[var(--accent-primary)] border border-[var(--border-color)] flex items-center justify-center flex-shrink-0">
            <Database size={16} />
          </div>
          <div className="min-w-0">
            <h3 className="font-medium text-[14px] truncate">{tenant.name}</h3>
            <div className="text-[var(--text-muted)] text-[11px] font-mono mt-0.5 truncate">{tenant.id}</div>
            {(tenant.replicas?.length || tenant.role === 'primary') && (
              <div className="text-[11px] text-[var(--text-muted)] mt-1">
                {tenant.replicas?.length ? `${tenant.replicas.length} replica${tenant.replicas.length === 1 ? '' : 's'}` : 'primary'}
              </div>
            )}
          </div>
        </div>
        <StatusBadge tenant={tenant} />
      </div>

      <div className="pt-3 border-t border-[var(--border-color)] grid grid-cols-2 gap-3">
        <div>
          <div className="page-label mb-1">RESP</div>
          <div className="text-[13px] font-mono">{tenant.resp_port ? `:${tenant.resp_port}` : '—'}</div>
        </div>
        <div>
          <div className="page-label mb-1">HTTP</div>
          <div className="text-[13px] font-mono">{tenant.http_port ? `:${tenant.http_port}` : '—'}</div>
        </div>
      </div>
    </button>
  );
}
