import type { ReactNode } from 'react';
import { useState, useRef } from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import {
  Activity, Database, Settings, Terminal, Network,
  ChevronDown, ChevronRight, BarChart2, Cpu,
  Shield, LogOut, User, MonitorDot, Gauge, Globe, HardDrive,
  Layers, Menu, PanelLeft, Key
} from 'lucide-react';
import logo from '../assets/logo.jpg';
import { openCommandPalette } from '../api';
import { useTenant } from './TenantProvider';
import { StatusBadge } from './PageChrome';

interface SidebarProps {
  clusterId: string;
}

interface NavGroup {
  label: string;
  icon: ReactNode;
  items: { path: string; label: string; icon: ReactNode }[];
}

export default function Sidebar({ clusterId }: SidebarProps) {
  const [collapsed, setCollapsed] = useState(false);
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({
    product: true,
    server: true,
    config: true,
  });
  const [showUserMenu, setShowUserMenu] = useState(false);
  const navigate = useNavigate();
  const sidebarRef = useRef<HTMLElement>(null);
  const { tenant } = useTenant(clusterId || undefined);

  const groups: NavGroup[] = clusterId
    ? [
        {
          label: 'Tenant',
          icon: <MonitorDot size={13} />,
          items: [
            { path: `/cluster/${clusterId}/overview`, label: 'Overview', icon: <Activity size={15} /> },
            { path: `/cluster/${clusterId}/explorer`, label: 'Data Explorer', icon: <Database size={15} /> },
            { path: `/cluster/${clusterId}/vector`, label: 'Vector Playground', icon: <Network size={15} /> },
            { path: `/cluster/${clusterId}/keys`, label: 'Tenant keys', icon: <Key size={15} /> },
            { path: `/cluster/${clusterId}/terminal`, label: 'Console', icon: <Terminal size={15} /> },
          ],
        },
        {
          label: 'Runtime',
          icon: <Cpu size={13} />,
          items: [
            { path: `/cluster/${clusterId}/hardware`, label: 'Hardware', icon: <Cpu size={15} /> },
            { path: `/cluster/${clusterId}/storage`, label: 'Storage', icon: <HardDrive size={15} /> },
            { path: `/cluster/${clusterId}/network`, label: 'Network', icon: <Globe size={15} /> },
            { path: `/cluster/${clusterId}/hosting`, label: 'Hosting', icon: <Gauge size={15} /> },
          ],
        },
        {
          label: 'Control plane',
          icon: <Layers size={13} />,
          items: [
            { path: `/settings`, label: 'Settings', icon: <Settings size={15} /> },
            { path: `/settings/security`, label: 'Security', icon: <Shield size={15} /> },
            { path: `/settings/replication`, label: 'Replication', icon: <BarChart2 size={15} /> },
          ],
        },
      ]
    : [
        {
          label: 'Control plane',
          icon: <Layers size={13} />,
          items: [
            { path: `/`, label: 'Tenants', icon: <Database size={15} /> },
            { path: `/settings`, label: 'Settings', icon: <Settings size={15} /> },
            { path: `/settings/security`, label: 'Security', icon: <Shield size={15} /> },
            { path: `/settings/replication`, label: 'Replication', icon: <BarChart2 size={15} /> },
          ],
        },
      ];

  const groupKeys = clusterId ? ['product', 'server', 'config'] : ['config'];

  const toggleGroup = (key: string) => {
    setOpenGroups(prev => ({ ...prev, [key]: !prev[key] }));
  };

  const handleLogout = () => {
    localStorage.removeItem('dbx_token');
    navigate('/login');
  };

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `flex items-center gap-3 px-3 py-[7px] text-[13px] font-medium transition-colors duration-150 border-l-2 ${
      collapsed ? 'justify-center border-l-0 px-0' : ''
    } ${
      isActive
        ? 'border-[var(--accent-primary)] text-[var(--accent-primary)] bg-[var(--accent-soft)]'
        : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] hover:border-[var(--border-highlight)]'
    }`;

  return (
    <>
      {!collapsed && (
        <div
          className="fixed inset-0 bg-black/40 z-10 lg:hidden"
          onClick={() => setCollapsed(true)}
        />
      )}

      <aside
        ref={sidebarRef}
        className={`
          flex flex-col z-20 flex-shrink-0 h-full overflow-hidden
          bg-[var(--bg-sidebar)] border-r border-[var(--border-color)]
          transition-[width] duration-150 ease-linear
          ${collapsed ? 'w-[56px]' : 'w-[240px]'}
        `}
      >
        <div className={`flex items-center border-b border-[var(--border-color)] h-12 ${collapsed ? 'justify-center px-1' : 'justify-between px-3'}`}>
          {!collapsed && (
            <button
              type="button"
              className="flex items-center gap-2.5 min-w-0"
              onClick={() => navigate('/')}
            >
              <div className="w-7 h-7 rounded overflow-hidden border border-[var(--border-color)] flex-shrink-0">
                <img src={logo} alt="" className="w-full h-full object-cover" />
              </div>
              <div className="text-left min-w-0">
                <div className="font-semibold text-[13px] tracking-tight leading-none text-[var(--text-primary)]">DBX</div>
                <div className="text-[10px] text-[var(--accent-primary)] font-semibold tracking-[0.14em] uppercase mt-0.5">Control plane</div>
              </div>
            </button>
          )}
          {collapsed && (
            <button
              type="button"
              className="w-7 h-7 rounded overflow-hidden border border-[var(--border-color)]"
              onClick={() => navigate('/')}
            >
              <img src={logo} alt="DBX" className="w-full h-full object-cover" />
            </button>
          )}
          <button
            type="button"
            onClick={() => setCollapsed(v => !v)}
            className="text-[var(--text-muted)] hover:text-[var(--text-primary)] p-1.5 rounded hover:bg-[var(--bg-tertiary)] flex-shrink-0"
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {collapsed ? <Menu size={15} /> : <PanelLeft size={15} />}
          </button>
        </div>

        <nav className="flex-1 overflow-y-auto py-3 space-y-5">
          {groups.map((group, gi) => {
            const key = groupKeys[gi];
            const isOpen = openGroups[key];
            return (
              <div key={key}>
                <button
                  type="button"
                  onClick={() => toggleGroup(key)}
                  className={`
                    w-full flex items-center gap-2 px-3 mb-1 text-[11px] font-semibold uppercase tracking-[0.08em]
                    text-[var(--text-muted)] hover:text-[var(--text-secondary)]
                    ${collapsed ? 'justify-center px-0' : 'justify-between'}
                  `}
                  title={collapsed ? group.label : undefined}
                >
                  <span className="flex items-center gap-2">
                    {group.icon}
                    {!collapsed && group.label}
                  </span>
                  {!collapsed && (isOpen
                    ? <ChevronDown size={12} />
                    : <ChevronRight size={12} />
                  )}
                </button>

                {(!collapsed && isOpen) && (
                  <div>
                    {group.items.map(item => (
                      <NavLink key={item.path} to={item.path} className={linkClass}>
                        {item.icon}
                        {item.label}
                      </NavLink>
                    ))}
                  </div>
                )}

                {collapsed && (
                  <div className="mt-1 space-y-0.5">
                    {group.items.map(item => (
                      <NavLink
                        key={item.path}
                        to={item.path}
                        title={item.label}
                        className={linkClass}
                      >
                        {item.icon}
                      </NavLink>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </nav>

        <div className="border-t border-[var(--border-color)] p-2">
          {!collapsed && clusterId && tenant && (
            <div className="px-2 py-2 mb-1 flex items-center justify-between gap-2">
              <span className="text-[11px] font-mono text-[var(--text-secondary)] truncate" title={clusterId}>
                {clusterId}
              </span>
              <StatusBadge tenant={tenant} />
            </div>
          )}

          <div className="relative">
            <button
              type="button"
              onClick={() => setShowUserMenu(!showUserMenu)}
              className="w-full flex items-center gap-2.5 p-2 rounded hover:bg-[var(--bg-tertiary)]"
            >
              <div className="w-7 h-7 rounded-full bg-zinc-800 dark:bg-zinc-200 text-white dark:text-zinc-900 flex items-center justify-center font-semibold text-xs flex-shrink-0">
                A
              </div>
              {!collapsed && (
                <>
                  <div className="text-left flex-1 min-w-0">
                    <div className="text-[13px] font-medium text-[var(--text-primary)] leading-tight">Admin</div>
                    <div className="text-[11px] text-[var(--text-muted)] font-mono truncate">operator</div>
                  </div>
                  <ChevronDown size={12} className="text-[var(--text-muted)]" />
                </>
              )}
            </button>

            {showUserMenu && !collapsed && (
              <div className="absolute bottom-full left-0 right-0 mb-1 bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md overflow-hidden z-30">
                <div className="px-3 py-2.5 border-b border-[var(--border-color)]">
                  <div className="text-[13px] font-semibold text-[var(--text-primary)]">Admin</div>
                  <div className="text-[11px] text-[var(--text-muted)]">control plane</div>
                </div>
                <div className="p-1">
                  <button
                    type="button"
                    className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded text-[13px] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
                    onClick={() => { navigate('/settings'); setShowUserMenu(false); }}
                  >
                    <User size={13} /> Settings
                  </button>
                  <button
                    type="button"
                    className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded text-[13px] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)]"
                    onClick={() => { openCommandPalette(); setShowUserMenu(false); }}
                  >
                    Command palette
                    <kbd className="ml-auto font-mono text-[10px] border border-[var(--border-color)] px-1 rounded">⌘K</kbd>
                  </button>
                  <button
                    type="button"
                    onClick={handleLogout}
                    className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded text-[13px] text-[var(--error)] hover:bg-rose-50 dark:hover:bg-rose-950/40"
                  >
                    <LogOut size={13} /> Sign out
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </aside>
    </>
  );
}
