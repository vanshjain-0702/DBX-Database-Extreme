import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { api, json, message, type Matter } from '../lib/api';
import { useSession } from '../lib/session';
import { ErrorNotice } from './Feedback';
export default function ProcessingConsent({matter, refreshMatter}: {matter: Matter; refreshMatter: () => Promise<Matter>}) {
    const {user, capabilities} = useSession();
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    async function toggle(enabled: boolean) {
        if (user?.role !== 'admin' || matter.hibernated) return;
        setBusy(true); setError('');
        try { await api(`/api/matters/${matter.id}/processing`, {method: 'PUT', body: json({enabled})}); await refreshMatter(); }
        catch (error) { setError(message(error)); } finally { setBusy(false); }
    }
    return <details className="processing-consent"><summary><ShieldCheck size={16}/><span>External processing</span><span className={`badge ${matter.external_processing ? 'medium' : 'neutral'}`}>{matter.external_processing ? 'Allowed for this matter' : 'Off'}</span></summary><div className="consent-body"><p>Semantic review, translation, OCR and dictation may send selected matter text or audio to configured external providers. Enable only when your firm's confidentiality requirements allow it.</p><label className="check-label"><input type="checkbox" checked={!!matter.external_processing} disabled={busy || user?.role !== 'admin' || matter.hibernated} onChange={event => void toggle(event.target.checked)}/>Allow configured providers to process this matter</label>{user?.role !== 'admin' && <p className="field-help">A firm administrator manages permission for this matter.</p>}{!capabilities && <p className="field-help">Provider availability could not be checked. Review Firm & connection before processing.</p>}<ErrorNotice error={error}/></div></details>;
}
