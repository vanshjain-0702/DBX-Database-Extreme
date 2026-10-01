import { Search, Command, ShieldCheck, LogOut } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

export default function Header() {
  const navigate = useNavigate();

  const handleLogout = () => {
    localStorage.removeItem('jurix_token');
    navigate('/login');
  };

  return (
    <header className="h-16 bg-black/30 backdrop-blur-md border-b border-[#C4A574]/20 flex items-center justify-between px-6 shrink-0 text-white relative z-20">
      <div className="flex items-center gap-4 flex-1">
        <div className="relative w-96 group">
          <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <Search size={16} className="text-gray-400 group-focus-within:text-[#C4A574] transition-colors" />
          </div>
          <input
            type="text"
            className="block w-full pl-10 pr-3 py-2 border border-[#C4A574]/20 rounded-lg leading-5 bg-black/40 placeholder-gray-500 focus:outline-none focus:bg-black/60 focus:ring-1 focus:ring-[#C4A574] focus:border-[#C4A574] sm:text-sm transition-all text-white"
            placeholder="Search this client's documents..."
          />
          <div className="absolute inset-y-0 right-0 pr-2 flex items-center">
            <kbd className="inline-flex items-center border border-gray-600 rounded px-2 text-xs font-sans font-medium text-gray-500 bg-black/50">⌘K</kbd>
          </div>
        </div>
      </div>
      
      <div className="flex items-center gap-6">
        <div className="flex items-center gap-2 px-3 py-1.5 bg-[#C4A574]/10 border border-[#C4A574]/20 text-[#C4A574] rounded-full text-xs font-medium shadow-[0_0_10px_rgba(196,165,116,0.1)]">
          <div className="w-2 h-2 rounded-full bg-green-400 shadow-[0_0_5px_#4ade80] animate-pulse"></div>
          DBX Connected
        </div>
        
        <div className="flex items-center gap-4 border-l border-[#C4A574]/20 pl-6">
          <div className="flex flex-col text-right">
            <span className="text-sm font-medium text-gray-200">Reviewer</span>
            <span className="text-xs text-[#C4A574]">Firm Bench</span>
          </div>
          <div className="w-9 h-9 rounded-full bg-black/50 border border-[#C4A574]/40 flex items-center justify-center text-[#C4A574] font-serif font-bold cursor-pointer hover:bg-[#C4A574]/20 transition-all shadow-[0_0_15px_rgba(196,165,116,0.15)]">
            RV
          </div>
          <button 
            onClick={handleLogout}
            className="text-gray-400 hover:text-red-400 hover:bg-red-400/10 p-1.5 rounded-lg transition-colors ml-1"
            title="Sign out"
          >
            <LogOut size={18} />
          </button>
        </div>
      </div>
    </header>
  );
}
