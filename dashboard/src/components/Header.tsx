import { useLocation, useParams } from 'react-router-dom';
import { Search, Bell, ChevronRight } from 'lucide-react';

export default function Header() {
  const location = useLocation();
  const { id } = useParams();

  // Generate breadcrumbs from pathname
  const pathnames = location.pathname.split('/').filter(x => x);
  
  // Format breadcrumbs nicely
  const getBreadcrumbLabel = (path: string) => {
    if (path === 'cluster') return 'Cluster';
    if (path === id) return id; // e.g. bench-tenant
    if (path === 'settings') return 'Settings';
    // Capitalize first letter of normal routes
    return path.charAt(0).toUpperCase() + path.slice(1);
  };

  return (
    <header className="h-16 bg-white border-b border-gray-200 flex items-center justify-between px-6 flex-shrink-0 z-10 sticky top-0">
      {/* Breadcrumbs */}
      <div className="flex items-center text-sm font-medium text-gray-500">
        {pathnames.map((value, index) => {
          const isLast = index === pathnames.length - 1;
          return (
            <div key={value} className="flex items-center">
              <span className={`transition-colors ${isLast ? 'text-slate-900 font-bold tracking-tight' : 'hover:text-slate-700 cursor-pointer'}`}>
                {getBreadcrumbLabel(value)}
              </span>
              {!isLast && <ChevronRight size={14} className="mx-2 text-gray-300" />}
            </div>
          );
        })}
      </div>

      {/* Right side actions */}
      <div className="flex items-center gap-4">
        {/* Mock Search / Command Trigger */}
        <button 
          onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))}
          className="flex items-center gap-2 bg-gray-50 border border-gray-200 rounded-lg px-3 py-1.5 hover:border-gray-300 hover:bg-gray-100 transition-all group dark:bg-slate-800 dark:border-slate-700 dark:hover:border-slate-600 dark:hover:bg-slate-700"
        >
          <Search size={14} className="text-gray-400 group-hover:text-gray-600 dark:group-hover:text-slate-300" />
          <span className="text-xs text-gray-400 font-medium">Search / CMD+K</span>
        </button>

        <div className="h-4 w-px bg-gray-200 mx-1"></div>

        <button className="relative text-gray-400 hover:text-slate-900 transition-colors">
          <Bell size={18} />
          <span className="absolute -top-1 -right-1 w-2 h-2 bg-red-500 rounded-full border border-white"></span>
        </button>

        <button className="flex items-center justify-center w-8 h-8 rounded-full bg-gradient-to-br from-red-500 to-red-600 text-white shadow-sm ring-2 ring-white hover:shadow-md transition-all">
          <span className="text-xs font-bold">A</span>
        </button>
      </div>
    </header>
  );
}
