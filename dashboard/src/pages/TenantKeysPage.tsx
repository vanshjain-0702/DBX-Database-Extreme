import { type FormEvent, useEffect, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { fetchWithAuth } from '../api';
import PageChrome from '../components/PageChrome';

type TenantKey = {
  id: string;
  name: string;
  role: string;
  key_patterns: string[];
  revoked: boolean;
  created_at: string;
};

export default function TenantKeysPage({ clusterId }: { clusterId: string }) {
  const [keys, setKeys] = useState<TenantKey[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [name, setName] = useState('agent-writer');
  const [role, setRole] = useState('writer');
  const [secret, setSecret] = useState<string | null>(null);
  const [keyId, setKeyId] = useState<string | null>(null);

  const load = async () => {
    try {
      const res = await fetchWithAuth(`/api/v1/tenants/${clusterId}/keys`);
      if (!res.ok) {
        setError(`HTTP ${res.status}`);
        return;
      }
      const data = await res.json();
      setKeys(Array.isArray(data) ? data : []);
      setError(null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to list keys');
    }
  };

  useEffect(() => {
    load();
  }, [clusterId]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      const res = await fetchWithAuth(`/api/v1/tenants/${clusterId}/keys`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, role }),
      });
      if (!res.ok) {
        setError(await res.text());
        return;
      }
      const data = await res.json();
      setSecret(data.secret);
      setKeyId(data.key?.id || null);
      setName('agent-writer');
      await load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to mint key');
    }
  };

  const handleRevoke = async (id: string) => {
    const res = await fetchWithAuth(`/api/v1/tenants/${clusterId}/keys/${id}`, { method: 'DELETE' });
    if (!res.ok && res.status !== 204) {
      setError(await res.text());
      return;
    }
    await load();
  };

  const live = keys.filter(k => !k.revoked);

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Tenant keys"
        purpose="Mint a reader, writer, or tenant-admin key for AUTH on :6380. The secret is shown once."
        extra={
          <button type="button" className="btn-primary" onClick={() => { setSecret(null); setKeyId(null); setShowModal(true); }}>
            <Plus size={14} /> Mint key
          </button>
        }
      />

      {error && <div className="alert-error">{error}</div>}

      {showModal && (
        <div className="modal-overlay" onClick={() => setShowModal(false)}>
          <div className="modal-content p-5" onClick={e => e.stopPropagation()}>
            <h3 className="text-[16px] font-semibold mb-4">Mint tenant key</h3>
            {!secret ? (
              <form onSubmit={handleCreate}>
                <label className="block mb-1.5">Name</label>
                <input className="input-field mb-4" value={name} onChange={e => setName(e.target.value)} required />
                <label className="block mb-1.5">Role</label>
                <select className="input-field mb-5" value={role} onChange={e => setRole(e.target.value)}>
                  <option value="reader">reader — GET, VSEARCH</option>
                  <option value="writer">writer — strings + VADD</option>
                  <option value="tenant-admin">tenant-admin — compact / admin</option>
                </select>
                <div className="flex justify-end gap-2">
                  <button type="button" className="btn-secondary" onClick={() => setShowModal(false)}>Cancel</button>
                  <button type="submit" className="btn-primary">Mint</button>
                </div>
              </form>
            ) : (
              <div>
                <p className="text-[13px] text-[var(--warning)] mb-3">Copy AUTH now. The secret is not stored in plaintext.</p>
                <p className="text-[12px] text-[var(--text-muted)] mb-1">Identity</p>
                <div className="bg-zinc-950 text-emerald-400 font-mono p-3 rounded-md break-all mb-3 text-[13px] select-all">
                  {clusterId}:{keyId}
                </div>
                <p className="text-[12px] text-[var(--text-muted)] mb-1">Secret</p>
                <div className="bg-zinc-950 text-emerald-400 font-mono p-3 rounded-md break-all mb-4 text-[13px] select-all">
                  {secret}
                </div>
                <button
                  type="button"
                  className="btn-primary w-full justify-center"
                  onClick={() => { setShowModal(false); setSecret(null); setKeyId(null); }}
                >
                  I have copied the secret
                </button>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="panel overflow-hidden mt-4">
        <table className="w-full text-left text-[13px]">
          <thead className="bg-[var(--bg-tertiary)] border-b border-[var(--border-color)]">
            <tr>
              <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)]">Name</th>
              <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)]">Role</th>
              <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)]">Key id</th>
              <th className="px-4 py-2.5 font-semibold text-[11px] uppercase tracking-wider text-[var(--text-muted)] text-right"> </th>
            </tr>
          </thead>
          <tbody>
            {live.length === 0 ? (
              <tr><td colSpan={4} className="px-4 py-10 text-center text-[var(--text-muted)]">No live keys. Mint a writer for AUTH on :6380.</td></tr>
            ) : (
              live.map(k => (
                <tr key={k.id} className="border-t border-[var(--border-color)]">
                  <td className="px-4 py-2.5 font-medium">{k.name}</td>
                  <td className="px-4 py-2.5 font-mono">{k.role}</td>
                  <td className="px-4 py-2.5 font-mono text-[var(--text-muted)]">{k.id}</td>
                  <td className="px-4 py-2.5 text-right">
                    <button type="button" onClick={() => handleRevoke(k.id)} className="p-1.5 text-[var(--text-muted)] hover:text-[var(--error)]" title="Revoke">
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
  );
}
