import { useState, useEffect } from 'react';
import { useParams, Link, Outlet, useLocation } from 'react-router-dom';
import { ArrowLeft, Loader2, LayoutDashboard, CheckSquare, FileText, MessageSquare } from 'lucide-react';

export default function MatterLayout() {
  const { id } = useParams();
  const location = useLocation();
  const [matter, setMatter] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function loadMatter() {
      try {
        const token = localStorage.getItem('jurix_token');
        const res = await fetch(`/api/matters/${id}`, {
          headers: { Authorization: `Bearer ${token}` }
        });
        if (res.ok) {
          const data = await res.json();
          setMatter(data);
        }
      } finally {
        setLoading(false);
      }
    }
    loadMatter();
  }, [id]);

  if (loading) return <div className="p-20 flex justify-center"><Loader2 className="animate-spin text-[#C4A574]" size={40} /></div>;
  if (!matter) return <div className="p-8 text-white font-serif text-2xl text-center">Matter not found.</div>;

  const tabs = [
    { name: 'Dashboard', path: 'dashboard', icon: LayoutDashboard },
    { name: 'Findings', path: 'findings', icon: CheckSquare },
    { name: 'Documents', path: 'documents', icon: FileText },
    { name: 'Counsel AI', path: 'counsel', icon: MessageSquare },
  ];

  return (
    <div className="space-y-6 animate-in fade-in duration-500 h-full flex flex-col">
      <Link to="/matters" className="flex items-center gap-2 text-sm text-gray-400 hover:text-[#C4A574] transition-colors w-fit">
        <ArrowLeft size={16} /> Back to Matters
      </Link>
      
      <div className="flex justify-between items-end border-b border-[#C4A574]/30 pb-6 shrink-0">
        <div>
          <div className="text-xs font-mono bg-black/50 border border-gray-700 text-gray-400 px-3 py-1.5 rounded inline-block mb-3 shadow-inner">
            Tenant: {matter.tenant_id}
          </div>
          <h1 className="text-4xl font-serif text-white mb-2 drop-shadow-md">{matter.matter || 'Matter'}</h1>
          <p className="text-[#C4A574] font-medium">Reviewer: {matter.reviewer_name}</p>
        </div>
      </div>

      <div className="flex gap-2 border-b border-white/10 pb-4 shrink-0">
        {tabs.map(tab => {
          const isActive = location.pathname.includes(tab.path);
          const Icon = tab.icon;
          return (
            <Link 
              key={tab.path}
              to={tab.path}
              className={`flex items-center gap-2 px-5 py-2.5 rounded-xl font-medium transition-all ${isActive ? 'bg-[#C4A574]/10 text-[#C4A574] border border-[#C4A574]/30' : 'text-gray-400 hover:bg-white/5 hover:text-white'}`}
            >
              <Icon size={18} />
              {tab.name}
            </Link>
          );
        })}
      </div>

      <div className="pt-4 flex-1 min-h-0">
        <Outlet context={{ matter }} />
      </div>
    </div>
  );
}
