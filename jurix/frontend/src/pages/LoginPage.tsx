import { FormEvent, useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { ShieldCheck, Scale, Loader2 } from 'lucide-react';
import MotionBackground from '../components/MotionBackground';

export default function LoginPage() {
  const [username, setUsername] = useState('admin@alpha.com');
  const [password, setPassword] = useState('adminadminadmin');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    if (localStorage.getItem('jurix_token')) {
      navigate('/');
    }
  }, [navigate]);

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password.trim()) {
      setError('Username and password are required.');
      return;
    }

    setLoading(true);
    setError('');

    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: username.trim(), password }),
      });

      const text = await res.text();
      let data: any = null;
      try {
        data = JSON.parse(text);
      } catch {
        // text is fallback
      }

      if (!res.ok) {
        throw new Error(data?.detail || data?.error || 'Login failed');
      }

      if (!data?.token) {
        throw new Error('Server did not return a session token.');
      }

      localStorage.setItem('jurix_token', data.token);
      navigate('/');
    } catch (err: any) {
      setError(err.message || 'An unexpected error occurred.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen w-full relative flex flex-col items-center justify-center bg-[#0a0806] overflow-hidden text-[#FDFCF8]">
      
      <MotionBackground />

      {/* Top Left Logo */}
      <div className="z-10 absolute top-8 left-10 text-2xl font-serif tracking-widest text-[#C4A574] flex items-center gap-3 animate-in slide-in-from-top-4 fade-in duration-700">
        <Scale size={32} />
        JURIX
      </div>

      {/* Center Login Card */}
      <div className="z-10 w-full max-w-md mx-auto px-6 animate-in slide-in-from-bottom-8 fade-in duration-1000 ease-out">
        <div className="bg-[#12100E]/70 backdrop-blur-2xl p-10 rounded-3xl border border-[#C4A574]/30 shadow-[0_30px_60px_rgba(0,0,0,0.5)]">
          <div className="text-center mb-8">
            <h1 className="text-4xl font-serif mb-3 tracking-tight">Chambers</h1>
            <p className="text-sm text-[#C4A574] font-medium tracking-wide uppercase">Secure Firm Access</p>
          </div>

          <form onSubmit={handleLogin} className="space-y-6">
            {error && (
              <div className="p-4 bg-red-900/30 text-red-200 text-sm rounded-xl border border-red-500/50 flex items-start gap-2">
                <ShieldCheck size={18} className="shrink-0 mt-0.5" />
                <span>{error}</span>
              </div>
            )}
            
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-400 mb-1.5 ml-1">Work email</label>
                <input
                  type="email"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="w-full bg-black/40 border border-[#C4A574]/20 rounded-xl px-5 py-3.5 focus:outline-none focus:ring-2 focus:ring-[#C4A574] focus:border-transparent transition-all text-white placeholder-gray-600"
                  placeholder="email@firm.com"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-400 mb-1.5 ml-1">Password</label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full bg-black/40 border border-[#C4A574]/20 rounded-xl px-5 py-3.5 focus:outline-none focus:ring-2 focus:ring-[#C4A574] focus:border-transparent transition-all font-mono text-white placeholder-gray-600"
                  placeholder="••••••••••••"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={loading}
              className="w-full bg-[#C4A574] text-[#0a0806] rounded-xl px-4 py-4 font-bold tracking-wide hover:bg-[#d8b884] transition-all flex items-center justify-center gap-2 mt-4 hover:scale-[1.02] active:scale-[0.98]"
            >
              {loading ? <Loader2 size={20} className="animate-spin" /> : <ShieldCheck size={20} />}
              {loading ? 'Authenticating...' : 'Enter Chambers'}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
