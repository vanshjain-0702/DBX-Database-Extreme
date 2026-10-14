import { useEffect, useState, type FormEvent } from 'react';
import { Landmark, Plus, RefreshCw, ArrowUpRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import { api, json, message, dateTime } from '../lib/api';
import { useSession } from '../lib/session';
import { Loading, ErrorNotice, EmptyState } from '../components/Feedback';
interface Watch {id: string; name: string; source_url: string; jurisdiction: string; effective_date?: string; versions?: {sha256: string; fetched_at: number; url: string}[]; source_sha256?: string; checked_at?: number; changed_at?: number; error?: string; keywords?: string[]}
interface Alert {id: string; matter_id?: string; title?: string; source_url?: string; effective_date?: string; excerpt: string; status: string; note?: string; created_at?: number}
function AlertReview({alert, editable, busy, onSave}: {alert: Alert; editable: boolean; busy: boolean; onSave: (id: string, status: string, note: string) => Promise<void>}) {
    const [status, setStatus] = useState(alert.status);
    const [note, setNote] = useState(alert.note || '');
    useEffect(() => {setStatus(alert.status); setNote(alert.note || '');}, [alert.status, alert.note]);
    return <article className="regulation-alert"><div className="section-heading"><h3>{alert.title || 'Source change for review'}</h3><span className={`badge ${alert.status === 'open' ? 'medium' : 'clear'}`}>{alert.status}</span></div>{alert.effective_date && <p className="field-help">Reported effective date: {alert.effective_date}</p>}<blockquote>{alert.excerpt || 'The registered source changed. Open the source and assess any impact.'}</blockquote><div className="inline-actions">{alert.source_url && <a className="text-button" href={alert.source_url} target="_blank" rel="noopener noreferrer">Authoritative source<ArrowUpRight size={14}/></a>}{alert.matter_id && <Link className="text-button" to={`/matters/${alert.matter_id}/dashboard`}>Review candidate matter<ArrowUpRight size={14}/></Link>}</div>
        {editable ? <form className="form-stack" onSubmit={event => {event.preventDefault(); void onSave(alert.id, status, note.trim());}}><div className="form-grid"><div><label htmlFor={`alert-status-${alert.id}`}>Review state</label><select id={`alert-status-${alert.id}`} value={status} onChange={event => setStatus(event.target.value)}><option value="open">Open</option><option value="reviewed">Reviewed</option><option value="dismissed">Dismissed</option></select></div><div><label htmlFor={`alert-note-${alert.id}`}>Rationale{status !== 'open' ? ' (required)' : ''}</label><textarea id={`alert-note-${alert.id}`} rows={2} maxLength={10000} value={note} onChange={event => setNote(event.target.value)} required={status !== 'open'}/></div></div><button className="button subtle" disabled={busy || status !== 'open' && !note.trim()}>Save review state</button></form> : alert.note && <p className="field-help">Review rationale: {alert.note}</p>}
    </article>;
}
export default function RegulationsPage() {
    const {user} = useSession();
    const editable = user?.role !== 'viewer';
    const [watches, setWatches] = useState<Watch[]>([]);
    const [alerts, setAlerts] = useState<Alert[]>([]);
    const [hosts, setHosts] = useState<string[]>([]);
    const [loading, setLoading] = useState(true);
    const [loaded, setLoaded] = useState(false);
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [showCreate, setShowCreate] = useState(false);
    const [filter, setFilter] = useState('open');
    const [name, setName] = useState('');
    const [url, setUrl] = useState('');
    const [jurisdiction, setJurisdiction] = useState('');
    const [effective, setEffective] = useState('');
    const [keywords, setKeywords] = useState('');
    async function load() {
        setLoading(true); setError('');
        try { const data = await api<{watches: Watch[]; alerts: Alert[]; supported_hosts?: string[]}>('/api/regulations'); setWatches(data.watches); setAlerts(data.alerts); setHosts(data.supported_hosts || []); setLoaded(true); }
        catch (error) { setError(message(error)); } finally { setLoading(false); }
    }
    useEffect(() => {void load();}, []);
    async function register(event: FormEvent) {
        event.preventDefault(); setBusy('create'); setError(''); setNotice('');
        try { if (!keywords.split(',').some(term => term.trim())) throw new Error('Enter at least one matter keyword.'); await api('/api/regulations', {method: 'POST', body: json({name: name.trim(), source_url: url.trim(), jurisdiction: jurisdiction.trim(), effective_date: effective || null, keywords: keywords.split(',').map(term => term.trim()).filter(Boolean)})}); await load(); setShowCreate(false); setName(''); setUrl(''); setJurisdiction(''); setEffective(''); setKeywords(''); setNotice('Authoritative source watch registered. Source changes create possible impact tasks for reviewer assessment.'); }
        catch (error) {setError(message(error));} finally {setBusy('');}
    }
    async function check(id: string) {
        setBusy(id); setError(''); setNotice('');
        try { await api(`/api/regulations/${id}/check`, {method: 'POST'}); await load(); setNotice('Source refresh completed. Review any changes against the source.'); }
        catch (error) {setError(message(error));} finally {setBusy('');}
    }
    async function save(id: string, status: string, note: string) {
        setBusy(id); setError('');
        try { await api(`/api/regulations/alerts/${id}`, {method: 'PATCH', body: json({status, note})}); await load(); setNotice('Regulatory review state recorded.'); }
        catch (error) {setError(message(error));} finally {setBusy('');}
    }
    const shown = alerts.filter(alert => filter === 'all' || alert.status === filter);
    return <div className="page-enter"><div className="page-heading"><div><p className="eyebrow">AUTHORITATIVE SOURCE WATCHES</p><h1>Regulations</h1><p className="muted">Track source changes and assess possible impact on your matters.</p></div>{editable && <button className="button primary" onClick={() => setShowCreate(!showCreate)}><Plus size={16}/>Register source</button>}</div><ErrorNotice error={error} retry={!loaded ? () => void load() : undefined}/>{notice && <p className="notice success" role="status">{notice}</p>}<p className="notice">Alerts are review tasks. Matter tags and linked references identify candidate scope; counsel determines applicability and required action.</p>
        {showCreate && <section className="surface regulation-create"><div className="section-heading"><h2>Register an authoritative source</h2><Landmark size={22} className="brass"/></div><form className="form-stack" onSubmit={register}><div className="form-grid"><div><label htmlFor="regulation-name">Watch name</label><input id="regulation-name" required maxLength={200} value={name} onChange={event => setName(event.target.value)}/></div><div><label htmlFor="regulation-jurisdiction">Jurisdiction</label><input id="regulation-jurisdiction" required maxLength={200} value={jurisdiction} onChange={event => setJurisdiction(event.target.value)}/></div></div><div><label htmlFor="regulation-url">Authoritative HTTPS source</label><input id="regulation-url" required type="url" pattern="https://.*" maxLength={2000} value={url} onChange={event => setUrl(event.target.value)}/>{hosts.length > 0 && <p className="field-help">Supported source hosts: {hosts.join(', ')}</p>}</div><div className="form-grid"><div><label htmlFor="regulation-effective">Effective date (if published)</label><input id="regulation-effective" type="date" value={effective} onChange={event => setEffective(event.target.value)}/></div><div><label htmlFor="regulation-keywords">Matter tags / keywords</label><input id="regulation-keywords" required maxLength={3000} value={keywords} onChange={event => setKeywords(event.target.value)} placeholder="privacy, data processing"/><p className="field-help">Enter at least one term. Separate terms with commas.</p></div></div><button className="button primary" disabled={!!busy}>Register source watch</button></form></section>}
        {loading ? <Loading text="Loading source watches…"/> : loaded && <div className="regulations-layout"><section className="surface source-watches"><div className="section-heading"><h2>Registered sources</h2><span className="mini-label">{watches.length} WATCHES</span></div>{watches.length ? watches.map(watch => <article className="watch-row" key={watch.id}><h3>{watch.name}</h3><p className="field-help">{watch.jurisdiction}{watch.effective_date ? ` · Effective ${watch.effective_date}` : ''}</p><a className="text-button source-link" href={watch.source_url} target="_blank" rel="noopener noreferrer">Open authoritative source<ArrowUpRight size={14}/></a><dl><div><dt>Last checked</dt><dd>{watch.checked_at ? dateTime(watch.checked_at) : 'Not checked yet'}</dd></div><div><dt>Source version</dt><dd className="mono">{watch.source_sha256 ? `${watch.source_sha256.slice(0,12)} / ${watch.versions?.length || 1} snapshot${watch.versions?.length === 1 ? '' : 's'}` : 'Awaiting first check'}</dd></div></dl>{watch.error && <p className="notice error">{watch.error}</p>}{editable && <button className="button subtle" disabled={!!busy} onClick={() => check(watch.id)}><RefreshCw size={14}/>{busy === watch.id ? 'Checking…' : 'Refresh source'}</button>}</article>) : <EmptyState title="No sources registered" text="Add an authoritative publication to start a source watch."/>}</section><section className="surface regulation-alerts"><div className="section-heading"><h2>Possible impact reviews</h2><div><label htmlFor="regulation-filter" className="sr-only">Alert state</label><select id="regulation-filter" value={filter} onChange={event => setFilter(event.target.value)}><option value="open">Open tasks</option><option value="all">All states</option><option value="reviewed">Reviewed</option><option value="dismissed">Dismissed</option></select></div></div>{shown.length ? shown.map(alert => <AlertReview key={alert.id} alert={alert} editable={editable} busy={!!busy} onSave={save}/>) : <EmptyState title="No matching review tasks" text="Source changes will appear here for counsel to assess."/>}</section></div>}
    </div>;
}
