import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Download, FileText, RefreshCw } from 'lucide-react';
import { api, downloadExport, message, dateTime, type ExportRecord } from '../lib/api';
import { Loading, ErrorNotice, EmptyState } from '../components/Feedback';
const exportLabels: Record<string, string> = { memorandum: 'Review memorandum', offboard: 'Offboarding memorandum', 'billing-review': 'Pre-bill review', 'clause-proposals': 'Accepted clause comparisons' };
export default function ExportsPage() {
    const [exports, setExports] = useState<ExportRecord[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [downloading, setDownloading] = useState('');
    async function load() { setLoading(true); setError(''); try {
        setExports((await api<{
            exports: ExportRecord[];
        }>('/api/exports')).exports);
    }
    catch (error) {
        setError(message(error));
    }
    finally {
        setLoading(false);
    } }
    useEffect(() => { load(); }, []);
    async function download(id: string) { setDownloading(id); setError(''); try {
        await downloadExport(id);
    }
    catch (error) {
        setError(message(error));
    }
    finally {
        setDownloading('');
    } }
    return <div className="page-enter"><div className="page-heading"><div><p className="eyebrow">THE REVIEW RECORD</p><h1>Exports</h1><p className="muted">Saved review reports and clause comparisons, ready to share as Markdown.</p></div><button className="button subtle" onClick={load} disabled={loading}><RefreshCw size={16}/>Refresh</button></div><ErrorNotice error={error}/>
    {loading ? <Loading text="Loading exports…"/> : exports.length ? <section className="surface export-list">{exports.map(row => <article key={row.id}><div className="matter-icon"><FileText size={21}/></div><div><h2>{row.client_name}</h2><p>{row.matter || row.kind.split('_').join(' ')} · {dateTime(row.created_at)}</p><span className="mini-label">{exportLabels[row.kind] || row.kind} · MARKDOWN</span></div><button className="button subtle" onClick={() => download(row.id)} disabled={!!downloading}><Download size={16}/>{downloading === row.id ? 'Downloading…' : 'Download'}</button></article>)}</section> : !error && <section className="surface"><EmptyState title="Your review record will appear here" text="Export a matter's memorandum, accepted clause comparisons or pre-bill review."><Link className="button primary" to="/matters">Open matters</Link></EmptyState></section>}
    <p className="page-footnote">Exports are saved snapshots. Generate a new report after changes to the review.</p>
  </div>;
}
