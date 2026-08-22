import { useState, useEffect, useLayoutEffect, useRef } from 'react';
import { ShieldAlert, CheckCircle2, Shield, Lock, Key, Layout, Monitor, Plus, Server, Network, Trash2, ChevronDown } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { useTheme } from '../components/ThemeProvider';
import { useLocation, useNavigate } from 'react-router-dom';
import gsap from 'gsap';
import { Flip } from 'gsap/Flip';

gsap.registerPlugin(Flip);

// Custom GSAP Dropdown
function GSAPDropdown({ options, value, onChange, label }: { options: string[], value: string, onChange: (v:string)=>void, label: string }) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  
  useEffect(() => {
    if (open) {
      gsap.fromTo(menuRef.current, 
        { height: 0, opacity: 0, scale: 0.95 }, 
        { height: 'auto', opacity: 1, scale: 1, duration: 0.3, ease: 'back.out(1.7)' }
      );
    } else if (menuRef.current) {
      gsap.to(menuRef.current, { height: 0, opacity: 0, scale: 0.95, duration: 0.2, ease: 'power2.in' });
    }
  }, [open]);

  return (
    <div className="relative mb-6">
      <label className="block text-xs font-bold text-slate-700/70 uppercase tracking-wider mb-2">{label}</label>
      <div 
        className="w-full bg-white/40 backdrop-blur-md border border-white/40 text-slate-900 text-sm rounded-xl px-4 py-3 cursor-pointer flex justify-between items-center shadow-sm hover:bg-white/60 transition-colors"
        onClick={() => setOpen(!open)}
      >
        <span>{value}</span>
        <ChevronDown size={16} className={`transition-transform duration-300 ${open ? 'rotate-180' : ''}`} />
      </div>
      
      <div 
        ref={menuRef}
        className="absolute top-full left-0 w-full mt-2 bg-white/80 backdrop-blur-2xl border border-white/50 rounded-xl shadow-xl overflow-hidden z-20"
        style={{ height: 0, opacity: 0 }}
      >
        {options.map(opt => (
          <div 
            key={opt}
            className={`px-4 py-3 text-sm cursor-pointer transition-colors ${value === opt ? 'bg-indigo-50/50 text-indigo-700 font-medium' : 'hover:bg-slate-50/50'}`}
            onClick={() => {
              onChange(opt);
              setOpen(false);
            }}
          >
            {opt}
          </div>
        ))}
      </div>
    </div>
  );
}

// 3D Tilt Card
function TiltCard({ children, className }: { children: React.ReactNode, className?: string }) {
  const cardRef = useRef<HTMLDivElement>(null);

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!cardRef.current) return;
    const rect = cardRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    const centerX = rect.width / 2;
    const centerY = rect.height / 2;
    
    // Rotate max 10 degrees
    const rotateX = ((y - centerY) / centerY) * -10;
    const rotateY = ((x - centerX) / centerX) * 10;
    
    gsap.to(cardRef.current, {
      rotateX,
      rotateY,
      transformPerspective: 1000,
      ease: "power2.out",
      duration: 0.4
    });
  };

  const handleMouseLeave = () => {
    if (!cardRef.current) return;
    gsap.to(cardRef.current, {
      rotateX: 0,
      rotateY: 0,
      ease: "elastic.out(1, 0.3)",
      duration: 1
    });
  };

  return (
    <div 
      ref={cardRef} 
      className={className} 
      onMouseMove={handleMouseMove} 
      onMouseLeave={handleMouseLeave}
      style={{ transformStyle: 'preserve-3d' }}
    >
      {children}
    </div>
  );
}

