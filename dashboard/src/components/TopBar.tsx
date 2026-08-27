import { useState } from 'react';
import { ChevronRight, LogOut, Settings } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { openCommandPalette } from '../api';

interface TopBarProps {
  crumbs: { label: string; href?: string }[];
  clusterId?: string;
}

/** Unused in routing today; kept graphite-aligned if a page opts in. */
export default function TopBar({ crumbs, clusterId }: TopBarProps) {
  const [showMenu, setShowMenu] = useState(false);
  const navigate = useNavigate();

  const handleLogout = () => {
    localStorage.removeItem('dbx_token');
    navigate('/login');
  };

  return (
    <header className="flex-shrink-0 h-12 bg-[var(--bg-panel)] border-b border-[var(--border-color)] flex items-center justify-between px-4 z-10">
      <nav className="flex items-center gap-1 text-[13px] font-medium">
        {crumbs.map((crumb, i) => (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <ChevronRight size={13} className="text-[var(--border-highlight)]" />}
            {crumb.href ? (
              <button
                type="button"
                className="text-[var(--text-muted)] hover:text-[var(--text-primary)] px-1.5 py-0.5 rounded"
                onClick={() => navigate(crumb.href!)}
              >
                {crumb.label}
              </button>
            ) : (
              <span className={i === crumbs.length - 1 ? 'text-[var(--text-primary)] font-semibold px-1.5' : 'text-[var(--text-muted)] px-1.5'}>
                {crumb.label}
              </span>
            )}
          </span>
        ))}
      </nav>

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={() => openCommandPalette()}
          className="h-7 px-2 text-[12px] text-[var(--text-muted)] border border-[var(--border-color)] rounded hover:text-[var(--text-primary)]"
        >
          ⌘K
        </button>
        <button
          type="button"
          onClick={() => navigate('/settings')}
          className="h-7 w-7 flex items-center justify-center rounded text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)]"
          title="Settings"
        >
          <Settings size={15} />
        </button>
        <div className="relative">
          <button
            type="button"
            onClick={() => setShowMenu(!showMenu)}
            className="w-7 h-7 rounded-full bg-zinc-800 dark:bg-zinc-200 text-white dark:text-zinc-900 flex items-center justify-center font-semibold text-[11px]"
          >
            A
          </button>
          {showMenu && (
            <>
              <div className="fixed inset-0 z-10" onClick={() => setShowMenu(false)} />
              <div className="absolute right-0 top-full mt-1 w-52 bg-[var(--bg-panel)] border border-[var(--border-color)] rounded-md z-20 overflow-hidden">
                {clusterId && (
                  <div className="px-3 py-2 border-b border-[var(--border-color)] font-mono text-[11px] text-[var(--text-muted)]">{clusterId}</div>
                )}
                <button
                  type="button"
                  onClick={handleLogout}
                  className="w-full flex items-center gap-2 px-3 py-2 text-[13px] text-[var(--error)] hover:bg-[var(--bg-tertiary)]"
                >
                  <LogOut size={13} /> Sign out
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
