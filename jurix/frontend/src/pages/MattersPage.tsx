import { useState, useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { FileText, Plus, FolderLock, ArrowRight, Loader2, X } from 'lucide-react';

export default function MattersPage() {
  const [matters, setMatters] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showNewMatter, setShowNewMatter] = useState(false);

  const [newClient, setNewClient] = useState('');
  const [newMatter, setNewMatterName] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [creating, setCreating] = useState(false);
  
  const navigate = useNavigate();

  async function fetchMatters() {
    try {
      const token = localStorage.getItem('jurix_token');
      const res = await fetch('/api/matters', {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!res.ok) throw new Error('Failed to load matters');
      const data = await res.json();
      setMatters(data.matters || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    fetchMatters();
  }, []);

  const handleCreateMatter = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newClient.trim() || !newMatter.trim()) return;
    
    setCreating(true);
    try {
      const token = localStorage.getItem('jurix_token');
      const res = await fetch('/api/matters', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}` 
        },
        body: JSON.stringify({
          name: newClient.trim(),
          matter: newMatter.trim(),
          description: newDesc.trim()
        })
      });

      if (!res.ok) {
        const d = await res.json();
        throw new Error(d.error || d.detail || 'Failed to create matter');
      }

      const created = await res.json();
      setShowNewMatter(false);
      setNewClient('');
      setNewMatterName('');
      setNewDesc('');
      
      // Navigate straight to the new matter
      if (created && created.id) {
        navigate(`/matters/${created.id}`);
      } else {
        fetchMatters();
      }
    } catch (err: any) {
      alert(err.message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-500 relative">
      <div className="flex justify-between items-end border-b border-[#C4A574]/30 pb-4">
        <div>
          <h1 className="text-4xl font-serif text-white mb-2 tracking-tight drop-shadow-md">Matters</h1>
          <p className="text-[#C4A574] font-medium tracking-wide">Sealed client memory spaces on DBX.</p>
        </div>
        <button 
          onClick={() => setShowNewMatter(true)}
          className="bg-[#C4A574] hover:bg-[#d8b884] text-[#0a0806] px-5 py-2.5 rounded-xl font-bold tracking-wide flex items-center gap-2 transition-all hover:scale-105 active:scale-95 shadow-[0_0_15px_rgba(196,165,116,0.3)]"
        >
          <Plus size={18} />
          New Matter
        </button>
      </div>

      {loading ? (
        <div className="flex justify-center items-center py-20 text-[#C4A574]">
          <Loader2 className="animate-spin" size={40} />
        </div>
      ) : error ? (
        <div className="bg-red-900/40 text-red-200 p-4 rounded-xl border border-red-500/50 backdrop-blur-md">
          {error}
        </div>
      ) : matters.length === 0 ? (
        <div className="text-center py-24 bg-black/40 rounded-2xl border border-dashed border-[#C4A574]/40 backdrop-blur-md">
          <FolderLock size={64} className="mx-auto text-[#C4A574] mb-6 opacity-60" />
          <h3 className="text-2xl font-serif text-white mb-3">No active matters</h3>
          <p className="text-gray-400 max-w-sm mx-auto">Create a new matter to provision a sealed DBX tenant for your client's documents.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {matters.map((matter) => (
            <Link key={matter.id} to={`/matters/${matter.id}`} className="block bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 p-6 shadow-xl hover:shadow-[0_10px_30px_rgba(196,165,116,0.15)] hover:border-[#C4A574]/60 transition-all duration-300 group cursor-pointer relative overflow-hidden">
              <div className="absolute top-0 left-0 w-1.5 h-full bg-[#C4A574] opacity-0 group-hover:opacity-100 transition-opacity duration-300 shadow-[0_0_10px_#C4A574]" />
              
              <div className="flex justify-between items-start mb-6">
                <div className="p-3 bg-gradient-to-br from-[#C4A574]/20 to-transparent rounded-xl text-[#C4A574] border border-[#C4A574]/30 group-hover:rotate-6 transition-transform">
                  <FolderLock size={24} />
                </div>
                <span className="text-[10px] font-mono bg-black/50 border border-gray-700 text-gray-400 px-2 py-1 rounded shadow-inner">
                  {matter.id.substring(0, 8)}
                </span>
              </div>
              
              <h3 className="text-xl font-serif font-bold text-white mb-2 group-hover:text-[#C4A574] transition-colors">{matter.matter || matter.name}</h3>
              <p className="text-sm text-gray-400 mb-8 line-clamp-2 leading-relaxed">{matter.description || `Client: ${matter.client}`}</p>
              
              <div className="flex justify-between items-center text-sm font-medium border-t border-[#C4A574]/20 pt-5 mt-auto">
                <span className="flex items-center gap-2 text-gray-400">
                  <FileText size={16} className="text-[#C4A574]" />
                  {matter.doc_count || 0} docs
                </span>
                <span className="text-[#C4A574] flex items-center gap-1 group-hover:translate-x-2 transition-transform duration-300 uppercase tracking-widest text-[10px]">
                  Open <ArrowRight size={14} />
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}

      {/* Animated Dialogue Box */}
      {showNewMatter && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-300">
          <div className="bg-[#12100E] border border-[#C4A574]/30 rounded-3xl p-8 max-w-md w-full shadow-[0_20px_50px_rgba(0,0,0,0.5)] animate-in zoom-in-95 slide-in-from-bottom-10 duration-500 ease-out">
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-2xl font-serif text-white">Create New Matter</h2>
              <button 
                onClick={() => setShowNewMatter(false)}
                className="text-gray-400 hover:text-white transition-colors bg-white/5 hover:bg-white/10 p-2 rounded-full"
              >
                <X size={20} />
              </button>
            </div>
            
            <form onSubmit={handleCreateMatter} className="space-y-5">
              <div>
                <label className="block text-sm font-medium text-gray-400 mb-1.5 ml-1">Client</label>
                <input 
                  type="text" 
                  required
                  value={newClient}
                  onChange={e => setNewClient(e.target.value)}
                  className="w-full bg-black/50 border border-[#C4A574]/20 rounded-xl px-4 py-3 focus:outline-none focus:ring-2 focus:ring-[#C4A574] text-white transition-all placeholder-gray-600"
                  placeholder="e.g. Acme Corp"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-400 mb-1.5 ml-1">Matter Name</label>
                <input 
                  type="text" 
                  required
                  value={newMatter}
                  onChange={e => setNewMatterName(e.target.value)}
                  className="w-full bg-black/50 border border-[#C4A574]/20 rounded-xl px-4 py-3 focus:outline-none focus:ring-2 focus:ring-[#C4A574] text-white transition-all placeholder-gray-600"
                  placeholder="e.g. Project Alpha"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-400 mb-1.5 ml-1">Description (Optional)</label>
                <textarea 
                  value={newDesc}
                  onChange={e => setNewDesc(e.target.value)}
                  className="w-full bg-black/50 border border-[#C4A574]/20 rounded-xl px-4 py-3 focus:outline-none focus:ring-2 focus:ring-[#C4A574] text-white transition-all placeholder-gray-600 h-24 resize-none"
                  placeholder="Matter details..."
                />
              </div>
              
              <button 
                type="submit"
                disabled={creating}
                className="w-full bg-[#C4A574] hover:bg-[#d8b884] text-[#0a0806] font-bold py-3.5 rounded-xl transition-all shadow-lg mt-2 disabled:opacity-50 flex justify-center items-center gap-2"
              >
                {creating && <Loader2 size={18} className="animate-spin" />}
                Provision Sealed Tenant
              </button>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