export default function SettingsPage() {
  const { theme, setTheme } = useTheme();
  const location = useLocation();
  const navigate = useNavigate();
  const pageRef = useRef<HTMLDivElement>(null);
  const bgRef = useRef<HTMLDivElement>(null);
  
  // Background animation
  useEffect(() => {
    if (!bgRef.current) return;
    const ctx = gsap.context(() => {
      gsap.to(".bg-blob-1", {
        x: "random(-200, 200)",
        y: "random(-200, 200)",
        duration: "random(10, 20)",
        repeat: -1,
        yoyo: true,
        ease: "sine.inOut"
      });
      gsap.to(".bg-blob-2", {
        x: "random(-200, 200)",
        y: "random(-200, 200)",
        duration: "random(12, 22)",
        repeat: -1,
        yoyo: true,
        ease: "sine.inOut"
      });
    }, bgRef);
    return () => ctx.revert();
  }, []);

  const getTabFromPath = () => {
    const p = location.pathname;
    if (p.includes('security')) return 'Security & TLS';
    if (p.includes('keys')) return 'API Keys';
    if (p.includes('replication')) return 'Replication (Raft)';
    return 'General Settings';
  };
  
  const [activeTab, setActiveTab] = useState(getTabFromPath());
  
  useEffect(() => {
    const nextTab = getTabFromPath();
    if (nextTab !== activeTab) setActiveTab(nextTab);
  }, [location.pathname]);

  const onTabClick = (tabName: string) => {
    // Flip animation for the active pill indicator
    const state = Flip.getState('.tab-pill');
    setActiveTab(tabName);
    requestAnimationFrame(() => {
      Flip.from(state, {
        duration: 0.5,
        ease: 'power3.out',
        absolute: true
      });
    });

    if (tabName === 'Security & TLS') navigate('/settings/security');
    else if (tabName === 'API Keys') navigate('/settings/keys');
    else if (tabName === 'Replication (Raft)') navigate('/settings/replication');
    else navigate('/settings');
  };

  useLayoutEffect(() => {
    if (pageRef.current) {
      gsap.fromTo('.settings-panel', 
        { y: 30, opacity: 0, scale: 0.98 }, 
        { y: 0, opacity: 1, scale: 1, duration: 0.6, stagger: 0.1, ease: 'power3.out' }
      );
    }
  }, [activeTab]);

  const tabs = [
    { name: 'General Settings', icon: <Layout size={16} /> },
    { name: 'Security & TLS', icon: <Shield size={16} /> },
    { name: 'API Keys', icon: <Key size={16} /> },
    { name: 'Replication (Raft)', icon: <Network size={16} /> }
  ];

  // Logic states
  const [apiKeys, setApiKeys] = useState<any[]>([]);
  const [newKeyName, setNewKeyName] = useState('');
  const [showKeyModal, setShowKeyModal] = useState(false);
  const [generatedKey, setGeneratedKey] = useState<string | null>(null);
  const [raftStatus, setRaftStatus] = useState<any>(null);

  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [timezone, setTimezone] = useState('UTC (Coordinated Universal Time)');

  const loadApiKeys = async () => {
    try {
      const res = await fetchWithAuth('/api/admin/keys');
      if (res.ok) {
        const data = await res.json();
        setApiKeys(data || []);
      }
    } catch (e) {}
  };

  const loadRaftStatus = async () => {
    try {
      const res = await fetchWithAuth('/api/admin/raft/status');
      if (res.ok) setRaftStatus(await res.json());
    } catch (e) {}
  };

  useEffect(() => {
    if (activeTab === 'API Keys') loadApiKeys();
    if (activeTab === 'Replication (Raft)') loadRaftStatus();
  }, [activeTab]);

  // Keys list staggering animation
  useEffect(() => {
    if (activeTab === 'API Keys' && apiKeys.length > 0) {
      gsap.fromTo('.key-row', 
        { opacity: 0, x: -20 },
        { opacity: 1, x: 0, duration: 0.5, stagger: 0.05, ease: 'back.out(1.5)' }
      );
    }
  }, [apiKeys, activeTab]);

  const handleCreateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newKeyName) return;
    try {
      const res = await fetchWithAuth('/api/admin/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newKeyName })
      });
      if (res.ok) {
        const data = await res.json();
        setGeneratedKey(data.api_key);
        setNewKeyName('');
        loadApiKeys();
      }
    } catch (e) {}
  };

  const handleRevokeKey = async (id: string) => {
    try {
      const res = await fetchWithAuth('/api/admin/keys/revoke', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id })
      });
      if (res.ok) loadApiKeys();
    } catch (e) {}
  };

  const handlePasswordChange = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(null);
    if (newPassword !== confirmPassword) {
      setError("New passwords do not match."); return;
    }
    if (newPassword.length < 8) {
      setError("Password must be at least 8 characters long."); return;
    }
    setLoading(true);
    try {
      const res = await fetchWithAuth('/api/admin/password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
      });
      if (!res.ok) throw new Error("Failed to update password");
      setSuccess("Password successfully updated. Please use the new password on your next login.");
      setOldPassword(''); setNewPassword(''); setConfirmPassword('');
    } catch (err: any) {
      setError(err.message || "An error occurred");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative flex h-full w-full overflow-hidden bg-slate-50">
      
      {/* Animated Background Mesh */}
      <div ref={bgRef} className="absolute inset-0 z-0 overflow-hidden pointer-events-none">
        <div className="bg-blob-1 absolute top-[-10%] left-[-10%] w-[50vw] h-[50vw] rounded-full bg-indigo-300/30 blur-[100px] mix-blend-multiply"></div>
        <div className="bg-blob-2 absolute bottom-[-10%] right-[-10%] w-[60vw] h-[60vw] rounded-full bg-blue-300/30 blur-[120px] mix-blend-multiply"></div>
        <div className="absolute inset-0 bg-white/40 backdrop-blur-[50px]"></div>
      </div>

      {/* Settings Navigation Sidebar */}
      <div className="w-64 border-r border-white/20 bg-white/20 backdrop-blur-3xl p-6 hidden md:block z-10 shadow-[4px_0_24px_rgba(0,0,0,0.02)]">
        <h2 className="text-xl font-black bg-gradient-to-r from-slate-900 to-slate-600 bg-clip-text text-transparent tracking-tight mb-8">Settings</h2>
        <nav className="space-y-2 relative">
          {tabs.map((tab) => {
            const isActive = activeTab === tab.name;
            return (
              <button
                key={tab.name}
                onClick={() => onTabClick(tab.name)}
                className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-semibold transition-colors relative z-10 ${
                  isActive ? 'text-indigo-900' : 'text-slate-500 hover:text-slate-800'
                }`}
              >
                {isActive && (
                  <div className="tab-pill absolute inset-0 bg-white shadow-[0_4px_20px_rgba(0,0,0,0.05)] rounded-xl -z-10 border border-white/60"></div>
                )}
                <span className={`${isActive ? 'text-indigo-600' : 'text-slate-400'}`}>
                  {tab.icon}
                </span>
                {tab.name}
              </button>
            );
          })}
        </nav>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 overflow-y-auto p-8 z-10" ref={pageRef}>
        <div className="max-w-4xl mx-auto">
          
          <div className="mb-8 md:hidden">
            <h2 className="text-2xl font-black text-slate-900 tracking-tight">Settings</h2>
          </div>

          <div className="mb-10 settings-panel">
            <h3 className="text-4xl font-black bg-gradient-to-br from-slate-900 to-slate-600 bg-clip-text text-transparent tracking-tight">{activeTab}</h3>
            <p className="text-base text-slate-500 mt-2 font-medium">
              Manage your {activeTab.toLowerCase().replace(' & tls', '').replace(' (raft)', '')} preferences and configurations.
            </p>
          </div>

          {activeTab === 'General Settings' && (
            <div className="settings-panel bg-white/40 backdrop-blur-2xl border border-white/50 rounded-3xl shadow-xl overflow-hidden p-8">
              <div className="flex items-center gap-6 mb-8">
                <div className="w-20 h-20 bg-gradient-to-br from-white to-slate-100 border border-white shadow-lg rounded-2xl flex items-center justify-center">
                  <Monitor size={32} className="text-indigo-500" />
                </div>
                <div>
                  <h4 className="text-xl font-bold text-slate-900">Workspace Details</h4>
                  <p className="text-sm text-slate-500 font-medium mt-1">Configure global dashboard preferences.</p>
                </div>
              </div>
              
              <div className="space-y-2">
                <GSAPDropdown 
                  label="Display Theme"
                  value={theme === 'system' ? 'System Default' : theme === 'light' ? 'Light Theme' : 'Dark Theme'}
                  options={['System Default', 'Light Theme', 'Dark Theme']}
                  onChange={(v) => {
                    if(v==='System Default') setTheme('system');
                    else if(v==='Light Theme') setTheme('light');
                    else setTheme('dark');
                  }}
                />
                
                <GSAPDropdown 
                  label="Timezone"
                  value={timezone}
                  options={['UTC (Coordinated Universal Time)', 'America/New_York (EST)', 'America/Los_Angeles (PST)']}
                  onChange={(v) => setTimezone(v)}
                />
              </div>
            </div>
          )}

          {activeTab === 'Security & TLS' && (
            <div className="settings-panel bg-white/40 backdrop-blur-2xl border border-white/50 rounded-3xl shadow-xl overflow-hidden">
              <div className="px-8 py-6 border-b border-white/30 bg-white/20 flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-red-100 text-red-600 rounded-lg shadow-sm border border-red-200">
                    <Shield size={20} />
                  </div>
                  <h4 className="text-base font-bold text-slate-900 tracking-wide">Administrator Password</h4>
                </div>
              </div>
              
              <div className="p-8">
                {error && (
                  <div className="bg-red-50 text-red-600 p-4 rounded-2xl mb-8 text-sm font-semibold border border-red-100 flex items-center gap-3 shadow-sm">
                    <ShieldAlert size={18} /> {error}
                  </div>
                )}
                {success && (
                  <div className="bg-emerald-50 text-emerald-700 p-4 rounded-2xl mb-8 text-sm font-semibold border border-emerald-100 flex items-center gap-3 shadow-sm">
                    <CheckCircle2 size={18} /> {success}
                  </div>
                )}

                <form onSubmit={handlePasswordChange} className="space-y-6 max-w-xl">
                  <div>
                    <label className="block text-xs font-bold text-slate-700/70 uppercase tracking-wider mb-2">Current Password</label>
                    <div className="relative">
                      <Lock size={18} className="absolute left-4 top-1/2 -translate-y-1/2 text-slate-400" />
                      <input
                        type="password"
                        className="w-full bg-white/50 border border-white/50 text-slate-900 text-sm rounded-2xl pl-12 pr-4 py-3.5 outline-none focus:border-indigo-400 focus:ring-4 focus:ring-indigo-100 transition-all shadow-inner"
                        placeholder="••••••••"
                        value={oldPassword} onChange={e => setOldPassword(e.target.value)} required
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="block text-xs font-bold text-slate-700/70 uppercase tracking-wider mb-2">New Password</label>
                      <div className="relative">
                        <Lock size={18} className="absolute left-4 top-1/2 -translate-y-1/2 text-slate-400" />
                        <input
                          type="password"
                          className="w-full bg-white/50 border border-white/50 text-slate-900 text-sm rounded-2xl pl-12 pr-4 py-3.5 outline-none focus:border-indigo-400 focus:ring-4 focus:ring-indigo-100 transition-all shadow-inner"
                          placeholder="••••••••"
                          value={newPassword} onChange={e => setNewPassword(e.target.value)} required
                        />
                      </div>
                    </div>
                    <div>
                      <label className="block text-xs font-bold text-slate-700/70 uppercase tracking-wider mb-2">Confirm Password</label>
                      <div className="relative">
                        <Lock size={18} className="absolute left-4 top-1/2 -translate-y-1/2 text-slate-400" />
                        <input
                          type="password"
                          className="w-full bg-white/50 border border-white/50 text-slate-900 text-sm rounded-2xl pl-12 pr-4 py-3.5 outline-none focus:border-indigo-400 focus:ring-4 focus:ring-indigo-100 transition-all shadow-inner"
                          placeholder="••••••••"
                          value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} required
                        />
                      </div>
                    </div>
                  </div>

                  <div className="pt-6">
                    <button type="submit" className="bg-slate-900 hover:bg-slate-800 text-white px-8 py-3.5 rounded-2xl font-bold text-sm transition-all shadow-lg hover:shadow-xl hover:-translate-y-0.5 disabled:opacity-50" disabled={loading}>
                      {loading ? 'Updating...' : 'Update Password'}
                    </button>
                  </div>
                </form>
              </div>
            </div>
          )}

          {activeTab === 'API Keys' && (
            <div className="space-y-8 settings-panel">
              
              {showKeyModal && (
                <div className="fixed inset-0 bg-slate-900/40 backdrop-blur-sm z-50 flex items-center justify-center p-4">
                  <div className="bg-white/80 backdrop-blur-2xl rounded-3xl p-8 max-w-md w-full shadow-2xl border border-white">
                    <h3 className="text-2xl font-black text-slate-900 mb-6 tracking-tight">Create Secret Key</h3>
                    {!generatedKey ? (
                      <form onSubmit={handleCreateKey}>
                        <label className="block text-xs font-bold text-slate-700/70 uppercase tracking-wider mb-2">Key Name (e.g. Production App)</label>
                        <input autoFocus className="w-full bg-white/50 border border-white text-slate-900 text-sm rounded-2xl px-4 py-3.5 mb-8 outline-none focus:ring-4 focus:ring-indigo-100 focus:border-indigo-400 transition-all shadow-inner" value={newKeyName} onChange={e=>setNewKeyName(e.target.value)} />
                        <div className="flex justify-end gap-3">
                          <button type="button" onClick={()=>setShowKeyModal(false)} className="px-6 py-3 bg-white border border-gray-200 shadow-sm rounded-xl font-bold text-slate-700 hover:bg-gray-50 transition-colors">Cancel</button>
                          <button type="submit" className="px-6 py-3 bg-slate-900 text-white shadow-lg rounded-xl font-bold hover:-translate-y-0.5 transition-all">Generate Key</button>
                        </div>
                      </form>
                    ) : (
                      <div>
                        <div className="bg-amber-100/50 border border-amber-200 text-amber-800 p-4 rounded-2xl text-sm mb-6 font-medium shadow-inner">
                          <strong className="block mb-1 text-base">⚠️ Important:</strong> Copy this key now. For security reasons, you will never be able to see it again!
                        </div>
                        <div className="bg-slate-900 text-emerald-400 font-mono p-5 rounded-2xl break-all mb-8 select-all shadow-inner border border-slate-700 text-lg">
                          {generatedKey}
                        </div>
                        <button onClick={()=>{setShowKeyModal(false); setGeneratedKey(null);}} className="w-full py-3.5 bg-slate-900 text-white rounded-xl font-bold shadow-lg hover:-translate-y-0.5 transition-all">I have copied my key</button>
                      </div>
                    )}
                  </div>
                </div>
              )}

              <div className="bg-white/40 backdrop-blur-2xl border border-white/50 rounded-3xl shadow-xl p-8 flex flex-col md:flex-row items-center justify-between gap-6">
                <div className="flex items-center gap-6">
                  <div className="w-16 h-16 bg-gradient-to-br from-red-50 to-orange-50 text-red-600 border border-red-100 rounded-2xl flex items-center justify-center shadow-sm">
                    <Key size={28} />
                  </div>
                  <div>
                    <h4 className="text-xl font-bold text-slate-900">Manage Programmatic Access</h4>
                    <p className="text-sm text-slate-500 mt-1 font-medium max-w-md">
                      API keys allow programmatic access to the cluster. Keep keys secure and never commit them to client-side code repositories.
                    </p>
                  </div>
                </div>
                <button onClick={()=>setShowKeyModal(true)} className="shrink-0 flex items-center gap-2 bg-slate-900 text-white hover:bg-slate-800 px-6 py-3.5 rounded-2xl font-bold text-sm transition-all shadow-lg hover:shadow-xl hover:-translate-y-0.5">
                  <Plus size={18} /> Generate New Key
                </button>
              </div>
              
              <div className="bg-white/40 backdrop-blur-2xl border border-white/50 rounded-3xl shadow-xl overflow-hidden">
                <table className="w-full text-left text-sm">
                  <thead className="bg-white/40 border-b border-white/50 backdrop-blur-md">
                    <tr>
                      <th className="px-8 py-5 font-bold text-slate-700 uppercase tracking-wider text-xs">Name</th>
                      <th className="px-8 py-5 font-bold text-slate-700 uppercase tracking-wider text-xs">Key Prefix</th>
                      <th className="px-8 py-5 font-bold text-slate-700 uppercase tracking-wider text-xs">Created At</th>
                      <th className="px-8 py-5 font-bold text-slate-700 uppercase tracking-wider text-xs text-right">Action</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-white/40">
                    {apiKeys.length === 0 ? (
                      <tr><td colSpan={4} className="px-8 py-12 text-center text-slate-500 font-medium">No API keys generated yet.</td></tr>
                    ) : (
                      apiKeys.map(k => (
                        <tr key={k.id} className="key-row hover:bg-white/60 transition-colors">
                          <td className="px-8 py-5 font-bold text-slate-900">{k.name}</td>
                          <td className="px-8 py-5 font-mono text-slate-500 font-medium bg-white/30 rounded inline-block mt-3 mb-3 ml-8">{k.prefix}••••••••••••</td>
                          <td className="px-8 py-5 text-slate-500 font-medium">{new Date(k.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}</td>
                          <td className="px-8 py-5 text-right">
                            <button onClick={()=>handleRevokeKey(k.id)} className="p-2 text-slate-400 hover:text-red-500 hover:bg-red-50 rounded-lg transition-colors inline-flex items-center justify-center">
                              <Trash2 size={18} />
                            </button>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {activeTab === 'Replication (Raft)' && (
            <div className="space-y-8 settings-panel">
              <div className="bg-white/40 backdrop-blur-2xl border border-white/50 rounded-3xl shadow-xl overflow-hidden p-8">
                <div className="flex flex-col md:flex-row md:items-center justify-between mb-8 gap-4">
                  <div className="flex items-center gap-6">
                    <div className="w-16 h-16 bg-gradient-to-br from-indigo-500 to-purple-600 text-white shadow-lg shadow-indigo-500/30 rounded-2xl flex items-center justify-center">
                      <Network size={28} />
                    </div>
                    <div>
                      <h4 className="text-xl font-bold text-slate-900">Cluster Topology</h4>
                      <p className="text-sm text-slate-500 font-medium mt-1">Live visualization of Raft consensus state</p>
                    </div>
                  </div>
                  {raftStatus && (
                    <div className="text-right bg-white/60 px-6 py-3 rounded-2xl border border-white shadow-sm">
                      <div className="text-xs text-slate-500 uppercase tracking-widest font-bold mb-1">Current Term</div>
                      <div className="text-3xl font-black bg-gradient-to-r from-indigo-600 to-purple-600 bg-clip-text text-transparent">{raftStatus.term}</div>
                    </div>
                  )}
                </div>

                {!raftStatus ? (
                  <div className="text-center py-12 text-slate-400 font-medium">Loading distributed consensus state...</div>
                ) : (
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8" style={{ perspective: '1000px' }}>
                    {raftStatus.peers?.map((peer: any) => (
                      <TiltCard key={peer.id} className={`p-6 border rounded-3xl relative overflow-hidden transition-colors ${peer.state === 'Leader' ? 'bg-gradient-to-br from-white/90 to-indigo-50/90 border-indigo-200 shadow-xl shadow-indigo-500/10' : 'bg-white/60 border-white shadow-lg'}`}>
                        {peer.state === 'Leader' && (
                          <div className="absolute top-0 right-0 w-32 h-32 bg-indigo-500/10 blur-2xl rounded-full -mr-10 -mt-10 pointer-events-none"></div>
                        )}
                        <div className="flex items-center gap-3 mb-4 relative z-10">
                          <div className={`p-2 rounded-xl ${peer.state === 'Leader' ? 'bg-indigo-100 text-indigo-600' : 'bg-slate-100 text-slate-500'}`}>
                            <Server size={18} />
                          </div>
                          <span className={`text-lg font-black ${peer.state === 'Leader' ? 'text-indigo-900' : 'text-slate-700'}`}>{peer.id}</span>
                        </div>
                        <div className="text-sm text-slate-500 font-mono mb-5 relative z-10">{peer.address}</div>
                        <span className={`inline-flex items-center px-3 py-1.5 rounded-lg text-xs font-bold tracking-wide relative z-10 ${
                          peer.state === 'Leader' ? 'bg-indigo-600 text-white shadow-md' : 'bg-white text-slate-600 shadow-sm border border-slate-200'
                        }`}>
                          {peer.state === 'Leader' && <div className="w-1.5 h-1.5 rounded-full bg-white animate-pulse mr-2"></div>}
                          {peer.state}
                        </span>
                      </TiltCard>
                    ))}
                  </div>
                )}
                
                {raftStatus && (
                  <div className="bg-slate-900 rounded-2xl p-6 flex items-center justify-between text-sm shadow-xl relative overflow-hidden">
                    <div className="absolute inset-0 bg-gradient-to-r from-emerald-500/10 to-transparent pointer-events-none"></div>
                    <span className="text-slate-400 font-medium relative z-10">Applied Index: <strong className="text-white font-mono text-lg ml-2">{raftStatus.applied_index}</strong></span>
                    <div className="flex items-center gap-3 relative z-10 bg-black/30 px-4 py-2 rounded-xl">
                      <div className="w-2.5 h-2.5 rounded-full bg-emerald-400 shadow-[0_0_12px_rgba(52,211,153,0.8)] animate-pulse"></div>
                      <span className="text-emerald-400 font-bold tracking-wide">Consensus Healthy</span>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

        </div>
      </div>
    </div>
  );
}
