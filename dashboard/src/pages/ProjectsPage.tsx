import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Database, Plus, Settings, Server, Search, Globe, X, Loader2, AlertCircle, LogOut, ChevronDown } from 'lucide-react';
import { fetchWithAuth } from '../api';
import logo from '../assets/logo.jpg';

interface Cluster {
  id: string;
  name: string;
  region?: string;
  engine?: string;
  status?: string;
}

export default function ProjectsPage() {
  const navigate = useNavigate();
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [newId, setNewId] = useState('');
  const [newName, setNewName] = useState('');
  const [loading, setLoading] = useState(false);
  const [pageLoading, setPageLoading] = useState(true);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [activeNav, setActiveNav] = useState('Overview');
  const [showUserMenu, setShowUserMenu] = useState(false);

  useEffect(() => {
    fetchWithAuth('/api/tenants')
      .then(res => res.json())
      .then(data => setClusters(Array.isArray(data) ? data : []))
      .catch(err => {
        console.error(err);
        setClusters([]);
      })
      .finally(() => setPageLoading(false));
  }, []);

  const handleNewCluster = async () => {
    if (!newId.trim() || !newName.trim()) {
      setError('Deployment Name and Namespace ID are required.');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const res = await fetchWithAuth('/api/provision', {
        method: 'POST',
        body: JSON.stringify({ id: newId.trim(), name: newName.trim() }),
        headers: { 'Content-Type': 'application/json' },
      });
      if (res.ok) {
        const newCluster = await res.json();
        setClusters(prev => [...prev, newCluster]);
        setIsModalOpen(false);
        setNewId('');
        setNewName('');
      } else {
        const text = await res.text();
        setError(text || 'Failed to create cluster. Please try again.');
      }
    } catch (e: any) {
      setError(e.message || 'An error occurred while creating the cluster.');
    } finally {
      setLoading(false);
    }
  };

  const handleLogout = () => {
    localStorage.removeItem('dbx_token');
    navigate('/login');
  };

  const filteredClusters = clusters.filter(c => {
    const q = search.toLowerCase().trim();
    if (!q) return true;
    return (
      (c.name || '').toLowerCase().includes(q) ||
      (c.id || '').toLowerCase().includes(q) ||
      (c.region || '').toLowerCase().includes(q)
    );
  });

  const navItems = ['Overview', 'Integrations', 'Activity', 'Domains', 'Usage'];

  return (
    <div className="projects-page flex flex-col h-full bg-[#f4f4f5]">
      {/* ── Top Navbar ── */}
      <header className="flex items-center justify-between px-8 py-3.5 border-b border-gray-200 bg-white sticky top-0 z-10 shadow-sm">
        <div className="flex items-center gap-10">
          {/* Brand */}
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg overflow-hidden border border-gray-200 shadow-sm">
              <img src={logo} alt="DBX Logo" className="w-full h-full object-cover" />
            </div>
            <div className="text-slate-900 font-bold text-[17px] tracking-tight">DBX Cloud</div>
          </div>

          {/* Nav links */}
          <nav className="hidden md:flex items-center gap-1">
            {navItems.map(item => (
              <button
                key={item}
                onClick={() => setActiveNav(item)}
                className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                  activeNav === item
                    ? 'bg-red-50 text-red-600'
                    : 'text-gray-500 hover:text-slate-900 hover:bg-gray-100'
                }`}
              >
                {item}
              </button>
            ))}
          </nav>
        </div>

        <div className="flex items-center gap-3">
          <button className="text-sm font-medium text-gray-500 hover:text-slate-900 px-3 py-1.5 rounded-lg hover:bg-gray-100 transition-colors">
            Feedback
          </button>
          <div className="w-px h-5 bg-gray-200" />
          <button
            onClick={() => navigate('/settings')}
            title="Settings"
            className="text-gray-500 hover:text-slate-900 p-1.5 rounded-lg hover:bg-gray-100 transition-colors"
          >
            <Settings size={17} />
          </button>

          {/* User avatar dropdown */}
          <div className="relative">
            <button
              onClick={() => setShowUserMenu(v => !v)}
              className="flex items-center gap-2 p-1 rounded-lg hover:bg-gray-100 transition-colors"
            >
              <div className="w-8 h-8 rounded-full bg-gradient-to-br from-red-500 to-red-600 flex items-center justify-center font-bold text-white text-sm shadow-inner">
                A
              </div>
              <ChevronDown size={14} className="text-gray-400" />
            </button>

            {showUserMenu && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setShowUserMenu(false)} />
                <div className="absolute right-0 top-full mt-2 w-52 bg-white border border-gray-200 rounded-xl shadow-xl overflow-hidden z-20">
                  <div className="px-4 py-3 border-b border-gray-100 bg-gray-50">
                    <div className="text-sm font-semibold text-slate-900">Admin User</div>
                    <div className="text-xs text-gray-500 mt-0.5">admin@dbx.local</div>
                  </div>
                  <div className="p-1.5">
                    <button
                      onClick={() => { navigate('/settings'); setShowUserMenu(false); }}
                      className="w-full flex items-center gap-2.5 px-3 py-2 text-sm text-slate-700 hover:bg-gray-100 rounded-lg transition-colors"
                    >
                      <Settings size={14} /> Account Settings
                    </button>
                    <div className="border-t border-gray-100 my-1" />
                    <button
                      onClick={handleLogout}
                      className="w-full flex items-center gap-2.5 px-3 py-2 text-sm text-red-500 hover:bg-red-50 rounded-lg transition-colors"
                    >
                      <LogOut size={14} /> Sign Out
                    </button>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </header>

      {/* ── Main Content ── */}
      <main className="flex-1 w-full px-8 py-10 overflow-y-auto">
        {/* Page Header */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-8">
          <div>
            <h1 className="text-3xl font-bold text-slate-900 tracking-tight mb-1.5">Deployments</h1>
            <p className="text-gray-500 text-[15px]">Manage and monitor your serverless database clusters.</p>
          </div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
              <input
                type="text"
                placeholder="Search deployments…"
                value={search}
                onChange={e => setSearch(e.target.value)}
                className="w-64 bg-white border border-gray-200 rounded-xl pl-9 pr-4 py-2.5 text-sm text-slate-700 placeholder-gray-400 outline-none focus:border-red-500 focus:ring-2 focus:ring-red-500/20 transition-all"
              />
            </div>
            <button
              className="bg-red-600 text-white hover:bg-red-700 px-4 py-2.5 rounded-xl font-semibold text-sm flex items-center gap-2 transition-colors shadow-sm hover:shadow-md hover:-translate-y-0.5 active:translate-y-0"
              onClick={() => setIsModalOpen(true)}
            >
              <Plus size={16} /> Add New
            </button>
          </div>
        </div>

        {/* Cluster Grid */}
        {pageLoading ? (
          <div className="flex items-center justify-center py-24 text-gray-400">
            <Loader2 size={28} className="animate-spin mr-3" />
            <span className="text-sm font-medium">Loading deployments…</span>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-5">
            {filteredClusters.map(cluster => (
              <div
                key={cluster.id}
                className="bg-white border border-gray-200 hover:border-red-300 rounded-2xl p-6 cursor-pointer transition-all hover:shadow-lg group flex flex-col"
                onClick={() => navigate(`/cluster/${cluster.id}/overview`)}
              >
                <div className="flex items-start justify-between mb-5">
                  <div className="flex items-center gap-3.5">
                    <div className="w-11 h-11 rounded-xl bg-red-50 text-red-600 border border-red-100 flex items-center justify-center group-hover:scale-105 transition-transform">
                      <Database size={20} />
                    </div>
                    <div>
                      <h3 className="text-slate-900 font-semibold text-[15px] group-hover:text-red-600 transition-colors leading-tight">
                        {cluster.name}
                      </h3>
                      <div className="text-gray-400 text-xs font-mono mt-1">{cluster.id}</div>
                    </div>
                  </div>
                  <span className="flex items-center gap-1.5 px-2.5 py-1 bg-emerald-50 border border-emerald-200 text-emerald-600 text-xs font-bold uppercase tracking-wider rounded-lg">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                    {cluster.status || 'Ready'}
                  </span>
                </div>

                <div className="mt-auto pt-4 border-t border-gray-100 grid grid-cols-2 gap-4">
                  <div>
                    <div className="text-[11px] text-gray-400 font-semibold uppercase tracking-wider mb-1">Region</div>
                    <div className="text-slate-700 text-sm flex items-center gap-1.5">
                      <Globe size={12} className="text-gray-400" />
                      {cluster.region || 'us-east-1'}
                    </div>
                  </div>
                  <div>
                    <div className="text-[11px] text-gray-400 font-semibold uppercase tracking-wider mb-1">Engine</div>
                    <div className="text-slate-700 text-sm flex items-center gap-1.5">
                      <Server size={12} className="text-gray-400" />
                      {cluster.engine || 'DBX v2.0'}
                    </div>
                  </div>
                </div>
              </div>
            ))}

            {/* No results */}
            {!pageLoading && filteredClusters.length === 0 && search && (
              <div className="col-span-full flex flex-col items-center justify-center py-16 text-gray-400">
                <Search size={32} className="mb-3 opacity-40" />
                <div className="text-sm font-medium">No deployments match "{search}"</div>
                <button onClick={() => setSearch('')} className="mt-2 text-xs text-red-500 hover:underline">Clear search</button>
              </div>
            )}

            {/* Add new card */}
            <div
              onClick={() => setIsModalOpen(true)}
              className="border-2 border-dashed border-gray-200 hover:border-red-300 rounded-2xl p-6 cursor-pointer transition-all hover:bg-red-50/30 flex flex-col items-center justify-center text-center h-[152px] group"
            >
              <div className="w-10 h-10 rounded-full bg-gray-100 group-hover:bg-red-100 group-hover:text-red-600 flex items-center justify-center text-gray-400 mb-3 transition-colors">
                <Plus size={22} />
              </div>
              <div className="text-slate-700 font-semibold text-sm group-hover:text-red-600 transition-colors">Create New Deployment</div>
              <div className="text-gray-400 text-xs mt-1">Spin up a new serverless cluster</div>
            </div>
          </div>
        )}
      </main>

      {/* ── Create Deployment Modal ── */}
      {isModalOpen && (
        <div
          className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4"
          onClick={() => setIsModalOpen(false)}
        >
          <div
            className="bg-white border border-gray-200 rounded-2xl w-full max-w-md shadow-2xl overflow-hidden"
            onClick={e => e.stopPropagation()}
          >
            <div className="px-6 py-4 border-b border-gray-100 bg-gray-50 flex items-center justify-between">
              <h3 className="text-slate-900 font-bold text-base">Create Deployment</h3>
              <button
                onClick={() => setIsModalOpen(false)}
                className="text-gray-400 hover:text-slate-900 transition-colors p-1 rounded-lg hover:bg-gray-200"
              >
                <X size={18} />
              </button>
            </div>

            <div className="p-6 space-y-5">
              <div>
                <label className="block text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">
                  Deployment Name
                </label>
                <input
                  type="text"
                  className="w-full bg-gray-50 border border-gray-200 text-slate-900 text-sm rounded-xl px-3.5 py-2.5 outline-none focus:border-red-500 focus:ring-2 focus:ring-red-500/20 transition-all"
                  placeholder="e.g. production-api"
                  value={newName}
                  onChange={e => setNewName(e.target.value)}
                  autoFocus
                />
              </div>
              <div>
                <label className="block text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">
                  Namespace ID
                </label>
                <input
                  type="text"
                  className="w-full bg-gray-50 border border-gray-200 text-slate-900 text-sm rounded-xl px-3.5 py-2.5 outline-none focus:border-red-500 focus:ring-2 focus:ring-red-500/20 transition-all font-mono"
                  placeholder="e.g. prod-db-1"
                  value={newId}
                  onChange={e => setNewId(e.target.value)}
                />
                <p className="text-gray-400 text-xs mt-1.5">Alphanumeric and dashes only. Used for routing.</p>
              </div>

              {error && (
                <div className="flex items-start gap-2 p-3.5 bg-red-50 border border-red-100 rounded-xl text-red-600 text-sm">
                  <AlertCircle size={15} className="mt-0.5 flex-shrink-0" />
                  {error}
                </div>
              )}

              <div className="pt-2 flex items-center justify-end gap-3">
                <button
                  className="px-4 py-2 text-sm font-medium text-gray-500 hover:text-slate-900 rounded-xl hover:bg-gray-100 transition-colors"
                  onClick={() => { setIsModalOpen(false); setError(''); }}
                >
                  Cancel
                </button>
                <button
                  className="bg-red-600 text-white hover:bg-red-700 px-5 py-2 rounded-xl font-semibold text-sm transition-colors shadow-sm hover:shadow-md disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                  onClick={handleNewCluster}
                  disabled={loading}
                >
                  {loading ? <><Loader2 size={14} className="animate-spin" /> Deploying…</> : 'Deploy'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
