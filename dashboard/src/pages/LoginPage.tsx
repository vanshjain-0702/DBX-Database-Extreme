import type { FormEvent } from 'react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Lock, User, Loader2, AlertCircle, ShieldCheck, Database } from 'lucide-react';
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
    <div
      className="min-h-screen w-full flex flex-col items-center justify-center px-4 py-8 relative overflow-hidden"
      style={{ background: '#080810' }}
    >
      {/* Animated background gradient */}
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background: 'radial-gradient(ellipse 80% 70% at 50% -20%, rgba(194,65,12,0.18) 0%, transparent 65%)',
        }}
      />
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background: 'radial-gradient(ellipse 50% 50% at 80% 80%, rgba(15,10,40,0.6) 0%, transparent 70%)',
        }}
      />

      {/* Subtle grid overlay */}
      <div
        className="absolute inset-0 pointer-events-none opacity-[0.025]"
        style={{
          backgroundImage: 'linear-gradient(rgba(255,255,255,0.5) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.5) 1px, transparent 1px)',
          backgroundSize: '40px 40px',
        }}
      />

      {/* Brand above card */}
      <div
        className="flex items-center gap-3 mb-8 z-10"
        style={{ animation: 'fade-in-up 0.4s ease both' }}
      >
        <div
          className="w-9 h-9 rounded-xl overflow-hidden border flex-shrink-0"
          style={{
            borderColor: 'rgba(194,65,12,0.4)',
            boxShadow: '0 0 20px rgba(194,65,12,0.3)',
          }}
        >
          <img src={logo} alt="DBX" className="w-full h-full object-cover" />
        </div>
        <div>
          <div className="text-white font-bold text-[18px] tracking-tight leading-none">DBX</div>
          <div className="text-[11px] text-[rgba(255,255,255,0.4)] font-medium tracking-[0.12em] uppercase mt-0.5">
            Database Extreme
          </div>
        </div>
      </div>

      {/* Card */}
      <div
        className="relative w-full max-w-[420px] z-10 rounded-2xl overflow-hidden"
        style={{
          border: '1px solid rgba(255,255,255,0.08)',
          background: 'rgba(15, 15, 26, 0.90)',
          backdropFilter: 'blur(20px)',
          WebkitBackdropFilter: 'blur(20px)',
          boxShadow: '0 32px 80px rgba(0,0,0,0.7), 0 0 0 1px rgba(194,65,12,0.15), 0 -1px 0 rgba(255,255,255,0.05)',
          animation: 'scale-in 0.3s cubic-bezier(0.4,0,0.2,1) both',
        }}
      >
        {/* Accent top bar */}
        <div
          className="h-[2px] w-full"
          style={{
            background: 'linear-gradient(90deg, #c2410c 0%, #ea580c 50%, rgba(234,88,12,0.3) 100%)',
          }}
        />

        <div className="p-8">
          {/* Heading */}
          <div className="mb-7">
            <h1 className="text-[22px] font-bold text-white tracking-tight leading-tight">
              Control plane
            </h1>
            <p className="text-[14px] text-[rgba(255,255,255,0.45)] mt-1.5 leading-relaxed">
              Sign in to manage your per-tenant engines.
            </p>
          </div>

          {/* Error */}
          {error && (
            <div
              className="flex items-start gap-2.5 rounded-xl p-3.5 mb-5 text-[13px] font-medium"
              style={{
                background: 'rgba(225,29,72,0.12)',
                border: '1px solid rgba(225,29,72,0.25)',
                color: '#fb7185',
                animation: 'fade-in 0.2s ease both',
              }}
            >
              <AlertCircle size={15} className="mt-0.5 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {/* Form */}
          <form onSubmit={handleLogin} className="flex flex-col gap-5">
            <div>
              <label
                htmlFor="login-username"
                className="block text-[11px] font-semibold uppercase tracking-wider mb-1.5"
                style={{ color: 'rgba(255,255,255,0.4)' }}
              >
                Username
              </label>
              <div className="relative">
                <span
                  className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5"
                  style={{ color: 'rgba(255,255,255,0.25)' }}
                >
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
                  className="w-full pl-10 pr-3.5 py-2.5 text-[13px] font-medium outline-none rounded-xl transition-all duration-200 disabled:opacity-50"
                  style={{
                    background: 'rgba(255,255,255,0.05)',
                    border: '1px solid rgba(255,255,255,0.08)',
                    color: 'rgba(255,255,255,0.9)',
                  }}
                  onFocus={e => {
                    e.currentTarget.style.borderColor = 'rgba(194,65,12,0.6)';
                    e.currentTarget.style.boxShadow = '0 0 0 3px rgba(194,65,12,0.15)';
                    e.currentTarget.style.background = 'rgba(255,255,255,0.07)';
                  }}
                  onBlur={e => {
                    e.currentTarget.style.borderColor = 'rgba(255,255,255,0.08)';
                    e.currentTarget.style.boxShadow = 'none';
                    e.currentTarget.style.background = 'rgba(255,255,255,0.05)';
                  }}
                />
              </div>
            </div>

            <div>
              <label
                htmlFor="login-password"
                className="block text-[11px] font-semibold uppercase tracking-wider mb-1.5"
                style={{ color: 'rgba(255,255,255,0.4)' }}
              >
                Password
              </label>
              <div className="relative">
                <span
                  className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5"
                  style={{ color: 'rgba(255,255,255,0.25)' }}
                >
                  <Lock size={15} />
                </span>
                <input
                  id="login-password"
                  type="password"
                  placeholder="••••••••••••"
                  value={password}
                  autoComplete="current-password"
                  onChange={e => setPassword(e.target.value)}
                  disabled={loading}
                  className="w-full pl-10 pr-3.5 py-2.5 text-[13px] font-medium outline-none rounded-xl transition-all duration-200 disabled:opacity-50"
                  style={{
                    background: 'rgba(255,255,255,0.05)',
                    border: '1px solid rgba(255,255,255,0.08)',
                    color: 'rgba(255,255,255,0.9)',
                  }}
                  onFocus={e => {
                    e.currentTarget.style.borderColor = 'rgba(194,65,12,0.6)';
                    e.currentTarget.style.boxShadow = '0 0 0 3px rgba(194,65,12,0.15)';
                    e.currentTarget.style.background = 'rgba(255,255,255,0.07)';
                  }}
                  onBlur={e => {
                    e.currentTarget.style.borderColor = 'rgba(255,255,255,0.08)';
                    e.currentTarget.style.boxShadow = 'none';
                    e.currentTarget.style.background = 'rgba(255,255,255,0.05)';
                  }}
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={loading}
              id="login-submit-btn"
              className="w-full mt-1 py-3 text-white font-bold text-[14px] rounded-xl flex items-center justify-center gap-2.5 transition-all duration-200 relative overflow-hidden disabled:opacity-60 disabled:cursor-not-allowed"
              style={{
                background: 'linear-gradient(135deg, #c2410c 0%, #ea580c 100%)',
                boxShadow: loading ? 'none' : '0 4px 20px rgba(194,65,12,0.4), 0 1px 4px rgba(194,65,12,0.2)',
              }}
              onMouseEnter={e => {
                if (!loading) {
                  (e.currentTarget as HTMLElement).style.boxShadow = '0 8px 28px rgba(194,65,12,0.55), 0 2px 8px rgba(194,65,12,0.3)';
                  (e.currentTarget as HTMLElement).style.transform = 'translateY(-1px)';
                }
              }}
              onMouseLeave={e => {
                (e.currentTarget as HTMLElement).style.boxShadow = '0 4px 20px rgba(194,65,12,0.4), 0 1px 4px rgba(194,65,12,0.2)';
                (e.currentTarget as HTMLElement).style.transform = 'translateY(0)';
              }}
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
        <div
          className="px-8 py-4 border-t flex items-center justify-between"
          style={{ borderColor: 'rgba(255,255,255,0.06)', background: 'rgba(0,0,0,0.2)' }}
        >
          <div className="flex items-center gap-2 text-[11px]" style={{ color: 'rgba(255,255,255,0.25)' }}>
            <Database size={11} />
            <span>DBX v1.2.0 — per-tenant memory engine</span>
          </div>
          <div
            className="text-[10px] font-mono px-2 py-0.5 rounded"
            style={{
              background: 'rgba(194,65,12,0.15)',
              color: 'rgba(194,65,12,0.8)',
              border: '1px solid rgba(194,65,12,0.2)',
            }}
          >
            BSL 1.1
          </div>
        </div>
      </div>

      {/* Footer hint */}
      <p
        className="mt-6 text-[12px] z-10"
        style={{
          color: 'rgba(255,255,255,0.2)',
          animation: 'fade-in 0.6s ease 0.3s both',
        }}
      >
        Isolated per-tenant memory · self-hosted · no cloud dependency
      </p>
    </div>
  );
}
