import type { FormEvent } from 'react';
import { useState, useEffect } from 'react';
import { ShieldAlert, CheckCircle2, Shield, Lock, Key, Layout, Plus, Trash2, AlertTriangle } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { useTheme } from '../components/ThemeProvider';
import { useLocation, useNavigate } from 'react-router-dom';

const TZ_KEY = 'dbx-timezone';

type ApiKeyRow = {
  id: string;
  name: string;
  prefix: string;
  created_at: string;
};

export default function SettingsPage() {
  const { theme, setTheme } = useTheme();
  const location = useLocation();
  const navigate = useNavigate();

  const getTabFromPath = () => {
    const p = location.pathname;
    if (p.includes('security')) return 'security';
    if (p.includes('keys')) return 'keys';
    if (p.includes('replication')) return 'replication';
    return 'general';
  };

  const [activeTab, setActiveTab] = useState(getTabFromPath());

  useEffect(() => {
    setActiveTab(getTabFromPath());
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname]);

  const onTabClick = (tab: string) => {
    setActiveTab(tab);
    if (tab === 'security') navigate('/settings/security');
    else if (tab === 'keys') navigate('/settings/keys');
    else if (tab === 'replication') navigate('/settings/replication');
    else navigate('/settings');
  };

  const tabs = [
    { id: 'general', name: 'General', icon: <Layout size={14} /> },
    { id: 'security', name: 'Security', icon: <Shield size={14} /> },
    { id: 'keys', name: 'API keys', icon: <Key size={14} /> },
    { id: 'replication', name: 'Replication', icon: <AlertTriangle size={14} /> },
  ];

  const [apiKeys, setApiKeys] = useState<ApiKeyRow[]>([]);
  const [newKeyName, setNewKeyName] = useState('');
  const [showKeyModal, setShowKeyModal] = useState(false);
  const [generatedKey, setGeneratedKey] = useState<string | null>(null);

  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [timezone, setTimezone] = useState(() => localStorage.getItem(TZ_KEY) || 'UTC');

  const persistTimezone = (v: string) => {
    setTimezone(v);
    localStorage.setItem(TZ_KEY, v);
  };

  const loadApiKeys = async () => {
    try {
      const res = await fetchWithAuth('/api/admin/keys');
      if (res.ok) {
        const data = await res.json();
        setApiKeys(data || []);
      }
    } catch (e) { console.error(e); }
  };

  useEffect(() => {
    if (activeTab === 'keys') loadApiKeys();
  }, [activeTab]);

  const handleCreateKey = async (e: FormEvent) => {
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
    } catch (e) { console.error(e); }
  };

  const handleRevokeKey = async (id: string) => {
    try {
      const res = await fetchWithAuth('/api/admin/keys/revoke', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id })
      });
      if (res.ok) loadApiKeys();
    } catch (e) { console.error(e); }
  };

  const handlePasswordChange = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(null);
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match.'); return;
    }
    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters long.'); return;
    }
    setLoading(true);
    try {
      const res = await fetchWithAuth('/api/admin/password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
      });
      if (!res.ok) throw new Error('Failed to update password');
      setSuccess('Password updated. Use it on the next login.');
      setOldPassword(''); setNewPassword(''); setConfirmPassword('');
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'An error occurred');
    } finally {
      setLoading(false);
    }
  };

  const titles: Record<string, { h: string; p: string }> = {
    general: { h: 'General', p: 'Display theme and timezone for this operator session.' },
    security: { h: 'Security', p: 'Control-plane credentials. TLS is configured outside this dashboard.' },
    keys: { h: 'API keys', p: 'Programmatic access to the orchestrator.' },
    replication: { h: 'Replication', p: 'Data-plane Raft status for this release.' },
  };

  return (
    <div className="flex h-full w-full bg-[var(--bg-primary)]">
      <div className="w-52 border-r border-[var(--border-color)] bg-[var(--bg-sidebar)] p-3 hidden md:block">
        <div className="px-2 py-2 text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">Settings</div>
        <nav className="space-y-0.5">
          {tabs.map(tab => {
            const isActive = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => onTabClick(tab.id)}
                className={`w-full flex items-center gap-2 px-2.5 py-1.5 text-[13px] font-medium border-l-2 ${
                  isActive
                    ? 'border-[var(--accent-primary)] bg-[var(--accent-soft)] text-[var(--accent-primary)]'
                    : 'border-transparent text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)]'
                }`}
              >
                {tab.icon}
                {tab.name}
              </button>
            );
          })}
        </nav>
      </div>

      <div className="flex-1 overflow-y-auto p-7">
        <div className="max-w-3xl">
          <div className="mb-6">
            <h3 className="text-[20px] font-semibold tracking-tight">{titles[activeTab].h}</h3>
            <p className="text-[13px] text-[var(--text-muted)] mt-1">{titles[activeTab].p}</p>
          </div>

          {activeTab === 'general' && (
            <div className="panel p-5 space-y-5">
              <div>
                <label className="block mb-1.5">Display theme</label>
                <select
                  className="input-field max-w-sm"
                  value={theme}
                  onChange={e => setTheme(e.target.value as 'light' | 'dark' | 'system')}
                >
                  <option value="light">Light</option>
                  <option value="dark">Dark</option>
                  <option value="system">System</option>
                </select>
                <p className="text-[12px] text-[var(--text-muted)] mt-1.5">Stored in this browser. Light is the default.</p>
              </div>
              <div>
                <label className="block mb-1.5">Timezone</label>
                <select
                  className="input-field max-w-sm"
                  value={timezone}
                  onChange={e => persistTimezone(e.target.value)}
                >
                  <option value="UTC">UTC</option>
                  <option value="America/New_York">America/New_York</option>
                  <option value="America/Los_Angeles">America/Los_Angeles</option>
                  <option value="Europe/London">Europe/London</option>
                  <option value="Asia/Kolkata">Asia/Kolkata</option>
                </select>
                <p className="text-[12px] text-[var(--text-muted)] mt-1.5">Persisted locally as {timezone}.</p>
              </div>
            </div>
          )}

          {activeTab === 'security' && (
            <div className="space-y-4">
              <div className="panel p-5">
                <h4 className="text-[14px] font-semibold mb-1">TLS</h4>
                <p className="text-[13px] text-[var(--text-secondary)] leading-relaxed">
                  TLS terminates at the control-plane reverse proxy or orchestrator bind. This dashboard does not issue certificates or persist TLS settings. Configure certificates in orchestrator / ingress config — not here.
                </p>
              </div>

              <div className="panel overflow-hidden">
                <div className="panel-header">
                  <div className="panel-title"><Lock size={14} /> Administrator password</div>
                </div>
                <div className="p-5">
                  {error && (
                    <div className="alert-error mb-4">
                      <ShieldAlert size={14} /> {error}
                    </div>
                  )}
                  {success && (
                    <div className="flex items-center gap-2 p-3 rounded-md text-[13px] mb-4 bg-emerald-50 dark:bg-emerald-950/30 text-[var(--success)] border border-emerald-200 dark:border-emerald-900">
                      <CheckCircle2 size={14} /> {success}
                    </div>
                  )}

                  <form onSubmit={handlePasswordChange} className="space-y-4 max-w-xl">
                    <div>
                      <label className="block mb-1.5">Current password</label>
                      <input
                        type="password"
                        className="input-field"
                        value={oldPassword}
                        onChange={e => setOldPassword(e.target.value)}
                        required
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="block mb-1.5">New password</label>
                        <input
                          type="password"
                          className="input-field"
                          value={newPassword}
                          onChange={e => setNewPassword(e.target.value)}
                          required
                        />
                      </div>
                      <div>
                        <label className="block mb-1.5">Confirm</label>
                        <input
                          type="password"
                          className="input-field"
                          value={confirmPassword}
                          onChange={e => setConfirmPassword(e.target.value)}
                          required
                        />
                      </div>
                    </div>
                    <button type="submit" className="btn-primary" disabled={loading}>
                      {loading ? 'Updating…' : 'Update password'}
                    </button>
                  </form>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'keys' && (
            <div className="space-y-4">
              {showKeyModal && (
                <div className="modal-overlay" onClick={() => setShowKeyModal(false)}>
                  <div className="modal-content p-5" onClick={e => e.stopPropagation()}>
                    <h3 className="text-[16px] font-semibold mb-4">Create API key</h3>
                    {!generatedKey ? (
                      <form onSubmit={handleCreateKey}>
                        <label className="block mb-1.5">Name</label>
                        <input autoFocus className="input-field mb-5" value={newKeyName} onChange={e => setNewKeyName(e.target.value)} />
                        <div className="flex justify-end gap-2">
                          <button type="button" onClick={() => setShowKeyModal(false)} className="btn-secondary">Cancel</button>
                          <button type="submit" className="btn-primary">Generate</button>
                        </div>
                      </form>
                    ) : (
                      <div>
                        <p className="text-[13px] text-[var(--warning)] mb-3">Copy this key now. It will not be shown again.</p>
                        <div className="bg-zinc-950 text-emerald-400 font-mono p-3 rounded-md break-all mb-4 text-[13px] select-all">
                          {generatedKey}
                        </div>
                        <button type="button" onClick={() => { setShowKeyModal(false); setGeneratedKey(null); }} className="btn-primary w-full justify-center">
                          I have copied the key
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              )}

              <div className="panel p-4 flex items-center justify-between gap-4">
                <p className="text-[13px] text-[var(--text-secondary)]">
                  Keys authenticate orchestrator API calls. Do not embed them in client-side apps.
                </p>
                <button type="button" onClick={() => setShowKeyModal(true)} className="btn-primary shrink-0">
                  <Plus size={14} /> Generate
                </button>
              </div>

              <div className="panel overflow-hidden">
                <table className="w-full text-left text-[13px]">
                  <thead className="bg-[var(--bg-tertiary)] border-b border-[var(--border-color)]">
                    <tr>
                      <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)]">Name</th>
                      <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)]">Prefix</th>
                      <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)]">Created</th>
                      <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)] text-right"> </th>
                    </tr>
                  </thead>
                  <tbody>
                    {apiKeys.length === 0 ? (
                      <tr><td colSpan={4} className="px-4 py-10 text-center text-[var(--text-muted)]">No API keys yet.</td></tr>
                    ) : (
                      apiKeys.map(k => (
                        <tr key={k.id} className="border-t border-[var(--border-color)]">
                          <td className="px-4 py-2.5 font-medium">{k.name}</td>
                          <td className="px-4 py-2.5 font-mono text-[var(--text-muted)]">{k.prefix}••••</td>
                          <td className="px-4 py-2.5 text-[var(--text-muted)]">{new Date(k.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}</td>
                          <td className="px-4 py-2.5 text-right">
                            <button type="button" onClick={() => handleRevokeKey(k.id)} className="p-1.5 text-[var(--text-muted)] hover:text-[var(--error)]">
                              <Trash2 size={14} />
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

          {activeTab === 'replication' && (
            <div className="panel p-5">
              <div className="flex items-start gap-3">
                <div className="mt-0.5 text-[var(--warning)]"><AlertTriangle size={18} /></div>
                <div>
                  <h4 className="text-[14px] font-semibold mb-1">Data-plane Raft is disabled</h4>
                  <p className="text-[13px] text-[var(--text-secondary)] leading-relaxed">
                    v1 runs a single-node engine per tenant. Consensus, replica add/remove, and a cluster builder are not available. Fail closed: do not assume multi-node durability from this control plane.
                  </p>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
