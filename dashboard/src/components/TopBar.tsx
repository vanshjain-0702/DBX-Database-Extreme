import { useState } from 'react';
import { Bell, ChevronRight, LogOut, Settings, Command, ExternalLink, X } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

interface TopBarProps {
  crumbs: { label: string; href?: string }[];
  clusterId?: string;
}

export default function TopBar({ crumbs, clusterId }: TopBarProps) {
  const [showMenu, setShowMenu] = useState(false);
  const [showNotifPanel, setShowNotifPanel] = useState(false);
  const navigate = useNavigate();

  const handleLogout = () => {
    localStorage.removeItem('dbx_token');
    navigate('/login');
  };

  return (
    <>
      {/* Notifications side panel */}
      {showNotifPanel && (
        <div
          className="fixed inset-0 bg-black/30 backdrop-blur-sm z-40"
          onClick={() => setShowNotifPanel(false)}
        >
          <div
            className="absolute right-0 top-0 h-full w-80 bg-white border-l border-gray-200 shadow-2xl flex flex-col"
            onClick={e => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-100">
              <h3 className="font-bold text-slate-900 text-sm">Notifications</h3>
              <button
                onClick={() => setShowNotifPanel(false)}
                className="text-gray-400 hover:text-slate-900 transition-colors p-1 rounded-lg hover:bg-gray-100"
              >
                <X size={16} />
              </button>
            </div>
            <div className="flex-1 flex flex-col items-center justify-center gap-2 text-gray-400">
              <Bell size={28} className="opacity-30" />
              <span className="text-sm">No new notifications</span>
            </div>
          </div>
        </div>
      )}

      <header className="flex-shrink-0 h-[56px] bg-white border-b border-gray-200 flex items-center justify-between px-5 z-10 relative shadow-sm">
        {/* Breadcrumbs */}
        <nav className="flex items-center gap-1.5 text-sm font-medium">
          {crumbs.map((crumb, i) => (
            <span key={i} className="flex items-center gap-1.5">
              {i > 0 && <ChevronRight size={14} className="text-gray-300" />}
              {crumb.href ? (
                <button
                  className="text-gray-500 hover:text-slate-900 cursor-pointer transition-colors px-2 py-1 rounded-lg hover:bg-gray-100"
                  onClick={() => navigate(crumb.href!)}
                >
                  {crumb.label}
                </button>
              ) : (
                <span
                  className={`px-2 py-1 rounded-lg text-sm ${
                    i === crumbs.length - 1
                      ? 'text-slate-900 font-semibold bg-gray-100'
                      : 'text-gray-500'
                  }`}
                >
                  {crumb.label}
                </span>
              )}
            </span>
          ))}
        </nav>

        {/* Right actions */}
        <div className="flex items-center gap-2">
          {/* Environment Badge */}
          <div className="hidden md:flex items-center gap-1.5 px-2.5 py-1 rounded-full border border-red-200 bg-red-50 text-[11px] font-bold text-red-600 uppercase tracking-wider">
            Production
          </div>

          {/* Support Link */}
          <button
            onClick={() => window.open('https://github.com', '_blank')}
            className="hidden md:flex items-center gap-1.5 text-sm font-medium text-gray-500 hover:text-slate-900 px-3 py-1.5 rounded-lg hover:bg-gray-100 transition-colors"
          >
            Support <ExternalLink size={13} />
          </button>

          <div className="w-px h-4 bg-gray-200 mx-1 hidden md:block" />

          {/* Notifications bell */}
          <button
            onClick={() => setShowNotifPanel(true)}
            className="relative p-2 rounded-lg hover:bg-gray-100 text-gray-500 hover:text-slate-900 transition-colors"
            title="Notifications"
          >
            <Bell size={16} />
            <span className="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-red-500 border-2 border-white" />
          </button>

          {/* Settings shortcut */}
          <button
            onClick={() => navigate('/settings')}
            className="p-2 rounded-lg hover:bg-gray-100 text-gray-500 hover:text-slate-900 transition-colors"
            title="Settings"
          >
            <Settings size={16} />
          </button>

          {/* User avatar + dropdown */}
          <div className="relative ml-1">
            <button
              onClick={() => setShowMenu(!showMenu)}
              className="w-8 h-8 rounded-full bg-gradient-to-br from-red-500 to-red-600 flex items-center justify-center font-bold text-white text-sm hover:ring-2 hover:ring-red-500/40 hover:ring-offset-1 transition-all shadow-sm"
            >
              A
            </button>

            {showMenu && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setShowMenu(false)} />
                <div className="absolute right-0 top-full mt-2 w-56 bg-white border border-gray-200 rounded-2xl shadow-xl overflow-hidden z-20">
                  <div className="px-4 py-3 border-b border-gray-100 bg-gray-50">
                    <div className="text-sm font-bold text-slate-900">Admin User</div>
                    <div className="text-xs text-gray-500 mt-0.5">admin@dbx.local</div>
                    {clusterId && (
                      <div className="text-[10px] text-red-600 mt-1.5 font-mono uppercase tracking-wider border border-red-200 bg-red-50 inline-block px-1.5 py-0.5 rounded">
                        {clusterId}
                      </div>
                    )}
                  </div>
                  <div className="p-1.5 space-y-0.5">
                    <button
                      onClick={() => { navigate('/settings'); setShowMenu(false); }}
                      className="w-full flex items-center gap-3 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-gray-100 hover:text-slate-900 rounded-xl transition-colors"
                    >
                      <Settings size={14} /> Account Settings
                    </button>
                    <button
                      onClick={() => { setShowMenu(false); }}
                      className="w-full flex items-center gap-3 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-gray-100 hover:text-slate-900 rounded-xl transition-colors"
                    >
                      <Command size={14} /> Command Menu
                      <span className="ml-auto text-[10px] text-gray-400 border border-gray-200 rounded px-1 py-0.5 font-mono">⌘K</span>
                    </button>
                    <div className="border-t border-gray-100 my-1" />
                    <button
                      onClick={handleLogout}
                      className="w-full flex items-center gap-3 px-3 py-2 text-sm font-medium text-red-500 hover:bg-red-50 rounded-xl transition-colors"
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
    </>
  );
}
