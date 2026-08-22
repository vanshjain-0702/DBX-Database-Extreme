import { useState, useRef } from 'react';
import gsap from 'gsap';
import { Flip } from 'gsap/Flip';

gsap.registerPlugin(Flip);
import { NavLink, useNavigate } from 'react-router-dom';
import {
  Activity, Database, Settings, Terminal, Network,
  ChevronDown, ChevronRight, BarChart2, Cpu, Server,
  Shield, Bell, Search, LogOut, User,
  MonitorDot, Gauge, Globe, HardDrive,
  Layers, X, Menu, Command
} from 'lucide-react';
import logo from '../assets/logo.jpg';

interface SidebarProps {
  clusterId: string;
}

interface NavGroup {
  label: string;
  icon: React.ReactNode;
  items: { path: string; label: string; icon: React.ReactNode }[];
}

export default function Sidebar({ clusterId }: SidebarProps) {
  const [collapsed, setCollapsed] = useState(false);
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({
    product: true,
    server: true,
    config: true,
  });
  const [showUserMenu, setShowUserMenu] = useState(false);
  const [showNotifications, setShowNotifications] = useState(false);
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [cmdSearch, setCmdSearch] = useState('');
  const navigate = useNavigate();
  const sidebarRef = useRef<HTMLElement>(null);

  const toggleCollapsed = () => {
    if (!sidebarRef.current) return;
    const state = Flip.getState(sidebarRef.current.querySelectorAll('*'), { props: "width,height,padding,opacity" });
    setCollapsed(prev => !prev);
    
    // We must wait for React to render the new state, so we use requestAnimationFrame
    requestAnimationFrame(() => {
      Flip.from(state, {
        duration: 0.5,
        ease: 'power3.inOut',
        absolute: true,
        nested: true,
      });
    });
  };

  const groups: NavGroup[] = [
    {
      label: 'Product Monitoring',
      icon: <MonitorDot size={14} />,
      items: [
        { path: `/cluster/${clusterId}/hosting`, label: 'Hosting Performance', icon: <Gauge size={15} /> },
        { path: `/cluster/${clusterId}/overview`, label: 'Metrics Overview', icon: <Activity size={15} /> },
        { path: `/cluster/${clusterId}/explorer`, label: 'Data Explorer', icon: <Database size={15} /> },
        { path: `/cluster/${clusterId}/vector`, label: 'Vector Playground', icon: <Network size={15} /> },
      ],
    },
    {
      label: 'Server Analytics',
      icon: <Server size={14} />,
      items: [
        { path: `/cluster/${clusterId}/terminal`, label: 'Interactive Console', icon: <Terminal size={15} /> },
        { path: `/cluster/${clusterId}/hardware`, label: 'Hardware & Load', icon: <Cpu size={15} /> },
        { path: `/cluster/${clusterId}/storage`, label: 'Disk & Storage', icon: <HardDrive size={15} /> },
        { path: `/cluster/${clusterId}/network`, label: 'Network I/O', icon: <Globe size={15} /> },
      ],
    },
    {
      label: 'Configuration',
      icon: <Layers size={14} />,
      items: [
        { path: `/settings`, label: 'General Settings', icon: <Settings size={15} /> },
        { path: `/settings/security`, label: 'Security & TLS', icon: <Shield size={15} /> },
        { path: `/settings/replication`, label: 'Replication (Raft)', icon: <BarChart2 size={15} /> },
      ],
    },
  ];

  const groupKeys = ['product', 'server', 'config'];

  const toggleGroup = (key: string) => {
    setOpenGroups(prev => ({ ...prev, [key]: !prev[key] }));
  };

  const handleLogout = () => {
    localStorage.removeItem('dbx_token');
    navigate('/login');
  };

  // All navigable items for command palette
  const allItems = groups.flatMap(g => g.items);
  const filteredCmdItems = allItems.filter(item =>
    item.label.toLowerCase().includes(cmdSearch.toLowerCase())
  );

  return (
    <>
      {/* Command Palette Modal */}
      {showCommandPalette && (
        <div
          className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-start justify-center pt-24 px-4"
          onClick={() => { setShowCommandPalette(false); setCmdSearch(''); }}
        >
          <div
            className="w-full max-w-lg bg-white border border-gray-200 rounded-2xl shadow-2xl overflow-hidden"
            onClick={e => e.stopPropagation()}
          >
            <div className="flex items-center gap-3 px-4 py-3.5 border-b border-gray-100">
              <Search size={16} className="text-gray-400 flex-shrink-0" />
              <input
                autoFocus
                type="text"
                placeholder="Search pages and actions…"
                value={cmdSearch}
                onChange={e => setCmdSearch(e.target.value)}
                className="flex-1 text-sm text-slate-900 placeholder-gray-400 outline-none bg-transparent"
              />
              <button
                onClick={() => { setShowCommandPalette(false); setCmdSearch(''); }}
                className="text-gray-400 hover:text-slate-900 transition-colors p-1"
              >
                <X size={16} />
              </button>
            </div>
            <div className="py-2 max-h-80 overflow-y-auto">
              {filteredCmdItems.length === 0 ? (
                <div className="px-4 py-8 text-center text-sm text-gray-400">No results for "{cmdSearch}"</div>
              ) : (
                filteredCmdItems.map(item => (
                  <button
                    key={item.path}
                    onClick={() => { navigate(item.path); setShowCommandPalette(false); setCmdSearch(''); }}
                    className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-slate-700 hover:bg-gray-50 hover:text-slate-900 transition-colors text-left"
                  >
                    <span className="text-gray-400">{item.icon}</span>
                    {item.label}
                  </button>
                ))
              )}
            </div>
            <div className="px-4 py-2.5 border-t border-gray-100 flex items-center gap-4 text-xs text-gray-400">
              <span><kbd className="font-mono bg-gray-100 px-1.5 py-0.5 rounded">↑↓</kbd> navigate</span>
              <span><kbd className="font-mono bg-gray-100 px-1.5 py-0.5 rounded">↵</kbd> select</span>
              <span><kbd className="font-mono bg-gray-100 px-1.5 py-0.5 rounded">Esc</kbd> close</span>
            </div>
          </div>
        </div>
      )}

      {/* Notifications panel */}
      {showNotifications && (
        <div
          className="fixed inset-0 bg-black/30 backdrop-blur-sm z-40"
          onClick={() => setShowNotifications(false)}
        >
          <div
            className="absolute left-[280px] top-0 h-full w-80 bg-white border-r border-gray-200 shadow-2xl flex flex-col"
            onClick={e => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-100">
              <h3 className="font-bold text-slate-900 text-sm">Notifications</h3>
              <button onClick={() => setShowNotifications(false)} className="text-gray-400 hover:text-slate-900 transition-colors">
                <X size={16} />
              </button>
            </div>
            <div className="flex-1 flex items-center justify-center text-sm text-gray-400">
              No new notifications
            </div>
          </div>
        </div>
      )}

      {/* Mobile overlay */}
      {!collapsed && (
        <div
          className="fixed inset-0 bg-black/40 z-10 lg:hidden backdrop-blur-sm"
          onClick={() => setCollapsed(true)}
        />
      )}

      <aside
        ref={sidebarRef}
        className={`
          flex flex-col bg-white border-r border-gray-200
          z-20 flex-shrink-0 h-full overflow-hidden
          ${collapsed ? 'w-[68px]' : 'w-[272px]'}
        `}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-4 border-b border-gray-100">
          {!collapsed && (
            <div className="flex items-center gap-3 cursor-pointer" onClick={() => navigate('/')}>
              <div className="w-8 h-8 rounded-lg overflow-hidden border border-gray-200 shadow-sm flex-shrink-0">
                <img src={logo} alt="DBX Logo" className="w-full h-full object-cover" />
              </div>
              <div>
                <div className="font-extrabold text-lg tracking-tight leading-none mb-0.5" style={{ background: 'linear-gradient(90deg, #111827 0%, #3b82f6 100%)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>DBX Cloud</div>
                <div className="text-[10px] text-red-600 font-bold tracking-widest uppercase">Enterprise</div>
              </div>
            </div>
          )}
          {collapsed && (
            <div
              className="w-8 h-8 rounded-lg overflow-hidden border border-gray-200 shadow-sm mx-auto cursor-pointer"
              onClick={() => navigate('/')}
            >
              <img src={logo} alt="DBX Logo" className="w-full h-full object-cover" />
            </div>
          )}
          <button
            onClick={toggleCollapsed}
            className="text-gray-400 hover:text-slate-900 transition-colors p-1.5 rounded-lg hover:bg-gray-100 flex-shrink-0 ml-auto"
          >
            {collapsed ? <Menu size={16} /> : <X size={16} />}
          </button>
        </div>

        {/* Search / Command Palette Trigger */}
        {!collapsed && (
          <div className="px-4 py-3 border-b border-gray-100">
            <button
              onClick={() => setShowCommandPalette(true)}
              className="w-full flex items-center justify-between gap-2 bg-gray-50 border border-gray-200 rounded-xl px-3 py-2 hover:border-gray-300 hover:bg-gray-100 transition-all group"
            >
              <div className="flex items-center gap-2 text-gray-400 group-hover:text-gray-600">
                <Search size={14} />
                <span className="text-sm">Search DBX…</span>
              </div>
              <div className="flex items-center gap-1">
                <kbd className="bg-white border border-gray-200 rounded px-1.5 py-0.5 text-[10px] font-mono text-gray-400">⌘</kbd>
                <kbd className="bg-white border border-gray-200 rounded px-1.5 py-0.5 text-[10px] font-mono text-gray-400">K</kbd>
              </div>
            </button>
          </div>
        )}

        {/* Nav Groups */}
        <nav className="flex-1 overflow-y-auto px-4 py-5 space-y-8">
          {groups.map((group, gi) => {
            const key = groupKeys[gi];
            const isOpen = openGroups[key];
            return (
              <div key={key}>
                <button
                  onClick={() => toggleGroup(key)}
                  className={`
                    w-full flex items-center gap-2 px-1 mb-2 text-[12px] font-bold uppercase tracking-[0.2em]
                    transition-colors text-slate-400 hover:text-slate-600
                    ${collapsed ? 'justify-center' : 'justify-between'}
                  `}
                  title={collapsed ? group.label : undefined}
                >
                  <span className="flex items-center gap-3">
                    {group.icon}
                    {!collapsed && group.label}
                  </span>
                  {!collapsed && (isOpen
                    ? <ChevronDown size={14} className="text-gray-400" />
                    : <ChevronRight size={14} className="text-gray-400" />
                  )}
                </button>

                {!collapsed && isOpen && (
                  <div className="space-y-2 mt-2">
                    {group.items.map(item => (
                      <NavLink
                        key={item.label + item.path}
                        to={item.path}
                        className={({ isActive }) => `
                          flex items-center gap-4 px-4 py-3 rounded-2xl text-[15px] font-medium transition-all duration-300 group relative
                          ${isActive
                            ? 'bg-gradient-to-r from-slate-900 to-slate-800 text-white shadow-lg shadow-slate-900/25 translate-x-1 border border-slate-700/50'
                            : 'text-slate-500 hover:text-slate-900 hover:bg-slate-50 hover:shadow-sm hover:translate-x-1 border border-transparent'
                          }
                        `}
                      >
                        {({ isActive }) => (
                          <>
                            <span className={`transition-transform duration-300 ${isActive ? 'text-white scale-110' : 'text-slate-400 group-hover:scale-110 group-hover:text-slate-700'}`}>
                              {item.icon}
                            </span>
                            {item.label}
                          </>
                        )}
                      </NavLink>
                    ))}
                  </div>
                )}

                {/* Collapsed: just icons */}
                {collapsed && (
                  <div className="space-y-2 mt-2">
                    {group.items.map(item => (
                      <NavLink
                        key={item.label}
                        to={item.path}
                        title={item.label}
                        className={({ isActive }) => `
                          flex items-center justify-center p-3.5 rounded-2xl transition-all duration-300 group relative
                          ${isActive 
                            ? 'bg-slate-900 text-white shadow-md shadow-slate-900/20' 
                            : 'text-slate-400 hover:text-slate-900 hover:bg-slate-100/80'}
                        `}
                      >
                        {() => (
                          <>
                            <span className="transition-transform duration-300 group-hover:scale-110">
                              {item.icon}
                            </span>
                          </>
                        )}
                      </NavLink>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </nav>

        {/* Footer / User Menu */}
        <div className="border-t border-gray-100 p-3 bg-gray-50 space-y-1">
          {/* Quick action buttons */}
          {!collapsed && (
            <div className="flex items-center gap-1 mb-2">
              <button
                onClick={() => setShowNotifications(true)}
                title="Notifications"
                className="flex-1 flex items-center justify-center gap-1.5 py-2 rounded-xl text-gray-500 hover:text-slate-900 hover:bg-white hover:border hover:border-gray-200 transition-all text-xs font-medium"
              >
                <Bell size={14} />
                Alerts
              </button>
              <button
                onClick={() => setShowCommandPalette(true)}
                title="Command Menu"
                className="flex-1 flex items-center justify-center gap-1.5 py-2 rounded-xl text-gray-500 hover:text-slate-900 hover:bg-white hover:border hover:border-gray-200 transition-all text-xs font-medium"
              >
                <Command size={14} />
                ⌘K
              </button>
            </div>
          )}

          {/* User button */}
          <div className="relative">
            <button
              onClick={() => setShowUserMenu(!showUserMenu)}
              className="w-full flex items-center gap-3 p-2.5 rounded-xl hover:bg-white hover:border hover:border-gray-200 transition-all group border border-transparent"
            >
              <div className="w-8 h-8 rounded-full bg-gradient-to-br from-red-500 to-red-600 flex items-center justify-center font-bold text-white text-sm flex-shrink-0 shadow-sm">
                A
              </div>
              {!collapsed && (
                <>
                  <div className="text-left flex-1 min-w-0">
                    <div className="text-sm font-semibold text-slate-700 leading-tight">Admin</div>
                    <div className="text-[11px] text-gray-400 font-mono truncate mt-0.5">{clusterId}</div>
                  </div>
                  <ChevronDown size={13} className="text-gray-400" />
                </>
              )}
            </button>

            {showUserMenu && !collapsed && (
              <div className="absolute bottom-full left-0 right-0 mb-2 bg-white border border-gray-200 rounded-xl shadow-xl overflow-hidden">
                <div className="px-4 py-3 border-b border-gray-100 bg-gray-50">
                  <div className="text-sm font-bold text-slate-900">Admin User</div>
                  <div className="text-xs text-gray-500 mt-0.5">admin@dbx.local</div>
                </div>
                <div className="p-1.5 space-y-0.5">
                  <button
                    className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-slate-700 hover:bg-gray-100 hover:text-slate-900 transition-colors"
                    onClick={() => { navigate('/settings'); setShowUserMenu(false); }}
                  >
                    <User size={14} /> Profile & Settings
                  </button>
                  <div className="border-t border-gray-100 my-1" />
                  <button
                    onClick={handleLogout}
                    className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-red-500 hover:bg-red-50 transition-colors"
                  >
                    <LogOut size={14} /> Sign Out
                  </button>
                </div>
              </div>
            )}
          </div>

          {/* Status indicator */}
          {!collapsed && (
            <div className="px-2 py-1.5 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <div className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
                </div>
                <span className="text-[11px] text-gray-500 font-medium">Orchestrator Connected</span>
              </div>
              <span className="text-[10px] text-gray-400 font-mono">v2.0</span>
            </div>
          )}
        </div>
      </aside>
    </>
  );
}
