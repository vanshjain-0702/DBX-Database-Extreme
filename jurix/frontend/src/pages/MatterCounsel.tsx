import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Search, FileText, ArrowUpRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import { useMatter } from './MatterLayout';
import { api, json, message, type DocumentContent } from '../lib/api';
import { Loading, ErrorNotice, EmptyState } from '../components/Feedback';
interface Hit {
    id?: string;
    doc_id?: string;
    doc_name: string;
    text: string;
    index?: number;
    citation?: string;
    score?: number;
}
export default function MatterCounsel() {
    const { matter } = useMatter();
    const [query, setQuery] = useState('');
    const [searched, setSearched] = useState('');
    const [hits, setHits] = useState<Hit[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [source, setSource] = useState<DocumentContent | null>(null);
    const [sourceBusy, setSourceBusy] = useState(false);
    const [sourceError, setSourceError] = useState('');
    const readRequest = useRef(0);
    useEffect(() => () => { readRequest.current++; }, []);
    async function search(event: FormEvent) { event.preventDefault(); if (!query.trim())
        return; setBusy(true); setError(''); readRequest.current++; setSource(null); setSourceBusy(false); setSourceError(''); setSearched(query.trim()); try {
        const data = await api<{
            hits: Hit[];
        }>(`/api/matters/${matter.id}/search`, { method: 'POST', body: json({ query: query.trim() }) });
        setHits(data.hits || []);
    }
    catch (error) {
        setError(message(error));
    }
    finally {
        setBusy(false);
    } }
    async function read(hit: Hit) { const requestId = ++readRequest.current; setSource(null); setSourceBusy(true); setSourceError(''); try {
        const docId = hit.doc_id || matter.documents?.find(doc => doc.name === hit.doc_name)?.id;
        if (!docId)
            throw new Error('Source document could not be identified. Open it in Documents.');
        const data = await api<DocumentContent>(`/api/matters/${matter.id}/document?doc_id=${encodeURIComponent(docId)}`);
        if (requestId === readRequest.current) setSource(data);
    }
    catch (error) {
        if (requestId === readRequest.current) setSourceError(message(error));
    }
    finally {
        if (requestId === readRequest.current) setSourceBusy(false);
    } }
    return <section className="surface document-search"><div className="section-heading counsel-heading"><div><p className="eyebrow">FIND THE SOURCE</p><h2>Document search</h2></div></div><p className="muted">Find passages in this matter’s documents. Results are source text for your review.</p>
    <form onSubmit={search} className="document-search-form"><label className="sr-only" htmlFor="document-query">Search contract text</label><input id="document-query" value={query} onChange={e => setQuery(e.target.value)} placeholder="Search for liability, renewal or a specific obligation…"/><button className="button primary" disabled={busy || !query.trim() || !matter.doc_count}><Search size={16}/>Search</button></form><div className="suggested-searches"><span className="mini-label">TRY</span>{['Liability cap', 'Governing law', 'Renewal notice', 'Return customer data'].map(value => <button className="text-button" key={value} onClick={() => setQuery(value)}>{value}</button>)}</div><ErrorNotice error={error}/>
    {busy ? <Loading text="Searching document passages…"/> : !matter.doc_count ? <EmptyState title="No documents to search" text="Add contract text to this matter first."><Link className="button subtle" to="../documents">Add documents<ArrowUpRight size={15}/></Link></EmptyState> : searched && !error ? hits.length ? <div className="search-results"><p className="mini-label">{hits.length} PASSAGES FOR “{searched}”</p>{hits.map((hit, index) => <article key={hit.id || index}><span className="step-number">{String(index + 1).padStart(2, '0')}</span><div><p>{hit.text}</p><footer><FileText size={13}/>{hit.doc_name}{hit.citation ? ` · ${hit.citation}` : ''}</footer><button className="text-button" disabled={sourceBusy} onClick={() => read(hit)}>Read source document<ArrowUpRight size={14}/></button></div></article>)}</div> : <EmptyState title="No matching passages" text="Try a shorter phrase or check the document’s wording."/> : <EmptyState title="Start with a phrase or a clause" text="Search the document corpus and open the original text for context."/>}
    <ErrorNotice error={sourceError}/>{sourceBusy ? <Loading text="Reading source…"/> : source && <div className="search-source"><div className="section-heading"><h3>{source.name}</h3><button className="text-button" onClick={() => { readRequest.current++; setSource(null); setSourceBusy(false); }}>Close source</button></div>{source.chunks.map(chunk => <section key={chunk.id} className="source-chunk"><span className="mini-label">PASSAGE {chunk.index + 1}</span><p>{chunk.text}</p></section>)}</div>}
  </section>;
}
