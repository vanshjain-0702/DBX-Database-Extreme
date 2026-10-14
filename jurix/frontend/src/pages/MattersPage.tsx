import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Plus, ArrowUpRight, Search, X, ArrowDownUp, RefreshCw } from 'lucide-react';
import { api, json, message, dateTime, type Matter } from '../lib/api';
import { useSession } from '../lib/session';
import { Loading, ErrorNotice, EmptyState } from '../components/Feedback';
type Sort = 'matter' | 'reviewer' | 'documents' | 'review' | 'updated';
export default function MattersPage() {
    const {user} = useSession();
    const [matters, setMatters] = useState<Matter[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [query, setQuery] = useState('');
    const [filter, setFilter] = useState('all');
    const [sort, setSort] = useState<Sort>('updated');
    const [descending, setDescending] = useState(true);
    const [showCreate, setShowCreate] = useState(false);
    const [client, setClient] = useState('');
    const [name, setName] = useState('');
    const [creating, setCreating] = useState(false);
    const [createError, setCreateError] = useState('');
    const navigate = useNavigate();
    async function load() {
        setLoading(true); setError('');
        try { setMatters((await api<{matters: Matter[]}>('/api/matters')).matters); }
        catch (error) { setError(message(error)); } finally { setLoading(false); }
    }
    useEffect(() => { void load(); }, []);
    async function create(event: FormEvent) {
        event.preventDefault(); setCreating(true); setCreateError('');
        try {
            const data = await api<{matter: Matter}>('/api/matters', { method: 'POST', body: json({ name: client.trim(), matter: name.trim() }) });
            navigate(`/matters/${data.matter.id}/documents`);
        } catch (error) { setCreateError(message(error)); } finally { setCreating(false); }
    }
    const outstanding = (matter: Matter) => matter.review_open ?? matter.open ?? 0;
    const shown = matters.filter(matter => `${matter.name} ${matter.matter} ${matter.reviewer_name}`.toLowerCase().includes(query.toLowerCase()) && (filter === 'all' || (filter === 'stale' ? matter.stale : filter === 'not_started' ? !matter.has_run : outstanding(matter) > 0))).sort((a,b) => {
        const value = sort === 'matter' ? a.matter.localeCompare(b.matter) : sort === 'reviewer' ? (a.reviewer_name || '').localeCompare(b.reviewer_name || '') : sort === 'documents' ? a.doc_count - b.doc_count : sort === 'review' ? outstanding(a) - outstanding(b) : (a.ran_at || 0) - (b.ran_at || 0);
        return descending ? -value : value;
    });
    function order(value: Sort) { if (sort === value) setDescending(!descending); else { setSort(value); setDescending(false); } }
    const heading = (value: Sort, label: string) => <th scope="col" aria-sort={sort === value ? descending ? 'descending' : 'ascending' : 'none'}><button onClick={() => order(value)}>{label}<ArrowDownUp size={13}/></button></th>;
    return <div className="page-enter">
        <div className="page-heading"><div><p className="eyebrow">FIRM WORKSPACE</p><h1>The matter docket</h1><p className="muted">Contracts, source evidence and the decisions that follow.</p></div><div className="inline-actions"><button className="button subtle" disabled={loading} onClick={load} aria-label="Refresh matters"><RefreshCw size={16}/></button>{user?.role !== 'viewer' && <button className="button primary" onClick={() => setShowCreate(!showCreate)}><Plus size={17}/>New matter</button>}</div></div>
        <div className="docket-summary" aria-label="Workspace totals"><span><strong>{loading ? '…' : error ? '—' : matters.length}</strong> matters</span><span><strong>{loading ? '…' : error ? '—' : matters.filter(m => m.has_run && outstanding(m) > 0).length}</strong> awaiting decisions</span><span><strong>{loading ? '…' : error ? '—' : matters.filter(m => m.stale).length}</strong> need a fresh audit</span></div>
        {showCreate && <section className="surface create-matter"><div className="section-heading"><h2>Create a matter</h2><button className="icon-button" aria-label="Close new matter form" onClick={() => setShowCreate(false)}><X size={18}/></button></div><form onSubmit={create} className="form-stack"><ErrorNotice error={createError}/><div className="form-grid"><div><label htmlFor="new-client">Client name</label><input id="new-client" autoFocus required maxLength={200} value={client} onChange={e => setClient(e.target.value)} placeholder="Acme Corporation"/></div><div><label htmlFor="new-matter">Matter name</label><input id="new-matter" required maxLength={200} value={name} onChange={e => setName(e.target.value)} placeholder="Commercial services agreement"/></div></div><button className="button primary" disabled={creating || !client.trim() || !name.trim()}>{creating ? 'Creating…' : 'Create & add documents'}<ArrowUpRight size={16}/></button></form></section>}
        <section className="surface docket">
            <div className="toolbar"><div className="search-field"><Search size={17}/><label htmlFor="matter-search" className="sr-only">Search matters</label><input id="matter-search" placeholder="Search client, matter or reviewer" value={query} onChange={e => setQuery(e.target.value)}/></div><label className="sr-only" htmlFor="matter-filter">Matter status</label><select id="matter-filter" value={filter} onChange={e => setFilter(e.target.value)}><option value="all">All matters</option><option value="open">Review outstanding</option><option value="not_started">Not audited</option><option value="stale">Needs a new audit</option></select></div>
            {loading ? <Loading/> : error ? <ErrorNotice error={error} retry={load}/> : !shown.length ? <EmptyState title={matters.length ? 'No matching matters' : 'Your first matter starts here'} text={matters.length ? 'Try a different search or status filter.' : 'Create a matter and add the source contract to begin.'}/> : <div className="table-scroll docket-scroll" role="region" aria-label="Matter docket"><table className="matter-table"><thead><tr>{heading('matter', 'Client / matter')}{heading('reviewer', 'Reviewer')}{heading('documents', 'Documents')}{heading('review', 'Review status')}{heading('updated', 'Last audit')}<th scope="col"><span className="sr-only">Open matter</span></th></tr></thead><tbody>{shown.map(matter => <tr key={matter.id}><th scope="row"><Link to={`/matters/${matter.id}/dashboard`} className="docket-title"><small>{matter.name}</small><strong>{matter.matter}</strong><span className="matter-id">{matter.id.slice(0,8).toUpperCase()}</span></Link></th><td data-label="Reviewer">{matter.reviewer_name || 'Unassigned'}</td><td data-label="Documents">{matter.doc_count || 0}</td><td data-label="Review status"><span className={`badge ${matter.stale ? 'missing' : !matter.has_run ? 'neutral' : outstanding(matter) ? 'medium' : 'clear'}`}>{matter.stale ? 'Audit out of date' : matter.has_run ? `${outstanding(matter)} to review` : 'Not audited'}</span></td><td data-label="Last audit" className="docket-date">{dateTime(matter.ran_at)}</td><td className="docket-open"><Link className="icon-button" to={`/matters/${matter.id}/dashboard`} aria-label={`Open ${matter.matter}`}><ArrowUpRight size={18}/></Link></td></tr>)}</tbody></table></div>}
            {!loading && !error && <p className="table-footnote">{shown.length} of {matters.length} matters · Only your firm's workspace</p>}
        </section>
    </div>;
}
