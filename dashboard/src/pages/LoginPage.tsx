import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Lock, User, Loader2, AlertCircle, ShieldCheck } from 'lucide-react';
import logo from '../assets/logo.jpg';

export default function LoginPage() {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleLogin = async (e: React.FormEvent) => {
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
        body: JSON.stringify({ username: username.trim(), password }),
      });

      // Always try to read as text first, then parse as JSON
      const text = await res.text();
      let data: any = null;
      try {
        data = JSON.parse(text);
      } catch {
        // response was plain text (e.g. "Invalid credentials\n")
      }

      if (!res.ok) {
        // Backend returns plain-text errors like "Invalid credentials\n"
        const msg =
          data?.error ||
          data?.message ||
          text.trim() ||
          `Login failed (HTTP ${res.status})`;
        throw new Error(msg);
      }

      if (!data?.token) {
        throw new Error('Server did not return a session token. Please try again.');
      }

      localStorage.setItem('dbx_token', data.token);
      navigate('/');
    } catch (err: any) {
      setError(err.message || 'An unexpected error occurred. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen w-full flex flex-col items-center justify-center bg-gradient-to-br from-slate-950 via-slate-900 to-slate-800 px-4 py-8">
      {/* Ambient glow */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute top-1/4 left-1/2 -translate-x-1/2 w-[600px] h-[600px] bg-red-600/10 rounded-full blur-3xl" />
      </div>

      {/* Card — no overflow-hidden so button is never clipped */}
      <div className="relative w-full max-w-[420px] bg-slate-900/80 backdrop-blur-xl border border-slate-700/60 rounded-2xl shadow-2xl shadow-black/40">
        {/* Top accent bar */}
        <div className="h-1 w-full bg-gradient-to-r from-red-600 via-red-500 to-rose-400 rounded-t-2xl" />

        <div className="p-8 pb-7">
          {/* Logo & Heading */}
          <div className="flex flex-col items-center mb-8">
            <div className="w-16 h-16 rounded-2xl border border-slate-700 shadow-lg overflow-hidden mb-5 ring-1 ring-red-500/30">
              <img src={logo} alt="DBX Logo" className="w-full h-full object-cover" />
            </div>
            <h1 className="text-2xl font-bold text-white tracking-tight">Welcome to DBX</h1>
            <p className="text-slate-400 text-sm mt-1.5 text-center">
              Sign in to manage your high-performance clusters.
            </p>
          </div>

          {/* Error alert */}
          {error && (
            <div className="flex items-start gap-2.5 bg-red-500/10 border border-red-500/30 text-red-400 rounded-xl p-3.5 mb-6 text-sm font-medium">
              <AlertCircle size={16} className="mt-0.5 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <form onSubmit={handleLogin} className="flex flex-col gap-5">
            {/* Username */}
            <div>
              <label
                htmlFor="login-username"
                className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2"
              >
                Username
              </label>
              <div className="relative">
                <span className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5 text-slate-500">
                  <User size={16} />
                </span>
                <input
                  id="login-username"
                  type="text"
                  placeholder="admin"
                  value={username}
                  autoComplete="username"
                  onChange={e => setUsername(e.target.value)}
                  disabled={loading}
                  style={{ paddingLeft: '40px' }}
                  className="w-full pr-4 py-3 bg-slate-800/70 border border-slate-700 rounded-xl text-white text-sm outline-none
                             focus:border-red-500 focus:ring-2 focus:ring-red-500/25 transition-all
                             disabled:opacity-50 placeholder:text-slate-600"
                />
              </div>
            </div>

            {/* Password */}
            <div>
              <label
                htmlFor="login-password"
                className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2"
              >
                Password
              </label>
              <div className="relative">
                <span className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5 text-slate-500">
                  <Lock size={16} />
                </span>
                <input
                  id="login-password"
                  type="password"
                  placeholder="••••••••"
                  value={password}
                  autoComplete="current-password"
                  onChange={e => setPassword(e.target.value)}
                  disabled={loading}
                  style={{ paddingLeft: '40px' }}
                  className="w-full pr-4 py-3 bg-slate-800/70 border border-slate-700 rounded-xl text-white text-sm outline-none
                             focus:border-red-500 focus:ring-2 focus:ring-red-500/25 transition-all
                             disabled:opacity-50 placeholder:text-slate-600"
                />
              </div>
            </div>

            {/* Submit — always at the bottom of the form, always visible */}
            <button
              type="submit"
              disabled={loading}
              id="login-submit-btn"
              className="w-full mt-1 py-3 bg-red-600 hover:bg-red-500 active:bg-red-700
                         disabled:opacity-60 disabled:cursor-not-allowed
                         text-white font-bold text-sm rounded-xl
                         transition-all duration-200 shadow-lg shadow-red-600/30
                         hover:shadow-red-500/40 hover:-translate-y-0.5 active:translate-y-0
                         flex items-center justify-center gap-2"
            >
              {loading ? (
                <>
                  <Loader2 size={16} className="animate-spin" />
                  Signing in…
                </>
              ) : (
                <>
                  <ShieldCheck size={16} />
                  Sign In
                </>
              )}
            </button>
          </form>
        </div>

        {/* Footer */}
        <div className="px-8 pb-6 flex items-center justify-center">
          <p className="text-center text-xs text-slate-600">
            DBX Cloud Enterprise &mdash; v2.0
          </p>
        </div>
      </div>
    </div>
  );
}
