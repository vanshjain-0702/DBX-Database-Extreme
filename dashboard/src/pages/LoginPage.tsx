import type { FormEvent } from 'react';
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
        body: JSON.stringify({ username: username.trim(), password }),
      });

      const text = await res.text();
      let data: { token?: string; error?: string; message?: string } | null = null;
      try {
        data = JSON.parse(text);
      } catch {
        /* plain-text error */
      }

      if (!res.ok) {
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
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'An unexpected error occurred. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen w-full flex flex-col items-center justify-center bg-zinc-950 px-4 py-8">
      <div className="relative w-full max-w-[400px] bg-zinc-900 border border-zinc-800 rounded-lg">
        <div className="h-0.5 w-full bg-[#c2410c] rounded-t-lg" />

        <div className="p-7">
          <div className="flex flex-col items-center mb-7">
            <div className="w-12 h-12 rounded-md border border-zinc-700 overflow-hidden mb-4">
              <img src={logo} alt="DBX" className="w-full h-full object-cover" />
            </div>
            <h1 className="text-[20px] font-semibold text-zinc-50 tracking-tight">DBX control plane</h1>
            <p className="text-zinc-400 text-[13px] mt-1.5 text-center">
              Sign in to manage per-tenant engines.
            </p>
          </div>

          {error && (
            <div className="flex items-start gap-2 bg-rose-500/10 border border-rose-500/30 text-rose-400 rounded-md p-3 mb-5 text-[13px] font-medium">
              <AlertCircle size={15} className="mt-0.5 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <form onSubmit={handleLogin} className="flex flex-col gap-4">
            <div>
              <label htmlFor="login-username" className="block text-[11px] font-semibold text-zinc-400 uppercase tracking-wider mb-1.5">
                Username
              </label>
              <div className="relative">
                <span className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-zinc-500">
                  <User size={15} />
                </span>
                <input
                  id="login-username"
                  type="text"
                  placeholder="admin"
                  value={username}
                  autoComplete="username"
                  onChange={e => setUsername(e.target.value)}
                  disabled={loading}
                  className="w-full pl-9 pr-3 py-2.5 bg-zinc-950 border border-zinc-800 rounded-md text-zinc-50 text-[13px] outline-none
                             focus:border-[#c2410c] focus:ring-2 focus:ring-[#c2410c]/20
                             disabled:opacity-50 placeholder:text-zinc-600"
                />
              </div>
            </div>

            <div>
              <label htmlFor="login-password" className="block text-[11px] font-semibold text-zinc-400 uppercase tracking-wider mb-1.5">
                Password
              </label>
              <div className="relative">
                <span className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-zinc-500">
                  <Lock size={15} />
                </span>
                <input
                  id="login-password"
                  type="password"
                  placeholder="Password"
                  value={password}
                  autoComplete="current-password"
                  onChange={e => setPassword(e.target.value)}
                  disabled={loading}
                  className="w-full pl-9 pr-3 py-2.5 bg-zinc-950 border border-zinc-800 rounded-md text-zinc-50 text-[13px] outline-none
                             focus:border-[#c2410c] focus:ring-2 focus:ring-[#c2410c]/20
                             disabled:opacity-50 placeholder:text-zinc-600"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={loading}
              id="login-submit-btn"
              className="w-full mt-1 py-2.5 bg-[#c2410c] hover:bg-[#9a3412] active:bg-[#7c2d12]
                         disabled:opacity-60 disabled:cursor-not-allowed
                         text-white font-semibold text-[13px] rounded-md
                         flex items-center justify-center gap-2"
            >
              {loading ? (
                <>
                  <Loader2 size={15} className="animate-spin" />
                  Signing in…
                </>
              ) : (
                <>
                  <ShieldCheck size={15} />
                  Sign In
                </>
              )}
            </button>
          </form>
        </div>

        <div className="px-7 pb-5">
          <p className="text-center text-[11px] text-zinc-600">DBX · single-node v1</p>
        </div>
      </div>
    </div>
  );
}
