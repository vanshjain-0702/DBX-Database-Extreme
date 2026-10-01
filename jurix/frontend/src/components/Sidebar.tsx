import { Link, useLocation } from 'react-router-dom';
import { Scale, CheckSquare, Download, Settings } from 'lucide-react';

const navItems = [
  { name: 'Matters', path: '/matters', icon: Scale },
  { name: 'Checklists', path: '/checklists', icon: CheckSquare },
  { name: 'Exports', path: '/exports', icon: Download },
];

export default function Sidebar() {
  const location = useLocation();
  const currentPath = location.pathname;

  return (
    <aside className="w-64 bg-black/40 backdrop-blur-xl border-r border-[#C4A574]/20 flex flex-col h-full text-white shadow-2xl relative z-20">
      <div className="p-6 flex items-center gap-3">
        <Scale className="text-[#C4A574]" size={28} />
        <div>
          <h1 className="font-serif tracking-widest font-bold text-lg leading-tight text-[#C4A574]">JURIX</h1>
          <div className="text-[10px] uppercase tracking-wider text-gray-400 font-semibold">Legal Audit</div>
        </div>
      </div>

      <nav className="flex-1 px-4 py-6 space-y-2">
        {navItems.map((item) => {
          const isActive = currentPath.startsWith(item.path);
          return (
            <Link
              key={item.name}
              to={item.path}
              className={`flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all ${
                isActive 
                  ? 'bg-[#C4A574]/20 text-[#C4A574] shadow-[inset_0_1px_1px_rgba(255,255,255,0.1)] border border-[#C4A574]/30' 
                  : 'text-gray-400 hover:bg-white/5 hover:text-white border border-transparent'
              }`}
            >
              <item.icon size={18} className={isActive ? 'text-[#C4A574]' : 'opacity-70'} />
              {item.name}
            </Link>
          );
        })}
      </nav>

      <div className="p-4 border-t border-[#C4A574]/20 bg-black/20">
        <Link
          to="/firm"
          className={`flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all ${
            currentPath.startsWith('/firm') 
              ? 'bg-[#C4A574]/20 text-[#C4A574] border border-[#C4A574]/30' 
              : 'text-gray-400 hover:bg-white/5 hover:text-white border border-transparent'
          }`}
        >
          <Settings size={18} className="opacity-70" />
          Firm Settings
        </Link>
        <div className="mt-6 px-4 pb-2">
          <div className="text-xs font-medium text-gray-300">Cross-tenant reads: <span className="font-mono text-[#C4A574] font-bold">0</span></div>
          <div className="text-[10px] text-gray-500 mt-1 uppercase tracking-wider">Sealed memory guaranteed</div>
        </div>
      </div>
    </aside>
  );
}
